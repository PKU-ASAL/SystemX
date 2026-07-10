"use client";

import { useEffect, useMemo, useState } from "react";
import {
  CheckIcon,
  CopyIcon,
  PackagePlusIcon,
  RefreshCwIcon,
  ServerIcon,
  TerminalIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { createDefaultManagerApiClient, getManagerDataSource } from "@/lib/api";
import type { DeployAgentCommandResponse } from "@/lib/api/types";
import {
  createAgentInstallCommand,
  loadDeployOptions,
  mapDeployOptionsToView,
  parseManagementLabels,
  type DeployOptionsView,
} from "@/lib/deploy-data";

const managerApiClient = createDefaultManagerApiClient();
const managerDataSource = getManagerDataSource();

export function DeployPage() {
  const [options, setOptions] = useState<DeployOptionsView | null>(null);
  const [managementLabels, setManagementLabels] = useState("env=prod,role=web");
  const [command, setCommand] = useState<DeployAgentCommandResponse | null>(null);
  const [copied, setCopied] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isGenerating, setIsGenerating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();

    loadDeployOptions({
      client: managerApiClient,
      dataSource: managerDataSource,
      tenantId: "default",
      signal: controller.signal,
    })
      .then((nextOptions) => {
        setOptions(mapDeployOptionsToView(nextOptions));
      })
      .catch((nextError: unknown) => {
        if (controller.signal.aborted) return;
        setError(nextError instanceof Error ? nextError.message : "Failed to load deploy options");
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false);
      });

    return () => controller.abort();
  }, [reloadKey]);

  const linuxArtifact = useMemo(() => activeLinuxArtifact(options), [options]);
  const labels = parseManagementLabels(managementLabels);

  function reloadOptions() {
    setIsLoading(true);
    setError(null);
    setReloadKey((value) => value + 1);
  }

  async function generateCommand() {
    setIsGenerating(true);
    setError(null);
    setCopied(false);

    try {
      const nextCommand = await createAgentInstallCommand({
        client: managerApiClient,
        dataSource: managerDataSource,
        request: {
          tenant_id: options?.tenantId ?? "default",
          agent_id: generatedAgentID(),
          gateway_addr: options?.gatewayAddr || "127.0.0.1:19444",
          gateway_sni: options?.gatewaySNI,
          artifact_id: linuxArtifact?.id,
          ttl: "24h",
          labels,
        },
      });
      setCommand(nextCommand);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to generate command");
    } finally {
      setIsGenerating(false);
    }
  }

  async function copyCommand() {
    if (!command?.install_command) return;
    await navigator.clipboard?.writeText(command.install_command);
    setCopied(true);
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-bg">
      <header className="shrink-0 border-b bg-muted/10 p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <PackagePlusIcon className="size-5 text-muted-fg" />
            <div className="text-sm font-semibold">Deploy agent</div>
          </div>
          <Button intent="outline" size="sm" onPress={reloadOptions}>
            <RefreshCwIcon />
            Refresh
          </Button>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-auto p-4">
        <div className="grid gap-4 xl:grid-cols-3">
          <LinuxDeployCard
            command={command}
            copied={copied}
            error={error}
            isGenerating={isGenerating}
            isLoading={isLoading}
            labels={labels}
            linuxArtifact={linuxArtifact}
            managementLabels={managementLabels}
            onCopy={copyCommand}
            onGenerate={generateCommand}
            onLabelsChange={setManagementLabels}
          />
          <DisabledDeployCard
            title="Windows Agent"
            platform="windows"
            text="Windows installation is not enabled in this preview."
          />
          <DisabledDeployCard
            title="Container Runtime"
            platform="container"
            text="Container support will install on the host and collect events from running containers."
          />
        </div>
      </div>
    </section>
  );
}

function LinuxDeployCard({
  command,
  copied,
  error,
  isGenerating,
  isLoading,
  labels,
  linuxArtifact,
  managementLabels,
  onCopy,
  onGenerate,
  onLabelsChange,
}: {
  command: DeployAgentCommandResponse | null;
  copied: boolean;
  error: string | null;
  isGenerating: boolean;
  isLoading: boolean;
  labels: Record<string, string>;
  linuxArtifact?: DeployOptionsView["artifacts"][number];
  managementLabels: string;
  onCopy: () => void;
  onGenerate: () => void;
  onLabelsChange: (value: string) => void;
}) {
  return (
    <div className="flex min-h-[520px] flex-col rounded-lg border bg-bg">
      <div className="border-b p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className="font-semibold">Linux Agent</div>
            <div className="mt-1 text-sm text-muted-fg">Default host installation.</div>
          </div>
          <Badge>supported</Badge>
        </div>
      </div>

      <div className="grid gap-4 p-4">
        <div className="rounded-lg border bg-muted/10 p-3">
          <div className="mb-2 text-xs font-semibold uppercase text-muted-fg">Artifact</div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge>{linuxArtifact?.version ?? (isLoading ? "loading" : "not found")}</Badge>
            <span className="font-mono text-xs text-muted-fg">{linuxArtifact?.platform ?? "linux/amd64"}</span>
          </div>
          <div className="mt-2 truncate font-mono text-xs text-muted-fg">sha256: {linuxArtifact?.sha256 ?? "-"}</div>
        </div>

        <label className="grid gap-1 text-sm">
          <span className="font-medium text-muted-fg">Management labels</span>
          <Input
            placeholder="env=prod,role=web,owner=secops"
            value={managementLabels}
            onChange={(event) => onLabelsChange(event.target.value)}
          />
        </label>

        <div className="flex flex-wrap gap-1.5">
          {Object.entries(labels).length === 0 ? (
            <Badge>no labels</Badge>
          ) : (
            Object.entries(labels).map(([key, value]) => <Badge key={key}>{key}:{value}</Badge>)
          )}
        </div>
      </div>

      <div className="mt-auto border-t p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <Button isDisabled={!linuxArtifact || isGenerating} onPress={onGenerate}>
            <TerminalIcon />
            {isGenerating ? "Generating..." : "Generate command"}
          </Button>
          <Button intent="outline" size="sm" isDisabled={!command?.install_command} onPress={onCopy}>
            {copied ? <CheckIcon /> : <CopyIcon />}
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
        <pre className="min-h-28 overflow-auto rounded-lg border bg-muted/20 p-4 font-mono text-xs">
          {command?.install_command ?? "curl -fsSL ... | sudo bash"}
        </pre>
        {error && <div className="mt-3 rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm text-danger">{error}</div>}
      </div>
    </div>
  );
}

function DisabledDeployCard({
  title,
  platform,
  text,
}: {
  title: string;
  platform: string;
  text: string;
}) {
  return (
    <div className="flex min-h-[520px] flex-col rounded-lg border bg-muted/20 opacity-70">
      <div className="border-b p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className="font-semibold">{title}</div>
            <div className="mt-1 text-sm text-muted-fg">{text}</div>
          </div>
          <Badge>coming soon</Badge>
        </div>
      </div>
      <div className="grid flex-1 place-items-center p-6 text-center text-muted-fg">
        <div>
          <ServerIcon className="mx-auto mb-3 size-8" />
          <div className="font-mono text-sm">{platform}</div>
        </div>
      </div>
    </div>
  );
}

function activeLinuxArtifact(options: DeployOptionsView | null) {
  return options?.artifacts.find((artifact) => artifact.status === "active" && artifact.platform === "linux/amd64")
    ?? options?.artifacts.find((artifact) => artifact.status === "active" && artifact.platform.startsWith("linux/"));
}

function generatedAgentID() {
  return `agent-${Date.now().toString(36)}`;
}
