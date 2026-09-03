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

test("eks stack creates OIDC-enabled cluster and EBS CSI", () => {
	expect(all).toContain("createOidcProvider: true");
	expect(all).toContain("aws-ebs-csi-driver");
	expect(all).toContain("AmazonEBSCSIDriverPolicy");
});

test("eks stack wires ECR IRSA and clears registry secrets via lib", () => {
	expect(all).toContain("EksRegistry");
	expect(all).toContain("EcrKpackIrsa");
	expect(all).toContain('platform: "eks"');
	expect(all).toContain("eksContainerRegistryRoleARN");
	expect(all).toContain("clusterType: \"EKS\"");
});

test("eks stack installs knative-runner Korifi and LoadBalancer Contour", () => {
	expect(all).toContain("KorifiRelease");
	expect(all).toContain("KorifiDependencies");
	expect(all).toContain("LoadBalancerService");
	expect(all).not.toContain("LocalRegistry");
	expect(all).not.toContain("kind create");
});

test("cf admin is a dedicated IAM user, not the cluster creator", () => {
	expect(all).toContain("cf-admin");
	expect(all).toContain("AccessEntry");
	expect(all).toContain("adminUserName");
});
