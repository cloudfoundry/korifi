import { expect, test } from "bun:test";
import * as fs from "node:fs";
import * as path from "node:path";
import {
	osbServiceBrokerGuid,
	osbServicePath,
} from "./osb-service-broker";

const text = fs.readFileSync(
	path.join(import.meta.dir, "osb-service-broker.ts"),
	"utf8",
);

test("osb-service broker serves HTTPS and registers CFServiceBroker", () => {
	expect(text).toContain("--tls-cert-file");
	expect(text).toContain("--tls-private-key-file");
	expect(text).toContain('scheme: "HTTPS"');
	expect(text).toContain("https://${fqdn}");
	expect(text).toContain("cert-manager.io/v1");
	expect(text).toContain("CFServiceBroker");
	expect(text).toContain(osbServiceBrokerGuid);
	expect(text).not.toContain("--insecure");
	expect(text).not.toContain("port: 8080");
	expect(text).toContain('postgres.sslMode ?? "require"');
});

test("image pull policy defaults to IfNotPresent, not Never", () => {
	expect(text).toContain('pullPolicy = args.imagePullPolicy ?? "IfNotPresent"');
	expect(text).not.toContain('imagePullPolicy: "Never"');
});

test("osbServicePath is the in-tree osb-service directory", () => {
	expect(osbServicePath("/cff/korifi/korifi")).toBe(
		"/cff/korifi/korifi/osb-service",
	);
});
