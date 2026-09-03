/**
 * Stack configuration for deploy/eks (INSTALL.EKS.md).
 */
import * as pulumi from "@pulumi/pulumi";
import { versions } from "@korifi/deploy-lib";

const cfg = new pulumi.Config();
const awsCfg = new pulumi.Config("aws");

export const clusterName = cfg.require("clusterName");
export const region = awsCfg.get("region") ?? cfg.get("region") ?? "us-west-1";
export const baseDomain = cfg.require("baseDomain");
export const adminUserName = cfg.get("adminUserName") ?? "cf-admin";
export const rootNamespace = cfg.get("rootNamespace") ?? "cf";
export const korifiNamespace = cfg.get("korifiNamespace") ?? "korifi";
export const gatewayNamespace = cfg.get("gatewayNamespace") ?? "korifi-gateway";
export const gatewayClassName = cfg.get("gatewayClassName") ?? "contour";

export const apiUrl = cfg.get("apiUrl") ?? `api.${baseDomain}`;
export const appDomain = cfg.get("appDomain") ?? `apps.${baseDomain}`;

export const nodeInstanceType = cfg.get("nodeInstanceType") ?? "m5.xlarge";
export const desiredCapacity = cfg.getNumber("desiredCapacity") ?? 3;

export const pinned = {
	korifi: cfg.get("korifiVersion") ?? versions.korifi,
	installerImage: cfg.get("installerImage") ?? versions.korifiInstallerImage,
} as const;
