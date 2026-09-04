/**
 * TLS material for vcluster-hosted UAA + kind kube-apiserver OIDC.
 *
 * The CA PEM must exist on the host before `kind create` so the control-plane
 * can mount it as oidc-ca-file. The server cert is reused by Contour/Gateway
 * for HTTPS terminate on the UAA hostname on the kind host cluster.
 */
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import * as command from "@pulumi/command";
import * as pulumi from "@pulumi/pulumi";
import * as tls from "@pulumi/tls";

export interface UaaCertsArgs {
	/** DNS name on the certificate (e.g. uaa.127.0.0.1.nip.io). */
	hostname: string;
	/**
	 * Directory for ca.pem / tls.crt / tls.key.
	 * Defaults to ~/.korifi/kind-uaa/<hostname>.
	 */
	outputDir?: string;
}

export class UaaCerts extends pulumi.ComponentResource {
	readonly hostname: string;
	readonly outputDir: string;
	readonly caPemPath: string;
	readonly certPemPath: string;
	readonly keyPemPath: string;
	readonly caCertPem: pulumi.Output<string>;
	readonly certPem: pulumi.Output<string>;
	readonly privateKeyPem: pulumi.Output<string>;
	/** Completes once PEMs are written to disk for kind extraMounts. */
	readonly filesReady: command.local.Command;

	constructor(
		name: string,
		args: UaaCertsArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:UaaCerts", name, {}, opts);

		this.hostname = args.hostname;
		this.outputDir =
			args.outputDir ??
			path.join(os.homedir(), ".korifi", "kind-uaa", args.hostname);
		this.caPemPath = path.join(this.outputDir, "ca.pem");
		this.certPemPath = path.join(this.outputDir, "tls.crt");
		this.keyPemPath = path.join(this.outputDir, "tls.key");

		const caKey = new tls.PrivateKey(
			`${name}-ca-key`,
			{ algorithm: "RSA", rsaBits: 2048 },
			{ parent: this },
		);
		const caCert = new tls.SelfSignedCert(
			`${name}-ca`,
			{
				privateKeyPem: caKey.privateKeyPem,
				validityPeriodHours: 24 * 365 * 5,
				isCaCertificate: true,
				allowedUses: ["cert_signing", "client_auth", "server_auth"],
				subject: { commonName: `korifi-uaa-ca-${args.hostname}` },
			},
			{ parent: this },
		);

		const serverKey = new tls.PrivateKey(
			`${name}-server-key`,
			{ algorithm: "RSA", rsaBits: 2048 },
			{ parent: this },
		);
		const serverCert = new tls.CertRequest(
			`${name}-server-csr`,
			{
				privateKeyPem: serverKey.privateKeyPem,
				subject: { commonName: args.hostname },
				dnsNames: [args.hostname, "localhost"],
				ipAddresses: ["127.0.0.1"],
			},
			{ parent: this },
		);
		const signed = new tls.LocallySignedCert(
			`${name}-server`,
			{
				certRequestPem: serverCert.certRequestPem,
				caPrivateKeyPem: caKey.privateKeyPem,
				caCertPem: caCert.certPem,
				validityPeriodHours: 24 * 365 * 2,
				allowedUses: ["server_auth", "client_auth"],
			},
			{ parent: this },
		);

		this.caCertPem = caCert.certPem;
		this.certPem = signed.certPem;
		this.privateKeyPem = serverKey.privateKeyPem;

		this.filesReady = new command.local.Command(
			`${name}-write-pems`,
			{
				create: pulumi
					.all([caCert.certPem, signed.certPem, serverKey.privateKeyPem])
					.apply(([caPem, certPem, keyPem]) => {
						fs.mkdirSync(this.outputDir, { recursive: true });
						fs.writeFileSync(this.caPemPath, caPem, { mode: 0o644 });
						fs.writeFileSync(this.certPemPath, certPem, { mode: 0o644 });
						fs.writeFileSync(this.keyPemPath, keyPem, { mode: 0o600 });
						return `mkdir -p '${this.outputDir}' && test -f '${this.caPemPath}' && echo ok`;
					}),
				delete: `rm -rf '${this.outputDir}'`,
			},
			{ parent: this },
		);

		this.registerOutputs({
			hostname: this.hostname,
			outputDir: this.outputDir,
			caPemPath: this.caPemPath,
		});
	}
}
