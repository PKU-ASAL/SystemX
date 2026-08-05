import { describe, expect, it, vi } from "vitest";

import type { ManagerApiClient } from "./api/client";
import {
  createAgentInstallCommand,
  loadDeployOptions,
  mapDeployOptionsToView,
  parseManagementLabels,
} from "./deploy-data";

describe("deploy data", () => {
  it("maps deploy options into install page view data", async () => {
    const client = {
      get: vi.fn(async () => ({
        tenant_id: "default",
        gateway_addr: "127.0.0.1:19444",
        supported_platforms: [{ os: "linux", arch: "amd64" }],
        artifacts: [
          {
            artifact_id: "art-linux-amd64",
            version: "0.8.0",
            os: "linux",
            arch: "amd64",
            sha256: "abc123",
            status: "active",
            download_url: "/api/v1/artifacts/art-linux-amd64/download",
            created_at: "2026-07-10T06:00:00Z",
          },
        ],
        enrollments: [
          {
            enrollment_id: "enr-existing",
            tenant_id: "default",
            agent_id: "agent-existing",
            status: "active",
            token_preview: "enr_...abcd",
            labels: { env: "prod" },
            expires_at: "2026-07-10T07:00:00Z",
          },
        ],
      })),
      post: vi.fn(),
    } satisfies ManagerApiClient;

    const options = await loadDeployOptions({ client, dataSource: "api" });
    const view = mapDeployOptionsToView(options);

    expect(view.artifacts[0]).toMatchObject({ id: "art-linux-amd64", platform: "linux/amd64" });
    expect(view.enrollments[0]).toMatchObject({ id: "enr-existing", labels: "env=prod" });
    expect(client.get).toHaveBeenCalledWith("/ui/deploy/options", expect.objectContaining({ query: { tenant_id: "default" } }));
  });

  it("creates an agent install command through the manager API", async () => {
    const client = {
      get: vi.fn(),
      post: vi.fn(async () => ({
        enrollment_id: "enr-new",
        token_expires_at: "2026-07-10T07:00:00Z",
        install_command: "curl -fsSL 'http://127.0.0.1:19443/api/v1/agent-install.sh?ticket=enr_x' | sudo bash",
        script_url: "http://127.0.0.1:19443/api/v1/agent-install.sh?ticket=enr_x",
        artifact: {
          artifact_id: "art-linux-amd64",
          download_url: "/api/v1/artifacts/art-linux-amd64/download",
          sha256: "abc123",
        },
      })),
    } satisfies ManagerApiClient;

    const command = await createAgentInstallCommand({
      client,
      dataSource: "api",
      request: {
        tenant_id: "default",
        agent_id: "agent-prod-001",
        host_id: "prod-api-01",
        gateway_addr: "127.0.0.1:19444",
        artifact_id: "art-linux-amd64",
        profile: "linux-container",
        channel: "linux-container-dev",
        ttl: "1h",
        labels: { env: "prod" },
      },
    });

    expect(command.install_command).toContain("agent-install.sh");
    expect(client.post).toHaveBeenCalledWith(
      "/ui/deploy/agent-command",
      expect.objectContaining({
        agent_id: "agent-prod-001",
        profile: "linux-container",
        channel: "linux-container-dev",
        labels: { env: "prod" },
      }),
      expect.any(Object),
    );
  });

  it("creates a mock container install command without sudo", async () => {
    const client = {
      get: vi.fn(),
      post: vi.fn(),
    } satisfies ManagerApiClient;

    const command = await createAgentInstallCommand({
      client,
      dataSource: "mock",
      request: {
        tenant_id: "default",
        agent_id: "agent-container-001",
        gateway_addr: "127.0.0.1:19444",
        profile: "linux-container",
        channel: "linux-container-dev",
      },
    });

    expect(command.install_command).toContain("| bash");
    expect(command.install_command).not.toContain("sudo bash");
    expect(command.entrypoint_command).toBe("/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent/agent.yaml");
  });

  it("parses management labels from comma separated key value pairs", () => {
    expect(parseManagementLabels("env=prod, role=api, empty= ")).toEqual({
      env: "prod",
      role: "api",
    });
  });
});
