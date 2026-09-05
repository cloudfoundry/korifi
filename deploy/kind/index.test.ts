import { expect, test } from "bun:test";
import * as fs from "node:fs";
import * as path from "node:path";

const dir = import.meta.dir;
const sources = fs
	.readdirSync(dir)
	.filter((name) => name.endsWith(".ts") && !name.endsWith(".test.ts"))
	.map((name) => ({
		name,
		text: fs.readFileSync(path.join(dir, name), "utf8"),
	}));
const all = sources.map((file) => file.text).join("\n");

test("kind stack uses kind, not k0s", () => {
	expect(all).toContain("kind create cluster");
	expect(all).not.toContain("k0s");
	expect(all).toContain("KindCluster");
});

test("kind stack composes shared lib components", () => {
	expect(all).toContain("KorifiDependencies");
	expect(all).toContain("KorifiRelease");
	expect(all).toContain("LocalRegistry");
	expect(all).toContain("ContourGateway");
	expect(all).toContain("ServiceBrokerServices");
	expect(all).toContain("UaaCerts");
	expect(all).toContain("UaaVcluster");
	expect(all).toContain("KnativeServing");
	expect(all).toContain("KindOsbBrokerImage");
	expect(all).toContain("OsbServiceBroker");
	expect(all).toContain("osbServicePath");
	expect(all).toContain("osb-service");
	expect(all).not.toContain("PostgresServiceBroker");
	expect(all).not.toContain("--insecure");
	expect(all).not.toContain('sslMode: "disable"');
	expect(all).toContain('platform: "kind"');
	expect(all).toContain("insecureTlsMetricsServer: true");
	expect(all).toContain("NodePortService");
	expect(all).toContain("uaaUrl");
	expect(all).toContain("oidc");
});

test("kind-config.yaml matches INSTALL.kind.md port mappings plus UAA", () => {
	const config = fs.readFileSync(path.join(dir, "kind-config.yaml"), "utf8");
	expect(config).toContain("containerPort: 32080");
	expect(config).toContain("hostPort: 80");
	expect(config).toContain("containerPort: 32443");
	expect(config).toContain("hostPort: 443");
	expect(config).toContain("containerPort: 30050");
	expect(config).toContain("containerPort: 30443");
	expect(config).toContain('config_path = "/etc/containerd/certs.d"');
});

test("knative-runner is the default run reconciler", () => {
	expect(all).toContain("knative-runner");
	expect(all).toContain("kindRegistryPrefix");
	expect(all).toContain("KnativeServing");
	expect(all).toContain("domain: appDomain");
	expect(all).toContain("localChart");
	expect(all).toContain("KindKorifiImages");
	expect(all).toContain("images.controllersImage");
	expect(all).not.toContain("cloudfoundry/korifi-controllers:latest");
});

test("auth flow uses cf login against UAA", () => {
	expect(all).toContain("cf login");
	expect(all).toContain("adminEmail");
	expect(all).not.toContain("cf auth kubernetes-admin");
});
