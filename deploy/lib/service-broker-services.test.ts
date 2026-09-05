import { expect, test } from "bun:test";
import * as fs from "node:fs";
import * as path from "node:path";

const text = fs.readFileSync(
	path.join(import.meta.dir, "service-broker-services.ts"),
	"utf8",
);

test("postgres backend serves TLS; broker admin uses sslmode=require", () => {
	expect(text).toContain("ssl=on");
	expect(text).toContain("ssl_cert_file=");
	expect(text).toContain("ssl_key_file=");
	expect(text).toContain("cert-manager.io/v1");
	expect(text).toContain('sslMode: "require"');
	expect(text).toContain("sslmode=require");
	expect(text).not.toContain('sslMode: "disable"');
});
