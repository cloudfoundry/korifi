/**
 * Deploy the in-tree `osb-service` OSB process and register it as a
 * CFServiceBroker. HTTPS only (port 8443). Pass `tlsSecretName` for a
 * platform-managed certificate; otherwise a cert-manager self-signed cert is
 * issued. Self-signed material needs Korifi `trustInsecureBrokers`.
 *
 * Backing stores come from ServiceBrokerServices via `backends`. Postgres is
 * first; add keys (and env) as offerings land in osb-service.
 *
 * Plans default to admin visibility; `cf enable-service-access postgres`
 * after install.
 */
import * as path from "node:path";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import * as random from "@pulumi/random";
import type { ServiceBrokerServiceConnection } from "./service-broker-services";

export const osbServiceBrokerGuid = "11111111-1111-4111-8111-111111111111";

const nsName = "osb-service";
const appName = "osb-service";
const fqdn = `${appName}.${nsName}.svc.cluster.local`;

export interface OsbServiceBrokerArgs {
	provider: k8s.Provider;
	image: pulumi.Input<string>;
	/** Default IfNotPresent. Kind load uses Never. */
	imagePullPolicy?: string;
	backends: {
		postgres?: ServiceBrokerServiceConnection;
	};
	/**
	 * Existing kubernetes.io/tls secret in the broker namespace.
	 * When omitted, cert-manager issues a self-signed certificate.
	 */
	tlsSecretName?: string;
	rootNamespace?: string;
	brokerUsername?: string;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class OsbServiceBroker extends pulumi.ComponentResource {
	readonly url: string;
	readonly namespace: k8s.core.v1.Namespace;
	readonly service: k8s.core.v1.Service;
	readonly broker: k8s.apiextensions.CustomResource;

	constructor(
		name: string,
		args: OsbServiceBrokerArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:OsbServiceBroker", name, {}, opts);

		const rootNs = args.rootNamespace ?? "cf";
		const username = args.brokerUsername ?? "broker";
		const pullPolicy = args.imagePullPolicy ?? "IfNotPresent";
		const child = {
			parent: this,
			provider: args.provider,
			dependsOn: args.dependsOn,
		};

		this.namespace = new k8s.core.v1.Namespace(
			`${name}-ns`,
			{
				metadata: {
					name: nsName,
					labels: {
						"pod-security.kubernetes.io/enforce": "restricted",
						"pod-security.kubernetes.io/audit": "restricted",
					},
				},
			},
			child,
		);

		const tlsSecretName = args.tlsSecretName ?? `${appName}-tls`;
		const certResources: pulumi.Resource[] = [];
		if (!args.tlsSecretName) {
			const issuer = new k8s.apiextensions.CustomResource(
				`${name}-issuer`,
				{
					apiVersion: "cert-manager.io/v1",
					kind: "Issuer",
					metadata: {
						name: `${appName}-selfsigned`,
						namespace: this.namespace.metadata.name,
					},
					spec: { selfSigned: {} },
				},
				{ ...child, dependsOn: [this.namespace] },
			);
			const certificate = new k8s.apiextensions.CustomResource(
				`${name}-certificate`,
				{
					apiVersion: "cert-manager.io/v1",
					kind: "Certificate",
					metadata: {
						name: appName,
						namespace: this.namespace.metadata.name,
					},
					spec: {
						secretName: tlsSecretName,
						commonName: fqdn,
						dnsNames: [
							appName,
							`${appName}.${nsName}`,
							`${appName}.${nsName}.svc`,
							fqdn,
						],
						issuerRef: {
							name: `${appName}-selfsigned`,
							kind: "Issuer",
						},
					},
				},
				{ ...child, dependsOn: [this.namespace, issuer] },
			);
			certResources.push(issuer, certificate);
		}

		const password = new random.RandomPassword(
			`${name}-auth-password`,
			{ length: 24, special: false },
			{ parent: this },
		);

		const stringData: Record<string, pulumi.Input<string>> = {
			BROKER_USERNAME: username,
			BROKER_PASSWORD: password.result,
		};
		const backendResources: pulumi.Resource[] = [];
		const postgres = args.backends.postgres;
		if (postgres) {
			stringData.POSTGRES_HOST = postgres.host;
			stringData.POSTGRES_PORT = String(postgres.port);
			stringData.POSTGRES_USER = postgres.adminUser;
			stringData.POSTGRES_PASSWORD = postgres.adminPassword;
			stringData.POSTGRES_DB = "postgres";
			stringData.POSTGRES_SSLMODE = postgres.sslMode ?? "require";
			backendResources.push(...postgres.resources);
		}

		const secret = new k8s.core.v1.Secret(
			`${name}-secret`,
			{
				metadata: { name: appName, namespace: this.namespace.metadata.name },
				stringData,
			},
			{ ...child, dependsOn: [this.namespace] },
		);

		const labels = { app: appName };
		const deployment = new k8s.apps.v1.Deployment(
			`${name}-deploy`,
			{
				metadata: { name: appName, namespace: this.namespace.metadata.name },
				spec: {
					replicas: 1,
					selector: { matchLabels: labels },
					template: {
						metadata: { labels },
						spec: {
							securityContext: {
								runAsNonRoot: true,
								runAsUser: 65532,
								runAsGroup: 65532,
								fsGroup: 65532,
								seccompProfile: { type: "RuntimeDefault" },
							},
							containers: [
								{
									name: "broker",
									image: args.image,
									imagePullPolicy: pullPolicy,
									args: [
										"--port",
										"8443",
										"--tls-cert-file",
										"/var/run/osb-service/tls.crt",
										"--tls-private-key-file",
										"/var/run/osb-service/tls.key",
									],
									envFrom: [{ secretRef: { name: secret.metadata.name } }],
									ports: [{ containerPort: 8443, name: "https" }],
									readinessProbe: {
										httpGet: {
											path: "/healthz",
											port: 8443,
											scheme: "HTTPS",
										},
										initialDelaySeconds: 3,
										periodSeconds: 5,
									},
									securityContext: {
										allowPrivilegeEscalation: false,
										capabilities: { drop: ["ALL"] },
										readOnlyRootFilesystem: true,
										runAsNonRoot: true,
										runAsUser: 65532,
									},
									resources: {
										requests: { cpu: "50m", memory: "64Mi" },
										limits: { memory: "256Mi" },
									},
									volumeMounts: [
										{
											name: "tls",
											mountPath: "/var/run/osb-service",
											readOnly: true,
										},
									],
								},
							],
							volumes: [
								{
									name: "tls",
									secret: {
										secretName: tlsSecretName,
										items: [
											{ key: "tls.crt", path: "tls.crt" },
											{ key: "tls.key", path: "tls.key" },
										],
									},
								},
							],
						},
					},
				},
			},
			{
				...child,
				dependsOn: [
					this.namespace,
					secret,
					...certResources,
					...backendResources,
				],
			},
		);

		this.service = new k8s.core.v1.Service(
			`${name}-svc`,
			{
				metadata: { name: appName, namespace: this.namespace.metadata.name },
				spec: {
					type: "ClusterIP",
					selector: labels,
					ports: [{ name: "https", port: 443, targetPort: 8443 }],
				},
			},
			{ ...child, dependsOn: [this.namespace] },
		);

		this.url = `https://${fqdn}`;

		const cfCreds = new k8s.core.v1.Secret(
			`${name}-cf-credentials`,
			{
				metadata: {
					name: "osb-service-broker-credentials",
					namespace: rootNs,
				},
				stringData: {
					credentials: pulumi.interpolate`{"username":"${username}","password":"${password.result}"}`,
				},
			},
			child,
		);

		this.broker = new k8s.apiextensions.CustomResource(
			`${name}-cfservicebroker`,
			{
				apiVersion: "korifi.cloudfoundry.org/v1alpha1",
				kind: "CFServiceBroker",
				metadata: {
					name: osbServiceBrokerGuid,
					namespace: rootNs,
				},
				spec: {
					name: "osb-service",
					url: this.url,
					credentials: { name: "osb-service-broker-credentials" },
				},
			},
			{ ...child, dependsOn: [cfCreds, this.service, deployment] },
		);

		this.registerOutputs({
			url: this.url,
		});
	}
}

/** In-tree broker sources: `<repo>/osb-service`. */
export function osbServicePath(korifiRepoRoot: string): string {
	return path.resolve(korifiRepoRoot, "osb-service");
}
