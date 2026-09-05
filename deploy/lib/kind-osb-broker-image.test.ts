import { expect, test } from "bun:test";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { hashOsbBrokerImageSources } from "./image-source-hash";

const text = fs.readFileSync(
	path.join(import.meta.dir, "kind-osb-broker-image.ts"),
	"utf8",
);

test("kind OSB image builds the osb-service Dockerfile and kind-loads it", () => {
	expect(text).toContain("docker build -f image/Dockerfile");
	expect(text).toContain("kind load docker-image");
	expect(text).toContain("osb-service:");
	expect(text).toContain("osb-service sources not found");
});

test("hashOsbBrokerImageSources changes when broker sources change", () => {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "osb-img-hash-"));
	try {
		fs.writeFileSync(path.join(root, "go.mod"), "module example\n");
		fs.writeFileSync(path.join(root, "go.sum"), "\n");
		fs.mkdirSync(path.join(root, "cmd"));
		fs.mkdirSync(path.join(root, "pkg"));
		fs.mkdirSync(path.join(root, "image"));
		fs.writeFileSync(
			path.join(root, "image", "Dockerfile"),
			"FROM scratch\n",
		);
		const a = hashOsbBrokerImageSources(root);
		fs.writeFileSync(
			path.join(root, "image", "Dockerfile"),
			"FROM scratch\n# change\n",
		);
		const b = hashOsbBrokerImageSources(root);
		expect(a).toHaveLength(64);
		expect(b).toHaveLength(64);
		expect(a).not.toBe(b);
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});
