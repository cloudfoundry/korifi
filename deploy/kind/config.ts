/**
 * Stack configuration for deploy/kind (INSTALL.kind.md).
 */
import * as os from "node:os";
import * as path from "node:path";
import * as pulumi from "@pulumi/pulumi";
import { versions } from "@korifi/deploy-lib";

const cfg = new pulumi.Config();

export const clusterName = cfg.get("clusterName") ?? "korifi";
export const appDomain = cfg.get("appDomain") ?? "apps-127-0-0-1.nip.io";
export const apiUrl = cfg.get("apiUrl") ?? "localhost";
/** UAA admin email (OIDC user_name / CF login username). */
export const adminEmail = cfg.get("adminEmail") ?? "admin@korifi.local";
/** OIDC username prefix baked into kind apiserver + Korifi adminUserName. */
export const oidcPrefix = cfg.get("oidcPrefix") ?? "uaa";
export const registryUser = cfg.get("registryUser") ?? "user";

export const kubeconfigPath =
	cfg.get("kubeconfigPath") ??
	path.join(os.homedir(), ".kube", `kind-${clusterName}.config`);

export const pinned = {
	korifi: cfg.get("korifiVersion") ?? versions.korifi,
	installerImage: cfg.get("installerImage") ?? versions.korifiInstallerImage,
} as const;
