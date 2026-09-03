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
	expect(all).toContain('platform: "kind"');
	expect(all).toContain("insecureTlsMetricsServer: true");
	expect(all).toContain("NodePortService");
});

test("kind-config.yaml matches INSTALL.kind.md port mappings", () => {
	const config = fs.readFileSync(path.join(dir, "kind-config.yaml"), "utf8");
	expect(config).toContain("containerPort: 32080");
	expect(config).toContain("hostPort: 80");
	expect(config).toContain("containerPort: 32443");
	expect(config).toContain("hostPort: 443");
	expect(config).toContain("containerPort: 30050");
	expect(config).toContain('config_path = "/etc/containerd/certs.d"');
});

test("knative-runner is the default run reconciler", () => {
	expect(all).toContain("knative-runner");
	expect(all).toContain("kindRegistryPrefix");
});
