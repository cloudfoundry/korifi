/**
 * UAA in a vcluster on the kind host, published via a TLS NodePort proxy.
 *
 * Why NodePort (not Contour): Korifi’s Gateway already owns host :443 / 32443.
 * A second HTTPS listener risks SSA wiping listeners or NodePort collisions.
 * A dedicated NodePort (30443) keeps one issuer URL reachable from both the
 * host (`cf login`) and kube-apiserver (NodePort on the kind node loopback).
 *
 * vcluster syncs the virtual `default/uaa` Service to the host namespace;
 * nginx on the host terminates TLS and proxies to that Service.
 *
 * The virtual-cluster kubeconfig defaults to https://localhost:8443 (needs a
 * port-forward). We keep a host-side forward on kindVclusterLocalApiPort so
 * Pulumi’s kubernetes provider can reach the vcluster API from the Mac/host.
 */
import * as os from "node:os";
import * as path from "node:path";
import * as command from "@pulumi/command";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import * as random from "@pulumi/random";
import * as tls from "@pulumi/tls";
import { versions } from "./versions";
import type { UaaCerts } from "./uaa-certs";

/** Kind NodePort / hostPort for the UAA TLS proxy. */
export const kindUaaNodePort = 30443 as const;

/** Host port for `kubectl port-forward` to the vcluster API Service. */
export const kindVclusterLocalApiPort = 18443 as const;

