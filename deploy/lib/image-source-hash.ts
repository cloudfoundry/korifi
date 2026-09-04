/**
 * Stable digest of the Go/Docker sources that go into Korifi images.
 * Kept free of Pulumi so unit tests can import it without the engine.
 */
import * as crypto from "node:crypto";
import * as fs from "node:fs";
import * as path from "node:path";

export const korifiImageSourceEntries = [
	"go.mod",
	"go.sum",
	"controllers/Dockerfile",
	"api/Dockerfile",
	"migration/Dockerfile",
	"controllers",
	"knative-runner",
	"api",
	"tools",
	"version",
	"kpack-image-builder",
	"job-task-runner",
	"statefulset-runner",
	"migration",
] as const;

export function hashKorifiImageSources(repoRoot: string): string {
	const hash = crypto.createHash("sha256");
	const files: string[] = [];
	for (const entry of korifiImageSourceEntries) {
		collectFiles(path.join(repoRoot, entry), repoRoot, files);
	}
	files.sort();
	for (const rel of files) {
		hash.update(rel);
		hash.update("\0");
		hash.update(fs.readFileSync(path.join(repoRoot, rel)));
		hash.update("\0");
	}
	return hash.digest("hex");
}

function collectFiles(abs: string, repoRoot: string, out: string[]): void {
	if (!fs.existsSync(abs)) {
		return;
	}
	const stat = fs.statSync(abs);
	if (stat.isFile()) {
		out.push(path.relative(repoRoot, abs));
		return;
	}
	if (!stat.isDirectory()) {
		return;
	}
	const base = path.basename(abs);
	if (base === "bin" || base === "node_modules" || base === ".git") {
		return;
	}
	for (const name of fs.readdirSync(abs)) {
		collectFiles(path.join(abs, name), repoRoot, out);
	}
}
