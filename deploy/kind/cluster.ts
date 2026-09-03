/**
 * Kind cluster matching INSTALL.kind.md / scripts/assets/kind-config.yaml.
 *
 * Creates the cluster with ingress + registry NodePorts, writes kubeconfig,
 * and configures containerd to pull from the in-cluster registry over HTTP.
 */
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import * as command from "@pulumi/command";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import { kindRegistry } from "@korifi/deploy-lib";

export interface KindClusterArgs {
	clusterName: string;
	kubeconfigPath?: string;
	/** Directory containing kind-config.yaml (defaults to this module's dir). */
	configDir?: string;
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
		const kindConfigPath = path.join(configDir, "kind-config.yaml");
		if (!fs.existsSync(kindConfigPath)) {
			throw new Error(`kind config not found: ${kindConfigPath}`);
		}

		const registryDir = `/etc/containerd/certs.d/${kindRegistry.clusterHost}`;
		const controlPlane = `${args.clusterName}-control-plane`;

		const bootstrap = new command.local.Command(
			`${name}-bootstrap`,
			{
				create: [
					`set -euo pipefail`,
					`if ! kind get clusters 2>/dev/null | grep -qx '${args.clusterName}'; then`,
					`  kind create cluster --name '${args.clusterName}' --wait 5m --config '${kindConfigPath}'`,
					`fi`,
					`kind export kubeconfig --name '${args.clusterName}' --kubeconfig '${this.kubeconfigPath}'`,
					`docker exec -i '${controlPlane}' sh -c "mkdir -p '${registryDir}' && cat >'${registryDir}/hosts.toml'" <<'EOF'`,
					`[host."http://127.0.0.1:${kindRegistry.nodePort}"]`,
					`EOF`,
					`cat '${this.kubeconfigPath}'`,
				].join("\n"),
				delete: [
					`set -euo pipefail`,
					`kind delete cluster --name '${args.clusterName}' || true`,
					`rm -f '${this.kubeconfigPath}'`,
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