export interface UaaVclusterArgs {
	provider: k8s.Provider;
	certs: UaaCerts;
	/** Kind cluster name (kubeconfig at ~/.kube/kind-<name>.config). */
	kindClusterName: string;
	/** Public UAA base URL (issuer.uri / experimental.uaa.url). */
	uaaUrl?: string;
	namespace?: string;
	vclusterName?: string;
	adminEmail?: string;
	oidcPrefix?: string;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class UaaVcluster extends pulumi.ComponentResource {
	readonly uaaUrl: string;
	readonly adminEmail: string;
	readonly adminPassword: pulumi.Output<string>;
	readonly oidcPrefix: string;
	readonly adminUserName: string;
	readonly namespace: string;
	readonly vclusterRelease: k8s.helm.v3.Release;
	readonly virtualProvider: k8s.Provider;
	readonly proxyService: k8s.core.v1.Service;

	constructor(
		name: string,
		args: UaaVclusterArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:UaaVcluster", name, {}, opts);

		this.uaaUrl =
			args.uaaUrl ?? `https://127.0.0.1:${kindUaaNodePort}/uaa`;
		this.adminEmail = args.adminEmail ?? "admin@korifi.local";
		this.oidcPrefix = args.oidcPrefix ?? "uaa";
		this.adminUserName = `${this.oidcPrefix}:${this.adminEmail}`;
		this.namespace = args.namespace ?? "uaa-vcluster";
		const vclusterName = args.vclusterName ?? "uaa";
		// Must not be the Helm release name: that Service is the vcluster API
		// endpoint. Replicating virtual default/uaa onto the same name drops
		// the API Service and blocks kubeconfig/cert generation.
		const hostUaaService = "uaa-app";

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

		const adminPassword = new random.RandomPassword(
			`${name}-admin-password`,
			{ length: 24, special: false },
			{ parent: this },
		);
		this.adminPassword = adminPassword.result;

		const jwtKey = new tls.PrivateKey(
			`${name}-jwt-key`,
			{ algorithm: "RSA", rsaBits: 2048 },
			{ parent: this },
		);

		const samlKey = new tls.PrivateKey(
			`${name}-saml-key`,
			{ algorithm: "RSA", rsaBits: 2048 },
			{ parent: this },
		);
		const samlCert = new tls.SelfSignedCert(
			`${name}-saml-cert`,
			{
				privateKeyPem: samlKey.privateKeyPem,
				validityPeriodHours: 24 * 365 * 5,
				allowedUses: ["key_encipherment", "digital_signature"],
				subject: { commonName: "uaa-saml" },
			},
			{ parent: this },
		);

		this.vclusterRelease = new k8s.helm.v3.Release(
			`${name}-vcluster`,
			{
				name: vclusterName,
				chart: "vcluster",
				version: versions.vclusterChart,
				repositoryOpts: { repo: "https://charts.loft.sh" },
				namespace: this.namespace,
				values: {
					exportKubeConfig: {
						// Reachable via the host port-forward below (not in-pod localhost).
						server: `https://127.0.0.1:${kindVclusterLocalApiPort}`,
						secret: { name: `vc-${vclusterName}` },
					},
					networking: {
						replicateServices: {
							toHost: [
								{
									from: "default/uaa",
									to: hostUaaService,
								},
							],
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

		const apiForward = new command.local.Command(
			`${name}-api-forward`,
			{
				create: `set -euo pipefail
KUBECONFIG="${hostKubeconfig}"
NS="${this.namespace}"
SVC="${vclusterName}"
PIDFILE='${pfPidFile}'
LOGFILE='${pfLogFile}'
PORT='${kindVclusterLocalApiPort}'
if [ -f "$PIDFILE" ]; then
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
`,
				// Re-establish the forward on every update — create alone won't
				// re-run when the host process died between pulumi ups.
				update: `set -euo pipefail
KUBECONFIG="${hostKubeconfig}"
NS="${this.namespace}"
SVC="${vclusterName}"
PIDFILE='${pfPidFile}'
LOGFILE='${pfLogFile}'
PORT='${kindVclusterLocalApiPort}'
if curl -sk --connect-timeout 1 "https://127.0.0.1:$PORT/readyz" >/dev/null 2>&1; then
  echo "vcluster API already forwarded on 127.0.0.1:$PORT"
  exit 0
fi
if [ -f "$PIDFILE" ]; then
  kill "$(cat "$PIDFILE")" 2>/dev/null || true
  rm -f "$PIDFILE"
fi
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
`,
				delete: `set -euo pipefail
PIDFILE='${pfPidFile}'
if [ -f "$PIDFILE" ]; then
  kill "$(cat "$PIDFILE")" 2>/dev/null || true
  rm -f "$PIDFILE"
fi
`,
				// Bump when the forward script changes; do not use Date.now() —
				// that replaces the Command every up and kills the live forward.
				triggers: ["vcluster-api-forward-v2"],
			},
			{ parent: this, dependsOn: [this.vclusterRelease] },
		);

		const kubeconfigCmd = new command.local.Command(
			`${name}-kubeconfig`,
			{
				create: `set -euo pipefail
KUBECONFIG="${hostKubeconfig}"
NS="${this.namespace}"
SECRET="vc-${vclusterName}"
PORT='${kindVclusterLocalApiPort}'
for i in $(seq 1 90); do
  if kubectl --kubeconfig "$KUBECONFIG" -n "$NS" get secret "$SECRET" -o jsonpath='{.data.config}' 2>/dev/null | grep -q .; then
    kubectl --kubeconfig "$KUBECONFIG" -n "$NS" get secret "$SECRET" -o go-template='{{ index .data.config | base64decode }}' \\
      | sed -E "s#server: https?://[^[:space:]]+#server: https://127.0.0.1:$PORT#"
    exit 0
  fi
  sleep 5
done
echo "timed out waiting for kubeconfig secret/$SECRET in $NS" >&2
kubectl --kubeconfig "$KUBECONFIG" -n "$NS" get svc,secret >&2 || true
exit 1
`,
			},
			{
				parent: this,
				dependsOn: [this.vclusterRelease, apiForward],
				additionalSecretOutputs: ["stdout"],
			},
		);

		this.virtualProvider = new k8s.Provider(
			`${name}-virtual-k8s`,
			{
				kubeconfig: kubeconfigCmd.stdout,
				enableServerSideApply: true,
			},
			{ parent: this, dependsOn: [kubeconfigCmd, apiForward] },
		);

		const uaaConfig = pulumi
			.all([
				jwtKey.privateKeyPem,
				samlKey.privateKeyPem,
				samlCert.certPem,
				adminPassword.result,
			])
			.apply(([signingKey, samlPrivateKey, samlCertificate, password]) =>
				renderUaaYml({
					issuerUri: this.uaaUrl,
					adminEmail: this.adminEmail,
					adminPassword: password,
					signingKey,
					samlPrivateKey,
					samlCertificate,
				}),
			);

		const configMap = new k8s.core.v1.ConfigMap(
			`${name}-config`,
			{
				metadata: { name: "uaa-config", namespace: "default" },
				data: { "uaa.yml": uaaConfig },
			},
			{ parent: this, provider: this.virtualProvider },
		);

		const deployment = new k8s.apps.v1.Deployment(
			`${name}-deploy`,
			{
				metadata: { name: "uaa", namespace: "default" },
				spec: {
					replicas: 1,
					progressDeadlineSeconds: 900,
					selector: { matchLabels: { app: "uaa" } },
					template: {
						metadata: { labels: { app: "uaa" } },
						spec: {
							containers: [
								{
									name: "uaa",
									image: versions.uaaImage,
									ports: [{ containerPort: 8080, name: "http" }],
									env: [
										{
											name: "CLOUDFOUNDRY_CONFIG_PATH",
											value: "/config",
										},
										{
											name: "SERVER_SERVLET_CONTEXT_PATH",
											value: "/uaa",
										},
									],
									volumeMounts: [
										{
											name: "uaa-config",
											mountPath: "/config",
											readOnly: true,
										},
									],
									readinessProbe: {
										httpGet: { path: "/uaa/healthz", port: 8080 },
										initialDelaySeconds: 120,
										periodSeconds: 10,
										failureThreshold: 36,
									},
									livenessProbe: {
										httpGet: { path: "/uaa/healthz", port: 8080 },
										initialDelaySeconds: 300,
										periodSeconds: 20,
										failureThreshold: 12,
									},
									resources: {
										requests: { cpu: "100m", memory: "768Mi" },
										limits: { memory: "2Gi" },
									},
								},
							],
							volumes: [
								{
									name: "uaa-config",
									configMap: { name: "uaa-config" },
								},
							],
						},
					},
				},
			},
			{
				parent: this,
				provider: this.virtualProvider,
				dependsOn: [configMap],
				customTimeouts: { create: "20m", update: "20m" },
			},
		);

		const virtualService = new k8s.core.v1.Service(
			`${name}-svc`,
			{
				metadata: { name: "uaa", namespace: "default" },
				spec: {
					selector: { app: "uaa" },
					ports: [{ name: "http", port: 8080, targetPort: 8080 }],
				},
			},
			{
				parent: this,
				provider: this.virtualProvider,
				dependsOn: [deployment],
			},
		);

		const proxyTls = new k8s.core.v1.Secret(
			`${name}-proxy-tls`,
			{
				metadata: { name: "uaa-proxy-tls", namespace: this.namespace },
				type: "kubernetes.io/tls",
				stringData: {
					"tls.crt": args.certs.certPem,
					"tls.key": args.certs.privateKeyPem,
				},
			},
			{
				parent: this,
				provider: args.provider,
				dependsOn: [...(args.dependsOn ?? []), ns, args.certs.filesReady],
			},
		);

		const proxyConfig = new k8s.core.v1.ConfigMap(
			`${name}-proxy-config`,
			{
				metadata: { name: "uaa-proxy-config", namespace: this.namespace },
				data: {
					"nginx.conf": `
worker_processes 1;
events { worker_connections 1024; }
http {
  # UAA often emits absolute URLs without :30443 (assumes HTTPS default port).
  # Rewrite them so kube-apiserver OIDC JWKS fetch hits this NodePort proxy.
  sub_filter_types application/json text/plain;
  sub_filter 'https://127.0.0.1/' 'https://127.0.0.1:${kindUaaNodePort}/';
  sub_filter 'http://127.0.0.1/' 'https://127.0.0.1:${kindUaaNodePort}/';
  sub_filter_once off;

  server {
    listen 8443 ssl;
    server_name 127.0.0.1 localhost ${args.certs.hostname};

    ssl_certificate     /etc/tls/tls.crt;
    ssl_certificate_key /etc/tls/tls.key;

    location /uaa/ {
      proxy_pass http://${hostUaaService}.${this.namespace}.svc.cluster.local:8080;
      proxy_http_version 1.1;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-Proto https;
      proxy_set_header X-Forwarded-Port ${kindUaaNodePort};
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Host $host:${kindUaaNodePort};
      # sub_filter cannot rewrite gzipped bodies
      proxy_set_header Accept-Encoding "";
    }
  }
}
`,
				},
			},
			{
				parent: this,
				provider: args.provider,
				dependsOn: [ns],
			},
		);

		const proxyDeploy = new k8s.apps.v1.Deployment(
			`${name}-proxy`,
			{
				metadata: { name: "uaa-proxy", namespace: this.namespace },
				spec: {
					replicas: 1,
					selector: { matchLabels: { app: "uaa-proxy" } },
					template: {
						metadata: { labels: { app: "uaa-proxy" } },
						spec: {
							containers: [
								{
									name: "nginx",
									image: "mirror.gcr.io/library/nginx:1.27-alpine",
									ports: [{ containerPort: 8443, name: "https" }],
									volumeMounts: [
										{
											name: "config",
											mountPath: "/etc/nginx/nginx.conf",
											subPath: "nginx.conf",
											readOnly: true,
										},
										{
											name: "tls",
											mountPath: "/etc/tls",
											readOnly: true,
										},
									],
									readinessProbe: {
										tcpSocket: { port: 8443 },
										initialDelaySeconds: 5,
										periodSeconds: 5,
									},
								},
							],
							volumes: [
								{
									name: "config",
									configMap: { name: "uaa-proxy-config" },
								},
								{
									name: "tls",
									secret: { secretName: "uaa-proxy-tls" },
								},
							],
						},
					},
				},
			},
			{
				parent: this,
				provider: args.provider,
				dependsOn: [proxyConfig, proxyTls, virtualService],
			},
		);

		this.proxyService = new k8s.core.v1.Service(
			`${name}-proxy-svc`,
			{
				metadata: { name: "uaa-proxy", namespace: this.namespace },
				spec: {
					type: "NodePort",
					selector: { app: "uaa-proxy" },
					ports: [
						{
							name: "https",
							port: 8443,
							targetPort: 8443,
							nodePort: kindUaaNodePort,
						},
					],
				},
			},
			{
				parent: this,
				provider: args.provider,
				dependsOn: [proxyDeploy],
			},
		);

		this.registerOutputs({
			uaaUrl: this.uaaUrl,
			adminEmail: this.adminEmail,
			adminUserName: this.adminUserName,
			namespace: this.namespace,
		});
	}
}

function renderUaaYml(input: {
	issuerUri: string;
	adminEmail: string;
	adminPassword: string;
	signingKey: string;
	samlPrivateKey: string;
	samlCertificate: string;
}): string {
	const keyBlock = (text: string, spaces: number) =>
		text
			.trim()
			.split("\n")
			.map((line) => `${" ".repeat(spaces)}${line}`)
			.join("\n");

	return `spring_profiles: default,hsqldb

issuer:
  uri: ${input.issuerUri}

uaa:
  url: ${input.issuerUri}
  token:
    url: ${input.issuerUri}/oauth/token

jwt:
  token:
    policy:
      activeKeyId: uaa-key
      keys:
        uaa-key:
          signingKey: |
${keyBlock(input.signingKey, 12)}

login:
  url: ${input.issuerUri}
  entityBaseURL: ${input.issuerUri}
  entityID: cloudfoundry-saml-login
  saml:
    activeKeyId: uaa-saml-key
    keys:
      uaa-saml-key:
        key: |
${keyBlock(input.samlPrivateKey, 10)}
        passphrase: ""
        certificate: |
${keyBlock(input.samlCertificate, 10)}

oauth:
  clients:
    cf:
      id: cf
      override: true
      authorized-grant-types: password,refresh_token,authorization_code
      scope: openid,cloud_controller.read,cloud_controller.write,password.write,uaa.user,scim.me,scim.read,scim.write
      authorities: uaa.none
      secret: ""
      autoapprove: true
      redirect-uri: http://localhost/**
    cloud_controller:
      id: cloud_controller
      override: true
      authorized-grant-types: password,refresh_token,authorization_code
      scope: openid,cloud_controller.read,cloud_controller.write,password.write,uaa.user,scim.me
      authorities: uaa.none
      secret: ""
      autoapprove: true
      redirect-uri: http://localhost/**

scim:
  users:
    - ${input.adminEmail}|${input.adminPassword}|${input.adminEmail}|Admin|User|uaa.admin,scim.write,scim.read,scim.me,openid,cloud_controller.read,cloud_controller.write,password.write,uaa.user
`;
}
