"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  BoxesIcon,
  CheckIcon,
  ContainerIcon,
  CopyIcon,
  MonitorIcon,
  PackagePlusIcon,
  PanelRightOpenIcon,
  RefreshCwIcon,
  ServerIcon,
  TerminalIcon,
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

type DeploymentModeID = "linux-host" | "linux-container" | "kubernetes-node" | "windows-host";

type DeploymentMode = {
  id: DeploymentModeID;
  title: string;
  subtitle: string;
  status: "Available" | "Coming soon";
  description: string;
  scope: string;
  runtime: string;
  bestFor: string;
  profile: "linux-systemd" | "linux-container" | "kubernetes-node" | "windows-service";
  channel: string;
  icon: ReactNode;
  accent: "primary" | "container" | "muted";
};

const deploymentModes: DeploymentMode[] = [
  {
    id: "linux-host",
    title: "Linux Host",
    subtitle: "Agent runs on a Linux host",
    status: "Available",
    description: "Collect host-level endpoint events from bare metal, VM, or cloud instances.",
    scope: "Host",
    runtime: "systemd",
    bestFor: "Servers and VM endpoints",
    profile: "linux-systemd",
    channel: "linux-systemd-dev",
    icon: <ServerIcon />,
    accent: "primary",
  },
  {
    id: "linux-container",
    title: "Linux Container",
    subtitle: "Agent runs inside a Linux container",
    status: "Available",
    description: "Collect only the current container namespace with a container entrypoint.",
    scope: "Current namespace",
    runtime: "entrypoint",
    bestFor: "Container-scoped workloads",
    profile: "linux-container",
    channel: "linux-container-dev",
    icon: <ContainerIcon />,
    accent: "container",
  },
  {
    id: "kubernetes-node",
    title: "Kubernetes Node",
    subtitle: "Agent runs on each Kubernetes node",
    status: "Coming soon",
    description: "Collect node and workload events through a privileged DaemonSet.",
    scope: "Node + workloads",
    runtime: "DaemonSet",
    bestFor: "Kubernetes clusters",
    profile: "kubernetes-node",
    channel: "kubernetes-node-dev",
    icon: <BoxesIcon />,
    accent: "muted",
  },
  {
    id: "windows-host",
    title: "Windows Host",
    subtitle: "Agent runs on a Windows host",
    status: "Coming soon",
    description: "Collect endpoint telemetry through a Windows service.",
    scope: "Host",
    runtime: "Windows service",
    bestFor: "Windows servers and workstations",
    profile: "windows-service",
    channel: "windows-host-dev",
    icon: <MonitorIcon />,
    accent: "muted",
  },
];

export function DeployPage() {
  const [options, setOptions] = useState<DeployOptionsView | null>(null);
  const [managementLabels, setManagementLabels] = useState("env=prod,role=web");
  const [command, setCommand] = useState<{
    modeID: DeploymentModeID;
    response: DeployAgentCommandResponse;
  } | null>(null);
  const [copied, setCopied] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [generatingModeID, setGeneratingModeID] = useState<DeploymentModeID | null>(null);
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

  async function generateCommand(mode: DeploymentMode) {
    setGeneratingModeID(mode.id);
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
          profile: mode.profile,
          channel: mode.channel,
          ttl: "24h",
          labels,
        },
      });
      setCommand({ modeID: mode.id, response: nextCommand });
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "Failed to generate command");
    } finally {
      setGeneratingModeID(null);
    }
  }

  async function copyCommand() {
    if (!command?.response.install_command) return;
    await navigator.clipboard?.writeText(command.response.install_command);
    setCopied(true);
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-bg">
      <header className="shrink-0 border-b bg-bg px-5 py-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <div className="grid size-8 place-items-center rounded-md border bg-muted/20 text-muted-fg">
              <PackagePlusIcon className="size-4" />
            </div>
            <div>
              <div className="text-sm font-semibold">Deploy agent</div>
              <div className="text-xs text-muted-fg">Choose a deployment target and generate an enrollment command.</div>
            </div>
          </div>
          <Button intent="outline" size="sm" onPress={reloadOptions}>
            <RefreshCwIcon />
            Refresh
          </Button>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-auto bg-muted/5 p-5">
        <div className="grid max-w-7xl gap-4 lg:grid-cols-2 2xl:grid-cols-4 2xl:items-stretch">
          {deploymentModes.map((mode) => {
            const available = mode.status === "Available";
            return (
              <DeploymentModeCard
                key={mode.id}
                mode={mode}
                action={
                  available ? (
                    <InstallCommandSheet
                      command={command?.modeID === mode.id ? command.response : null}
                      copied={copied && command?.modeID === mode.id}
                      error={error}
                      isGenerateDisabled={generatingModeID !== null}
                      isGenerating={generatingModeID === mode.id}
                      isLoading={isLoading}
                      labels={labels}
                      linuxArtifact={linuxArtifact}
                      managementLabels={managementLabels}
                      mode={mode}
                      onCopy={copyCommand}
                      onGenerate={() => generateCommand(mode)}
                      onLabelsChange={setManagementLabels}
                    />
                  ) : undefined
                }
              />
            );
          })}
        </div>
      </div>
    </section>
  );
}

