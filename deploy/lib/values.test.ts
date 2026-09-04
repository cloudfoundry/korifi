/**
 * Unit tests for pure value builders — no cluster, no Pulumi engine.
 */
import { describe, expect, test } from "bun:test";
import {
	buildKorifiValues,
	eksKpackBuilderRepository,
	eksRepositoryPrefix,
	gkeKpackBuilderRepository,
	gkeRepositoryPrefix,
	kindGatewayPorts,
	kindKpackBuilderRepository,
	kindRegistryPrefix,
} from "./values";
import { korifiChartUrl, versions } from "./versions";

describe("buildKorifiValues", () => {
	test("kind mirrors install-korifi-kind.yaml", () => {
		const values = buildKorifiValues({
			platform: "kind",
			adminUserName: "kubernetes-admin",
			apiUrl: "localhost",
			appDomain: "apps-127-0-0-1.nip.io",
			containerRepositoryPrefix: kindRegistryPrefix(),
			kpackBuilderRepository: kindKpackBuilderRepository(),
			networking: { gatewayPorts: kindGatewayPorts },
		});

		expect(values.adminUserName).toBe("kubernetes-admin");
		expect(values.defaultAppDomainName).toBe("apps-127-0-0-1.nip.io");
		expect(values.generateIngressCertificates).toBe(true);
		expect(values.logLevel).toBe("debug");
		expect(values.knativeRunner).toEqual({ include: true });
		expect(values.reconcilers).toEqual({ run: "knative-runner" });
		expect(values.networking).toEqual({
			gatewayClass: "contour",
			gatewayNamespace: "korifi-gateway",
			gatewayPorts: { http: 32080, https: 32443 },
		});
		expect(values.containerRepositoryPrefix).toBe(kindRegistryPrefix());
		expect(values.experimental).toEqual({
			managedServices: { enabled: true, trustInsecureBrokers: true },
		});
		expect(values.eksContainerRegistryRoleARN).toBeUndefined();
	});

	test("kind pins locally built Korifi images over Hub latest", () => {
		const values = buildKorifiValues({
			platform: "kind",
			adminUserName: "kubernetes-admin",
			apiUrl: "localhost",
			appDomain: "apps-127-0-0-1.nip.io",
			containerRepositoryPrefix: kindRegistryPrefix(),
			kpackBuilderRepository: kindKpackBuilderRepository(),
			networking: { gatewayPorts: kindGatewayPorts },
			images: {
				controllers: "korifi-controllers:kind-abc",
				api: "korifi-api:kind-abc",
				migration: "korifi-migration:kind-abc",
			},
		});

		expect(values.controllers).toEqual({
			taskTTL: "5s",
			image: "korifi-controllers:kind-abc",
			imagePullPolicy: "IfNotPresent",
		});
		expect(values.api).toEqual({
			apiServer: { url: "localhost" },
			image: "korifi-api:kind-abc",
			imagePullPolicy: "IfNotPresent",
		});
		expect(values.migration).toEqual({
			image: "korifi-migration:kind-abc",
			imagePullPolicy: "IfNotPresent",
		});
	});

	test("eks clears registry secrets and requires IRSA role ARN", () => {
		const values = buildKorifiValues({
			platform: "eks",
			adminUserName: "cf-admin",
			apiUrl: "api.korifi.example.org",
			appDomain: "apps.korifi.example.org",
			containerRepositoryPrefix: eksRepositoryPrefix(
				"123456789012",
				"us-west-1",
				"my-cluster",
			),
			kpackBuilderRepository: eksKpackBuilderRepository(
				"123456789012",
				"us-west-1",
				"my-cluster",
			),
			eksContainerRegistryRoleARN:
				"arn:aws:iam::123456789012:role/korifi-ecr-service-account-role",
		});

		expect(values.containerRegistrySecrets).toEqual({});
		expect(values.eksContainerRegistryRoleARN).toBe(
			"arn:aws:iam::123456789012:role/korifi-ecr-service-account-role",
		);
		expect(values.knativeRunner).toEqual({ include: true });
		expect(values.reconcilers).toEqual({ run: "knative-runner" });
		expect(values.logLevel).toBeUndefined();
		expect(values.experimental).toBeUndefined();
	});

	test("eks throws without role ARN", () => {
		expect(() =>
			buildKorifiValues({
				platform: "eks",
				adminUserName: "cf-admin",
				apiUrl: "api.example.org",
				appDomain: "apps.example.org",
				containerRepositoryPrefix: "prefix/",
				kpackBuilderRepository: "prefix/kpack-builder",
			}),
		).toThrow(/eksContainerRegistryRoleARN/);
	});

	test("gke uses Artifact Registry prefix and keeps pull secrets", () => {
		const prefix = gkeRepositoryPrefix(
			"europe",
			"my-project",
			"korifi",
		);
		const values = buildKorifiValues({
			platform: "gke",
			adminUserName: "cf-admin",
			apiUrl: "api.korifi.example.org",
			appDomain: "apps.korifi.example.org",
			containerRepositoryPrefix: prefix,
			kpackBuilderRepository: gkeKpackBuilderRepository(
				"europe",
				"my-project",
				"korifi",
			),
		});

		expect(values.containerRepositoryPrefix).toBe(
			"europe-docker.pkg.dev/my-project/korifi/",
		);
		expect(values.containerRegistrySecrets).toBeUndefined();
		expect(values.eksContainerRegistryRoleARN).toBeUndefined();
		expect(values.knativeRunner).toEqual({ include: true });
	});

	test("extraValues deep-merge overrides nested keys", () => {
		const values = buildKorifiValues({
			platform: "gke",
			adminUserName: "cf-admin",
			apiUrl: "api.example.org",
			appDomain: "apps.example.org",
			containerRepositoryPrefix: "prefix/",
			kpackBuilderRepository: "prefix/kpack-builder",
			extraValues: {
				api: { apiServer: { url: "api.override.org", port: 8443 } },
				logLevel: "debug",
			},
		});

		expect(values.api).toEqual({
			apiServer: { url: "api.override.org", port: 8443 },
		});
		expect(values.logLevel).toBe("debug");
	});
});

describe("repository helpers", () => {
	test("eks prefix matches INSTALL.EKS.md", () => {
		expect(eksRepositoryPrefix("111122223333", "eu-west-1", "prod")).toBe(
			"111122223333.dkr.ecr.eu-west-1.amazonaws.com/prod/",
		);
		expect(
			eksKpackBuilderRepository("111122223333", "eu-west-1", "prod"),
		).toBe(
			"111122223333.dkr.ecr.eu-west-1.amazonaws.com/prod/kpack-builder",
		);
	});

	test("gke prefix matches INSTALL.md Artifact Registry table", () => {
		expect(gkeRepositoryPrefix("us", "proj", "korifi")).toBe(
			"us-docker.pkg.dev/proj/korifi/",
		);
	});
});

describe("versions", () => {
	test("chart URL and installer stay on the same release", () => {
		expect(korifiChartUrl()).toBe(
			`https://github.com/cloudfoundry/korifi/releases/download/v${versions.korifi}/korifi-${versions.korifi}.tgz`,
		);
		expect(versions.korifiInstallerImage).toContain("korifi-installer");
		expect(versions.knativeServing).toMatch(/^\d+\.\d+\.\d+$/);
		expect(versions.knativeOperatorChart).toMatch(/^v\d+\.\d+\.\d+$/);
	});
});
