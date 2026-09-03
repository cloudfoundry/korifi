import { expect, test } from "bun:test";
import * as fs from "node:fs";

const dir = import.meta.dir;
const sources = fs
	.readdirSync(dir)
	.filter((name) => name.endsWith(".ts") && !name.endsWith(".test.ts"))
	.map((name) => ({
		name,
		text: fs.readFileSync(`${dir}/${name}`, "utf8"),
	}));
const all = sources.map((file) => file.text).join("\n");

test("gke stack creates GKE cluster with workload identity", () => {
	expect(all).toContain("GkeCluster");
	expect(all).toContain("workloadIdentityConfig");
	expect(all).toContain("gke-gcloud-auth-plugin");
});

test("gke stack uses Artifact Registry with _json_key pull secret", () => {
	expect(all).toContain("GkeRegistry");
	expect(all).toContain("artifactregistry");
	expect(all).toContain("_json_key");
	expect(all).toContain("registryPullSecret");
	expect(all).toContain('platform: "gke"');
});

test("gke stack composes shared lib and LoadBalancer Contour", () => {
	expect(all).toContain("KorifiDependencies");
	expect(all).toContain("KorifiRelease");
	expect(all).toContain("LoadBalancerService");
	expect(all).toContain('clusterType: "GKE"');
	expect(all).not.toContain("LocalRegistry");
	expect(all).not.toContain("kind create");
	expect(all).not.toContain("eksContainerRegistryRoleARN");
});

test("dns guidance uses A records for GKE IPs", () => {
	const index = sources.find((file) => file.name === "index.ts")?.text ?? "";
	expect(index).toContain("A records");
});