function DeploymentModeCard({
  action,
  mode,
}: {
  action?: ReactNode;
  mode: DeploymentMode;
}) {
  const disabled = mode.status !== "Available";
  return (
    <div
      className={cn(
        "group flex min-h-[340px] flex-col rounded-lg border bg-bg p-4 transition-colors",
        !disabled && "hover:border-muted-fg/35 hover:bg-muted/10",
        disabled && "bg-muted/15 text-muted-fg",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div
          className={cn(
            "grid size-9 place-items-center rounded-md border bg-bg text-muted-fg",
            mode.accent === "primary" && "border-primary/20 text-primary",
            mode.accent === "container" && "border-success/20 text-success",
          )}
        >
          <div className="[&>svg]:size-5">{mode.icon}</div>
        </div>
        <Badge>{mode.status}</Badge>
      </div>
      <div className="mt-5">
        <div className="text-xs font-medium text-muted-fg">{mode.subtitle}</div>
        <div className="mt-1 text-lg font-semibold">{mode.title}</div>
        <p className="mt-3 text-sm leading-6 text-muted-fg">{mode.description}</p>
      </div>
      <div className="mt-5 grid gap-2">
        {[
          ["Scope", mode.scope],
          ["Runtime", mode.runtime],
          ["Best for", mode.bestFor],
        ].map(([label, value]) => (
          <div key={label} className="flex items-center justify-between gap-3 text-sm text-muted-fg">
            <span>{label}</span>
            <span className="text-right font-medium text-fg">{value}</span>
          </div>
        ))}
      </div>
      <div className="mt-4 flex flex-wrap gap-1.5">
        {[mode.profile, mode.channel].map((detail) => (
          <div key={detail} className="flex items-center gap-1.5 text-xs text-muted-fg">
            <span className="size-1 rounded-full bg-current opacity-60" />
            {detail}
          </div>
        ))}
      </div>
      <div className="mt-auto pt-5">
        {action ?? (
          <Button className="w-full" intent="outline" isDisabled>
            Not available
          </Button>
        )}
      </div>
    </div>
  );
}

function InstallCommandSheet({
  command,
  copied,
  error,
  isGenerateDisabled,
  isGenerating,
  isLoading,
  labels,
  linuxArtifact,
  managementLabels,
  mode,
  onCopy,
  onGenerate,
  onLabelsChange,
}: {
  command: DeployAgentCommandResponse | null;
  copied: boolean;
  error: string | null;
  isGenerateDisabled: boolean;
  isGenerating: boolean;
  isLoading: boolean;
  labels: Record<string, string>;
  linuxArtifact?: DeployOptionsView["artifacts"][number];
  managementLabels: string;
  mode: DeploymentMode;
  onCopy: () => void;
  onGenerate: () => void;
  onLabelsChange: (value: string) => void;
}) {
  return (
    <Sheet>
      <SheetTrigger className={cn(buttonStyles({ intent: "primary", size: "md" }), "w-full")}>
        <PanelRightOpenIcon />
        Generate command
      </SheetTrigger>
      <SheetContent className="sm:max-w-[560px]" aria-label={`Install ${mode.title}`}>
        <SheetHeader>
          <SheetTitle>Install {mode.title}</SheetTitle>
        </SheetHeader>
        <SheetBody className="gap-4">
          <div className="rounded-lg border bg-muted/10 p-3">
            <div className="flex flex-wrap items-center gap-2">
              <Badge>{mode.profile}</Badge>
              <Badge>{mode.channel}</Badge>
            </div>
            <div className="mt-2 text-sm text-muted-fg">{mode.description}</div>
          </div>

          <div className="rounded-lg border bg-muted/10 p-3">
            <div className="mb-2 text-xs font-semibold uppercase text-muted-fg">Agent package</div>
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
            <Button isDisabled={!linuxArtifact || isGenerateDisabled} onPress={onGenerate}>
              <TerminalIcon />
              {isGenerating ? "Generating..." : "Generate command"}
            </Button>
            <Button intent="outline" size="sm" isDisabled={!command?.install_command} onPress={onCopy}>
              {copied ? <CheckIcon /> : <CopyIcon />}
              {copied ? "Copied" : "Copy install"}
            </Button>
          </div>

          <div className="text-xs font-semibold uppercase text-muted-fg">Install command</div>
          <pre className="min-h-36 overflow-auto rounded-lg border bg-muted/20 p-4 font-mono text-xs">
            {command?.install_command ?? placeholderCommand(mode)}
          </pre>
          {mode.profile === "linux-container" && (
            <>
              <div className="text-xs font-semibold uppercase text-muted-fg">Entrypoint command</div>
              <pre className="overflow-auto rounded-lg border bg-muted/20 p-4 font-mono text-xs">
                {command?.entrypoint_command ?? placeholderEntrypointCommand()}
              </pre>
            </>
          )}
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

function placeholderCommand(mode: DeploymentMode) {
  return mode.profile === "linux-container" ? "curl -fsSL ... | bash" : "curl -fsSL ... | sudo bash";
}

function placeholderEntrypointCommand() {
  return "/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent.yaml";
}
