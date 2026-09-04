import { expect, test } from "bun:test";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { hashKorifiImageSources } from "./image-source-hash";

const text = fs.readFileSync(path.join(import.meta.dir, "kind-images.ts"), "utf8");

test("kind images build controllers, api, and migration then kind-load them", () => {
	expect(text).toContain("docker build -f controllers/Dockerfile");
	expect(text).toContain("docker build -f api/Dockerfile");
	expect(text).toContain("docker build -f migration/Dockerfile");
	expect(text).toContain("kind load docker-image");
	expect(text).toContain("korifi-controllers:");
	expect(text).not.toContain("cloudfoundry/korifi-controllers:latest");
});

test("hashKorifiImageSources changes when a source file changes", () => {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "korifi-img-hash-"));
	try {
		fs.writeFileSync(path.join(root, "go.mod"), "module example\n");
		fs.mkdirSync(path.join(root, "controllers"));
		fs.writeFileSync(
			path.join(root, "controllers", "Dockerfile"),
			"FROM scratch\n",
		);
		const a = hashKorifiImageSources(root);
		fs.writeFileSync(
			path.join(root, "controllers", "Dockerfile"),
			"FROM scratch\n# change\n",
		);
		const b = hashKorifiImageSources(root);
		expect(a).toHaveLength(64);
		expect(b).toHaveLength(64);
		expect(a).not.toBe(b);
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});
