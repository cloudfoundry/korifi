import { expect, test } from "bun:test";
import * as fs from "node:fs";
import * as path from "node:path";

const text = fs.readFileSync(path.join(import.meta.dir, "knative.ts"), "utf8");

test("knative component installs the operator helm chart and Serving CR", () => {
	expect(text).toContain("knative-operator");
	expect(text).toContain("https://knative.github.io/operator");
	expect(text).toContain('kind: "KnativeServing"');
	expect(text).toContain("operator.knative.dev/v1beta1");
	expect(text).toContain('"service-type": "ClusterIP"');
	expect(text).toContain("kourier.ingress.networking.knative.dev");
	expect(text).not.toContain("k0s");
});

test("dependencies Job skips Knative so Pulumi owns the operator", () => {
	const deps = fs.readFileSync(
		path.join(import.meta.dir, "dependencies.ts"),
		"utf8",
	);
	expect(deps).toContain('SKIP_KNATIVE');
	expect(deps).toContain("KnativeServing");
});

test("knative-runner RBAC and RunnerInfo are created for Korifi", () => {
	expect(text).toContain("korifi-knative-runner-appworkload-manager-role");
	expect(text).toContain("korifi-controllers-controller-manager");
	expect(text).toContain('kind: "RunnerInfo"');
	expect(text).toContain('runnerName: "knative-runner"');
});
