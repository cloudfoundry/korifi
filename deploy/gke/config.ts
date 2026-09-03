/**
 * Stack configuration for deploy/gke (INSTALL.md on GKE + Artifact Registry).
 */
import * as pulumi from "@pulumi/pulumi";
import { versions } from "@korifi/deploy-lib";

const cfg = new pulumi.Config();
const gcpCfg = new pulumi.Config("gcp");

export const project = gcpCfg.require("project");
export const region = gcpCfg.get("region") ?? cfg.get("region") ?? "us-central1";
export const location = cfg.get("location") ?? region;
export const clusterName = cfg.require("clusterName");
export const baseDomain = cfg.require("baseDomain");

export const adminUserName = cfg.get("adminUserName") ?? "cf-admin";
export const rootNamespace = cfg.get("rootNamespace") ?? "cf";
export const korifiNamespace = cfg.get("korifiNamespace") ?? "korifi";
export const gatewayNamespace = cfg.get("gatewayNamespace") ?? "korifi-gateway";
export const gatewayClassName = cfg.get("gatewayClassName") ?? "contour";

export const apiUrl = cfg.get("apiUrl") ?? `api.${baseDomain}`;
export const appDomain = cfg.get("appDomain") ?? `apps.${baseDomain}`;

/** Artifact Registry repository id (must exist as a Docker repo). */
export const artifactRegistryRepo =
	cfg.get("artifactRegistryRepo") ?? "korifi";

/**
 * Registry region host prefix, e.g. `us` or `europe` for
 * `<region>-docker.pkg.dev`. Defaults to the first segment of `region`.
 */
export const artifactRegistryLocation =
	cfg.get("artifactRegistryLocation") ?? region.split("-")[0]!;

export const nodeMachineType = cfg.get("nodeMachineType") ?? "e2-standard-4";
export const nodeCount = cfg.getNumber("nodeCount") ?? 3;

export const pinned = {
	korifi: cfg.get("korifiVersion") ?? versions.korifi,
	installerImage: cfg.get("installerImage") ?? versions.korifiInstallerImage,
} as const;
