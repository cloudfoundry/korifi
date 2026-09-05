/**
 * OpenEverest in a vcluster: database operators stay off the Korifi host.
 *
 * This component does not create database clusters. OSB provisions one
 * DatabaseCluster per CF service instance via the in-cluster kubeconfig.
 *
 * Port-forward pattern matches UaaVcluster; API listen port is distinct so
 * both vclusters can be up on the same kind node.
 */
import * as os from "node:os";
import * as path from "node:path";
import * as command from "@pulumi/command";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import { versions } from "./versions";

/** Host port for `kubectl port-forward` to the Everest vcluster API. */
export const kindEverestVclusterLocalApiPort = 18444 as const;

export interface EverestVclusterArgs {
	provider: k8s.Provider;
	kindClusterName: string;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class EverestVcluster extends pulumi.ComponentResource {
	readonly namespace: string;
	readonly dbNamespace: string;
	readonly vclusterName: string;
	readonly virtualProvider: k8s.Provider;
	/** Kubeconfig the OSB broker uses from inside the host cluster. */
	readonly inClusterKubeconfig: pulumi.Output<string>;
	readonly ready: pulumi.Resource;

	constructor(
		name: string,
		args: EverestVclusterArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:EverestVcluster", name, {}, opts);

		this.namespace = "everest-vcluster";
		this.dbNamespace = "everest";
		this.vclusterName = "everest";
		const vclusterName = this.vclusterName;

		const childOpts: pulumi.CustomResourceOptions = {
			parent: this,
			provider: args.provider,
			dependsOn: args.dependsOn,
		};

		const ns = new k8s.core.v1.Namespace(
			`${name}-ns`,
			{ metadata: { name: this.namespace } },
			childOpts,
		);

		const vclusterRelease = new k8s.helm.v3.Release(
			`${name}-vcluster`,
			{
				name: vclusterName,
				chart: "vcluster",
				version: versions.vclusterChart,
				repositoryOpts: { repo: "https://charts.loft.sh" },
				namespace: this.namespace,
				values: {
					exportKubeConfig: {
						server: `https://${vclusterName}.${this.namespace}.svc`,
						secret: { name: `vc-${vclusterName}` },
					},
					sync: {
						toHost: {
							services: { enabled: true },
						},
					},
				},
				timeout: 900,
			},
			{
				...childOpts,
				dependsOn: [...(args.dependsOn ?? []), ns],
				customTimeouts: { create: "20m", update: "20m" },
			},
		);

		const hostKubeconfig = `$HOME/.kube/kind-${args.kindClusterName}.config`;
		const pfPidFile = path.join(
			os.tmpdir(),
			`korifi-vcluster-${vclusterName}-pf.pid`,
		);
		const pfLogFile = path.join(
			os.tmpdir(),
			`korifi-vcluster-${vclusterName}-pf.log`,
		);
		const apiPort = String(kindEverestVclusterLocalApiPort);

		const apiForward = new command.local.Command(
			`${name}-api-forward`,
			{
				create: vclusterForwardScript({
					hostKubeconfig,
					namespace: this.namespace,
					svc: vclusterName,
					pidFile: pfPidFile,
					logFile: pfLogFile,
					port: apiPort,
					mode: "create",
				}),
				update: vclusterForwardScript({
					hostKubeconfig,
					namespace: this.namespace,
					svc: vclusterName,
					pidFile: pfPidFile,
					logFile: pfLogFile,
					port: apiPort,
					mode: "update",
				}),
				delete: `set -euo pipefail
PIDFILE='${pfPidFile}'
if [ -f "$PIDFILE" ]; then
  kill "$(cat "$PIDFILE")" 2>/dev/null || true
  rm -f "$PIDFILE"
fi
`,
				triggers: ["everest-vcluster-api-forward-v1"],
			},
			{ parent: this, dependsOn: [vclusterRelease] },
		);

		const kubeconfigCmd = new command.local.Command(
			`${name}-kubeconfig`,
			{
				create: `set -euo pipefail
KUBECONFIG="${hostKubeconfig}"
NS="${this.namespace}"
SECRET="vc-${vclusterName}"
PORT='${apiPort}'
for i in $(seq 1 90); do
  if kubectl --kubeconfig "$KUBECONFIG" -n "$NS" get secret "$SECRET" -o jsonpath='{.data.config}' 2>/dev/null | grep -q .; then
    kubectl --kubeconfig "$KUBECONFIG" -n "$NS" get secret "$SECRET" -o go-template='{{ index .data.config | base64decode }}' \\
      | sed -E "s#server: https?://[^[:space:]]+#server: https://127.0.0.1:$PORT#"
    exit 0
  fi
  sleep 5
done
echo "timed out waiting for kubeconfig secret/$SECRET in $NS" >&2
exit 1
`,
			},
			{
				parent: this,
				dependsOn: [vclusterRelease, apiForward],
				additionalSecretOutputs: ["stdout"],
			},
		);

		const pulumiKubeconfig = kubeconfigCmd.stdout.apply((raw) =>
			raw.replace(
				/server:\s*https?:\/\/[^\s]+/,
				`server: https://127.0.0.1:${apiPort}`,
			),
		);
		this.inClusterKubeconfig = kubeconfigCmd.stdout.apply((raw) =>
			raw.replace(
				/server:\s*https?:\/\/[^\s]+/,
				`server: https://${vclusterName}.${this.namespace}.svc`,
			),
		);

		this.virtualProvider = new k8s.Provider(
			`${name}-virtual-k8s`,
			{
				kubeconfig: pulumiKubeconfig,
				enableServerSideApply: true,
			},
			{ parent: this, dependsOn: [kubeconfigCmd, apiForward] },
		);

		const virtualOpts: pulumi.CustomResourceOptions = {
			parent: this,
			provider: this.virtualProvider,
		};

		const systemNs = new k8s.core.v1.Namespace(
			`${name}-everest-system`,
			{ metadata: { name: "everest-system" } },
			virtualOpts,
		);

		const everest = new k8s.helm.v3.Release(
			`${name}-everest`,
			{
				name: "everest",
				chart: "openeverest",
				version: versions.openeverestChart,
				repositoryOpts: {
					repo: "https://openeverest.github.io/helm-charts/",
				},
				namespace: "everest-system",
				timeout: 900,
				values: {
					telemetry: false,
					createMonitoringResources: false,
					"kube-state-metrics": { enabled: false },
					monitoring: { enabled: false },
					dataImporters: {
						perconaPXCOperator: { enabled: false },
						perconaPSMDBOperator: { enabled: false },
					},
					dbNamespace: {
						enabled: true,
						namespaceOverride: this.dbNamespace,
						postgresql: true,
						pxc: false,
						psmdb: false,
					},
				},
			},
			{
				...virtualOpts,
				dependsOn: [systemNs],
				customTimeouts: { create: "20m", update: "20m" },
			},
		);

		this.ready = everest;
		this.registerOutputs({
			namespace: this.namespace,
			dbNamespace: this.dbNamespace,
		});
	}
}

function vclusterForwardScript(args: {
	hostKubeconfig: string;
	namespace: string;
	svc: string;
	pidFile: string;
	logFile: string;
	port: string;
	mode: "create" | "update";
}): string {
	const skipIfUp =
		args.mode === "update"
			? `if curl -sk --connect-timeout 1 "https://127.0.0.1:${args.port}/readyz" >/dev/null 2>&1; then
  echo "vcluster API already forwarded on 127.0.0.1:${args.port}"
  exit 0
fi
`
			: "";
	return `set -euo pipefail
KUBECONFIG="${args.hostKubeconfig}"
NS="${args.namespace}"
SVC="${args.svc}"
PIDFILE='${args.pidFile}'
LOGFILE='${args.logFile}'
PORT='${args.port}'
${skipIfUp}if [ -f "$PIDFILE" ]; then
  kill "$(cat "$PIDFILE")" 2>/dev/null || true
  rm -f "$PIDFILE"
fi
for i in $(seq 1 90); do
  kubectl --kubeconfig "$KUBECONFIG" -n "$NS" get svc "$SVC" >/dev/null 2>&1 && break
  sleep 2
done
kubectl --kubeconfig "$KUBECONFIG" -n "$NS" get svc "$SVC" >/dev/null
nohup kubectl --kubeconfig "$KUBECONFIG" -n "$NS" port-forward "svc/$SVC" "$PORT:443" >"$LOGFILE" 2>&1 &
echo $! >"$PIDFILE"
for i in $(seq 1 60); do
  if curl -sk --connect-timeout 1 "https://127.0.0.1:$PORT/readyz" >/dev/null 2>&1 \\
    || nc -z 127.0.0.1 "$PORT" 2>/dev/null; then
    echo "vcluster API forwarded on 127.0.0.1:$PORT"
    exit 0
  fi
  sleep 1
done
echo "timed out waiting for port-forward on $PORT" >&2
cat "$LOGFILE" >&2 || true
exit 1
`;
}
