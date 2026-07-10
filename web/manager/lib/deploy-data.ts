import { generateAgentInstallCommand, getDeployOptions } from "./api/deploy";
import type { ManagerApiClient } from "./api/client";
import type {
  DeployAgentCommandRequest,
  DeployAgentCommandResponse,
  DeployArtifact,
  DeployEnrollment,
  DeployOptionsResponse,
} from "./api/types";

export type LoadDeployOptionsArgs = {
  client: ManagerApiClient;
  dataSource: "api" | "mock";
  tenantId?: string;
  signal?: AbortSignal;
};

export type CreateAgentInstallCommandArgs = {
  client: ManagerApiClient;
  dataSource: "api" | "mock";
  request: DeployAgentCommandRequest;
  signal?: AbortSignal;
};

export type DeployOptionsView = {
  tenantId: string;
  gatewayAddr: string;
  gatewaySNI: string;
  platforms: Array<{ id: string; label: string; os: string; arch: string }>;
  artifacts: Array<{
    id: string;
    version: string;
    platform: string;
    sha256: string;
    status: string;
    downloadUrl: string;
    createdAt: string;
  }>;
  enrollments: Array<{
    id: string;
    agent: string;
    tenant: string;
    status: string;
    labels: string;
    expiresAt: string;
    usedAt: string;
  }>;
};

export async function loadDeployOptions({
  client,
  dataSource,
  tenantId = "default",
  signal,
}: LoadDeployOptionsArgs) {
  if (dataSource === "mock") {
    return mockDeployOptions;
  }

  return getDeployOptions(client, { tenantId, signal });
}

export async function createAgentInstallCommand({
  client,
  dataSource,
  request,
  signal,
}: CreateAgentInstallCommandArgs) {
  if (dataSource === "mock") {
    return mockDeployCommand(request);
  }

  return generateAgentInstallCommand(client, request, { signal });
}

export function mapDeployOptionsToView(options: DeployOptionsResponse): DeployOptionsView {
  return {
    tenantId: options.tenant_id,
    gatewayAddr: options.gateway_addr,
    gatewaySNI: options.gateway_sni ?? "",
    platforms: options.supported_platforms.map((platform) => ({
      id: `${platform.os}/${platform.arch}`,
      label: `${platform.os}/${platform.arch}`,
      os: platform.os,
      arch: platform.arch,
    })),
    artifacts: options.artifacts.map(mapArtifact),
    enrollments: options.enrollments.map(mapEnrollment),
  };
}

export function parseManagementLabels(raw: string) {
  const labels: Record<string, string> = {};

  for (const token of raw.split(",")) {
    const [key, value] = token.split("=");
    if (key?.trim() && value?.trim()) {
      labels[key.trim()] = value.trim();
    }
  }

  return labels;
}

function mapArtifact(artifact: DeployArtifact) {
  return {
    id: artifact.artifact_id,
    version: artifact.version,
    platform: `${artifact.os}/${artifact.arch}`,
    sha256: artifact.sha256,
    status: artifact.status,
    downloadUrl: artifact.download_url,
    createdAt: displayValue(artifact.created_at),
  };
}

function mapEnrollment(enrollment: DeployEnrollment) {
  return {
    id: enrollment.enrollment_id,
    agent: displayValue(enrollment.agent_id),
    tenant: displayValue(enrollment.tenant_id),
    status: enrollment.status,
    labels: labelsToString(enrollment.labels),
    expiresAt: displayValue(enrollment.expires_at),
    usedAt: displayValue(enrollment.used_at),
  };
}

function labelsToString(labels?: Record<string, string>) {
  const entries = Object.entries(labels ?? {});
  if (entries.length === 0) return "-";

  return entries.map(([key, value]) => `${key}=${value}`).join(", ");
}

function displayValue(value?: string) {
  return value && value.trim() !== "" ? value : "-";
}

function mockDeployCommand(request: DeployAgentCommandRequest): DeployAgentCommandResponse {
  return {
    enrollment_id: "enr-mock",
    token_expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
    install_command:
      "curl -fsSL 'http://127.0.0.1:19443/api/v1/agent-install.sh?token=enr_mock' | sudo bash",
    script_url: "http://127.0.0.1:19443/api/v1/agent-install.sh?token=enr_mock",
    artifact: {
      artifact_id: request.artifact_id,
      download_url: "/api/v1/artifacts/art-linux-amd64/download",
      sha256: "mock-sha256",
    },
  };
}

const mockDeployOptions: DeployOptionsResponse = {
  tenant_id: "default",
  gateway_addr: "127.0.0.1:19444",
  gateway_sni: "",
  supported_platforms: [
    { os: "linux", arch: "amd64" },
    { os: "linux", arch: "arm64" },
  ],
  artifacts: [
    {
      artifact_id: "art-linux-amd64",
      version: "0.8.0",
      os: "linux",
      arch: "amd64",
      sha256: "mock-sha256",
      status: "active",
      download_url: "/api/v1/artifacts/art-linux-amd64/download",
      created_at: "2026-07-10T06:00:00Z",
    },
  ],
  enrollments: [],
};
