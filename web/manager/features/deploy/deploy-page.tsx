"use client";

import { useEffect, useMemo, useState } from "react";
import {
  CheckIcon,
  ContainerIcon,
  CopyIcon,
  HardDriveDownloadIcon,
  PackagePlusIcon,
  PanelRightOpenIcon,
  RefreshCwIcon,
  TerminalIcon,
  WrenchIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button, buttonStyles } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetBody,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { createDefaultManagerApiClient, getManagerDataSource } from "@/lib/api";
import type { DeployAgentCommandResponse } from "@/lib/api/types";
import {
  createAgentInstallCommand,
  loadDeployOptions,
  mapDeployOptionsToView,
  parseManagementLabels,
  type DeployOptionsView,
} from "@/lib/deploy-data";
import { cn } from "@/lib/utils";

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
        <div className="grid gap-4 xl:grid-cols-3 xl:items-stretch">
          <PlatformCard
            accent="primary"
            badge="default"
            icon={<HardDriveDownloadIcon />}
            title="Linux Agent"
            eyebrow="Host installation"
            description="Install the SysArmor agent as a managed systemd service and enroll the host into the manager."
            details={["mTLS enrollment", "systemd service", "event and signal collection"]}
            action={
              <LinuxInstallSheet
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
            }
          />
          <PlatformCard
            accent="muted"
            badge="coming soon"
            icon={<WrenchIcon />}
            title="Windows Agent"
            eyebrow="Endpoint installation"
            description="Prepare Windows host enrollment with the same policy and management label model."
            details={["MSI workflow planned", "service mode", "endpoint telemetry"]}
            disabled
          />
          <PlatformCard
            accent="container"
            badge="coming soon"
            icon={<ContainerIcon />}
            title="Container Runtime"
            eyebrow="Host-side container visibility"
            description="Install once on the host and collect event logs from every running container workload."
            details={["host install", "container events", "workload labels"]}
            disabled
          />
        </div>
      </div>
    </section>
  );
}

function PlatformCard({
  accent,
  action,
  badge,
  description,
  details,
  disabled = false,
  eyebrow,
  icon,
  title,
}: {
  accent: "primary" | "container" | "muted";
  action?: React.ReactNode;
  badge: string;
  description: string;
  details: string[];
  disabled?: boolean;
  eyebrow: string;
  icon: React.ReactNode;
  title: string;
}) {
  return (
    <div
      className={cn(
        "relative flex min-h-[460px] flex-col overflow-hidden rounded-lg border bg-bg p-5 shadow-sm",
        disabled && "bg-muted/20 text-muted-fg",
      )}
    >
      <div
        className={cn(
          "absolute inset-x-0 top-0 h-1",
          accent === "primary" && "bg-primary",
          accent === "container" && "bg-success",
          accent === "muted" && "bg-muted-fg/30",
        )}
      />
      <div className="flex items-start justify-between gap-4">
        <div
          className={cn(
            "grid size-11 place-items-center rounded-lg border bg-muted/20 text-muted-fg",
            accent === "primary" && "border-primary/30 bg-primary/10 text-primary",
            accent === "container" && "border-success/30 bg-success/10 text-success",
          )}
        >
          <div className="[&>svg]:size-5">{icon}</div>
        </div>
        <Badge>{badge}</Badge>
      </div>
      <div className="mt-6">
        <div className="text-xs font-semibold uppercase tracking-wide text-muted-fg">{eyebrow}</div>
        <div className="mt-2 text-xl font-semibold">{title}</div>
        <p className="mt-3 text-sm leading-6 text-muted-fg">{description}</p>
      </div>
      <div className="mt-6 grid gap-2">
        {details.map((detail) => (
          <div key={detail} className="flex items-center gap-2 text-sm">
            <span className="size-1.5 rounded-full bg-current opacity-50" />
            {detail}
          </div>
        ))}
      </div>
      <div className="mt-auto pt-6">
        {action ?? (
          <Button className="w-full" intent="outline" isDisabled>
            Not available
          </Button>
        )}
      </div>
    </div>
  );
}

function LinuxInstallSheet({
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
    <Sheet>
      <SheetTrigger className={cn(buttonStyles({ intent: "primary", size: "md" }), "w-full")}>
        <PanelRightOpenIcon />
        Install
      </SheetTrigger>
      <SheetContent className="sm:max-w-[540px]" aria-label="Install Linux agent">
        <SheetHeader>
          <SheetTitle>Install Linux Agent</SheetTitle>
        </SheetHeader>
        <SheetBody className="gap-4">
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

          <div className="flex flex-wrap items-center gap-2">
            <Button isDisabled={!linuxArtifact || isGenerating} onPress={onGenerate}>
              <TerminalIcon />
              {isGenerating ? "Generating..." : "Generate command"}
            </Button>
            <Button intent="outline" size="sm" isDisabled={!command?.install_command} onPress={onCopy}>
              {copied ? <CheckIcon /> : <CopyIcon />}
              {copied ? "Copied" : "Copy"}
            </Button>
          </div>

          <pre className="min-h-36 overflow-auto rounded-lg border bg-muted/20 p-4 font-mono text-xs">
            {command?.install_command ?? "curl -fsSL ... | sudo bash"}
          </pre>
          {error && <div className="rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm text-danger">{error}</div>}
        </SheetBody>
      </SheetContent>
    </Sheet>
  );
}

function activeLinuxArtifact(options: DeployOptionsView | null) {
  return options?.artifacts.find((artifact) => artifact.status === "active" && artifact.platform === "linux/amd64")
    ?? options?.artifacts.find((artifact) => artifact.status === "active" && artifact.platform.startsWith("linux/"));
}

function generatedAgentID() {
  return `agent-${Date.now().toString(36)}`;
}
