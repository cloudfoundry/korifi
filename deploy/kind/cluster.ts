/**
 * Kind cluster matching INSTALL.kind.md / scripts/assets/kind-config.yaml.
 *
 * Creates the cluster with ingress + registry NodePorts, optional UAA OIDC
 * patches, writes kubeconfig, and configures containerd for the local registry.
 */
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import * as command from "@pulumi/command";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import { kindRegistry } from "@korifi/deploy-lib";

export interface KindOidcArgs {
	/** Full issuer URL including path, e.g. https://uaa…/uaa/oauth/token */
	issuerUrl: string;
	/** Host directory containing ca.pem (mounted read-only at /ssl). */
	caDir: string;
	/** Filename of the CA inside caDir (default ca.pem). */
	caFileName?: string;
	clientId?: string;
	usernameClaim?: string;
	usernamePrefix?: string;
}

export interface KindClusterArgs {
	clusterName: string;
	kubeconfigPath?: string;
	/** Directory containing kind-config.yaml (defaults to this module's dir). */
	configDir?: string;
	/** When set, kind is created/recreated with kube-apiserver OIDC flags. */
	oidc?: KindOidcArgs;
}

export class KindCluster extends pulumi.ComponentResource {
	readonly kubeconfig: pulumi.Output<string>;
	readonly kubeconfigPath: string;
	readonly provider: k8s.Provider;
	readonly clusterName: string;

	constructor(
		name: string,
		args: KindClusterArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:KindCluster", name, {}, opts);

		this.clusterName = args.clusterName;
		this.kubeconfigPath =
			args.kubeconfigPath ??
			path.join(os.homedir(), ".kube", `kind-${args.clusterName}.config`);

		const configDir = args.configDir ?? __dirname;
		const baseConfigPath = path.join(configDir, "kind-config.yaml");
		if (!fs.existsSync(baseConfigPath)) {
			throw new Error(`kind config not found: ${baseConfigPath}`);
		}

		const generatedConfigPath = path.join(
			os.tmpdir(),
			`korifi-kind-${args.clusterName}.yaml`,
		);
		const kindConfigPath = args.oidc
			? writeOidcKindConfig(baseConfigPath, generatedConfigPath, args.oidc)
			: baseConfigPath;

		const registryDir = `/etc/containerd/certs.d/${kindRegistry.clusterHost}`;
		const controlPlane = `${args.clusterName}-control-plane`;
		const expectedIssuer = args.oidc?.issuerUrl;

		const bootstrap = new command.local.Command(
			`${name}-bootstrap`,
			{
				create: [
					`set -euo pipefail`,
					`CLUSTER='${args.clusterName}'`,
					`CONTROL_PLANE='${controlPlane}'`,
					`CONFIG='${kindConfigPath}'`,
					`KUBECONFIG_OUT='${this.kubeconfigPath}'`,
					`need_create=0`,
					`if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then`,
					`  need_create=1`,
					`elif [ -n '${expectedIssuer ?? ""}' ]; then`,
					`  if ! docker exec "$CONTROL_PLANE" sh -c 'ps auxww | grep -F "[k]ube-apiserver" | grep -Fq "oidc-issuer-url=${expectedIssuer ?? ""}"' 2>/dev/null; then`,
					`    echo "kind cluster $CLUSTER exists without expected OIDC issuer; recreating"`,
					`    kind delete cluster --name "$CLUSTER"`,
					`    need_create=1`,
					`  fi`,
					`fi`,
					`if [ "$need_create" -eq 1 ]; then`,
					`  kind create cluster --name "$CLUSTER" --wait 5m --config "$CONFIG"`,
					`fi`,
					`kind export kubeconfig --name "$CLUSTER" --kubeconfig "$KUBECONFIG_OUT"`,
					`docker exec -i "$CONTROL_PLANE" sh -c "mkdir -p '${registryDir}' && cat >'${registryDir}/hosts.toml'" <<'EOF'`,
					`[host."http://127.0.0.1:${kindRegistry.nodePort}"]`,
					`EOF`,
					`cat "$KUBECONFIG_OUT"`,
				].join("\n"),
				delete: [
					`set -euo pipefail`,
					`kind delete cluster --name '${args.clusterName}' || true`,
					`rm -f '${this.kubeconfigPath}'`,
					args.oidc ? `rm -f '${generatedConfigPath}'` : `true`,
				].join("\n"),
				logging: command.local.Logging.Stderr,
			},
			{ parent: this, additionalSecretOutputs: ["stdout"] },
		);

		this.kubeconfig = bootstrap.stdout;
		this.provider = new k8s.Provider(
			`${name}-k8s`,
			{ kubeconfig: this.kubeconfig, enableServerSideApply: true },
			{ parent: this, dependsOn: [bootstrap] },
		);

		this.registerOutputs({
			clusterName: this.clusterName,
			kubeconfigPath: this.kubeconfigPath,
		});
	}
}

/**
 * Merge OIDC mounts/patches into the base kind config without a YAML library.
 * Preserves containerd + extraPortMappings from the checked-in template.
 */
function writeOidcKindConfig(
	basePath: string,
	outPath: string,
	oidc: KindOidcArgs,
): string {
	const caFile = oidc.caFileName ?? "ca.pem";
	const clientId = oidc.clientId ?? "cf";
	const usernameClaim = oidc.usernameClaim ?? "user_name";
	const usernamePrefix = oidc.usernamePrefix ?? "uaa:";
	const base = fs.readFileSync(basePath, "utf8");

	// Strip the trailing nodes section and rebuild with OIDC fields on the
	// control-plane node while keeping port mappings from the template.
	const portMappingsMatch = base.match(
		/extraPortMappings:\n(?:  - containerPort:.*\n    hostPort:.*\n    protocol:.*\n)+/,
	);
	let portMappings = portMappingsMatch?.[0] ?? "";
	if (!portMappings.includes("30443")) {
		portMappings += `  - containerPort: 30443
    hostPort: 30443
    protocol: TCP
`;
	}
	const containerdMatch = base.match(
		/containerdConfigPatches:\n(?:- \|-\n(?:  .*\n)+)/,
	);
	const containerd = containerdMatch?.[0] ?? "";

	const content = `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
${containerd}nodes:
- role: control-plane
  extraMounts:
  - containerPath: /ssl
    hostPath: ${oidc.caDir}
    readOnly: true
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraVolumes:
        - name: ssl-certs
          hostPath: /ssl
          mountPath: /etc/uaa-ssl
      extraArgs:
        oidc-issuer-url: ${oidc.issuerUrl}
        oidc-client-id: ${clientId}
        oidc-ca-file: /etc/uaa-ssl/${caFile}
        oidc-username-claim: ${usernameClaim}
        oidc-username-prefix: "${usernamePrefix}"
        oidc-signing-algs: "RS256"
  ${portMappings.trimStart()}
`;
	fs.writeFileSync(outPath, content);
	return outPath;
}
