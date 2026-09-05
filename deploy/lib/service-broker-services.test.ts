import { expect, test } from "bun:test";
import * as fs from "node:fs";
import * as path from "node:path";

const text = fs.readFileSync(
	path.join(import.meta.dir, "service-broker-services.ts"),
	"utf8",
);

test("postgres backend is OpenEverest in a vcluster", () => {
	expect(text).toContain("EverestVcluster");
	expect(text).toContain("kindClusterName");
	expect(text).toContain("inClusterKubeconfig");
	expect(text).not.toContain('chart: "pg-operator"');
	expect(text).not.toContain("apps.v1.StatefulSet");
});
