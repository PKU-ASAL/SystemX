"use client";

import { useEffect, useMemo, useState } from "react";
import {
  CheckIcon,
  CopyIcon,
  DownloadIcon,
  PackagePlusIcon,
  RefreshCwIcon,
  TerminalIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button, buttonStyles } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
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

type DeployFormState = {
  tenantId: string;
  agentId: string;
  hostId: string;
  managementLabels: string;
  artifactId: string;
  ttl: string;
  gatewayAddr: string;
  gatewaySNI: string;
};

const initialForm: DeployFormState = {
  tenantId: "default",
  agentId: "",
  hostId: "",
  managementLabels: "env=prod,role=web",
  artifactId: "",
  ttl: "24h",
  gatewayAddr: "127.0.0.1:19444",
  gatewaySNI: "",
};

export function DeployPage() {
  const [options, setOptions] = useState<DeployOptionsView | null>(null);
  const [form, setForm] = useState<DeployFormState>(initialForm);
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
      tenantId: form.tenantId,
      signal: controller.signal,
    })
      .then((nextOptions) => {
        const view = mapDeployOptionsToView(nextOptions);
        setOptions(view);
        setForm((current) => ({
          ...current,
          gatewayAddr: current.gatewayAddr || view.gatewayAddr,
          gatewaySNI: current.gatewaySNI || view.gatewaySNI,
          artifactId: current.artifactId || activeLinuxArtifact(view)?.id || "",
        }));
      })
      .catch((nextError: unknown) => {
        if (controller.signal.aborted) return;
        setError(nextError instanceof Error ? nextError.message : "Failed to load deploy options");
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false);
      });

    return () => controller.abort();
  }, [form.tenantId, reloadKey]);

  const linuxArtifacts = useMemo(
    () => (options?.artifacts ?? []).filter((artifact) => artifact.platform.startsWith("linux/")),
    [options],
  );
  const selectedArtifact = linuxArtifacts.find((artifact) => artifact.id === form.artifactId);
  const labels = parseManagementLabels(form.managementLabels);

  function updateForm<K extends keyof DeployFormState>(key: K, value: DeployFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

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
          tenant_id: form.tenantId,
          agent_id: form.agentId,
          host_id: form.hostId,
          gateway_addr: form.gatewayAddr,
          gateway_sni: form.gatewaySNI,
          artifact_id: form.artifactId,
          ttl: form.ttl,
          labels,
        },
      });
      setCommand(nextCommand);
      reloadOptions();
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
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
          <InstallCard
            command={command}
            copied={copied}
            error={error}
            form={form}
            isGenerating={isGenerating}
            labels={labels}
            linuxArtifacts={linuxArtifacts}
            selectedArtifact={selectedArtifact}
            onCopy={copyCommand}
            onGenerate={generateCommand}
            onUpdate={updateForm}
          />
          <div className="grid content-start gap-4">
            <CompactCard title="Linux ARM64 Agent" badge="ready">
              <p className="text-sm text-muted-fg">Use the same install flow with a linux/arm64 artifact.</p>
              <Badge>{linuxArtifacts.find((artifact) => artifact.platform === "linux/arm64")?.version ?? "no artifact"}</Badge>
            </CompactCard>
            <CompactCard title="Manual install" badge="advanced">
              <div className="flex flex-wrap gap-2">
                {command?.script_url && (
                  <a className={buttonStyles({ intent: "outline", size: "sm" })} href={command.script_url}>
                    <DownloadIcon />
                    Script
                  </a>
                )}
                {selectedArtifact?.downloadUrl && (
                  <a className={buttonStyles({ intent: "outline", size: "sm" })} href={selectedArtifact.downloadUrl}>
                    <DownloadIcon />
                    Artifact
                  </a>
                )}
              </div>
              <div className="truncate font-mono text-xs text-muted-fg">sha256: {selectedArtifact?.sha256 ?? "-"}</div>
            </CompactCard>
          </div>
        </div>

        <RecentEnrollments isLoading={isLoading} rows={(options?.enrollments ?? []).slice(0, 5)} />
      </div>
    </section>
  );
}

function InstallCard({
  command,
  copied,
  error,
  form,
  isGenerating,
  labels,
  linuxArtifacts,
  selectedArtifact,
  onCopy,
  onGenerate,
  onUpdate,
}: {
  command: DeployAgentCommandResponse | null;
  copied: boolean;
  error: string | null;
  form: DeployFormState;
  isGenerating: boolean;
  labels: Record<string, string>;
  linuxArtifacts: DeployOptionsView["artifacts"];
  selectedArtifact?: DeployOptionsView["artifacts"][number];
  onCopy: () => void;
  onGenerate: () => void;
  onUpdate: <K extends keyof DeployFormState>(key: K, value: DeployFormState[K]) => void;
}) {
  return (
    <div className="rounded-lg border bg-bg">
      <div className="border-b p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="text-base font-semibold">Linux Agent</div>
            <div className="mt-1 text-sm text-muted-fg">Generate an enrollment command for a Linux host.</div>
          </div>
          <Badge>linux/amd64</Badge>
        </div>
      </div>

      <div className="grid gap-4 p-4 lg:grid-cols-2">
        <Field label="Agent ID">
          <Input
            placeholder="agent-prod-001"
            value={form.agentId}
            onChange={(event) => onUpdate("agentId", event.target.value)}
          />
        </Field>
        <Field label="Host ID">
          <Input
            placeholder="prod-api-01"
            value={form.hostId}
            onChange={(event) => onUpdate("hostId", event.target.value)}
          />
        </Field>
        <Field label="Artifact version">
          <select
            className="h-9 rounded-lg border bg-bg px-3 text-sm outline-none"
            value={form.artifactId}
            onChange={(event) => onUpdate("artifactId", event.target.value)}
          >
            <option value="">No artifact selected</option>
            {linuxArtifacts.map((artifact) => (
              <option key={artifact.id} value={artifact.id}>
                {artifact.version} · {artifact.platform}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Token TTL">
          <select
            className="h-9 rounded-lg border bg-bg px-3 text-sm outline-none"
            value={form.ttl}
            onChange={(event) => onUpdate("ttl", event.target.value)}
          >
            <option value="1h">1h</option>
            <option value="24h">24h</option>
            <option value="168h">7d</option>
          </select>
        </Field>
        <Field label="Tenant">
          <Input value={form.tenantId} onChange={(event) => onUpdate("tenantId", event.target.value)} />
        </Field>
        <Field label="Gateway">
          <Input value={form.gatewayAddr} onChange={(event) => onUpdate("gatewayAddr", event.target.value)} />
        </Field>
        <div className="lg:col-span-2">
          <Field label="Management labels">
            <Input
              placeholder="env=prod,role=web,owner=secops"
              value={form.managementLabels}
              onChange={(event) => onUpdate("managementLabels", event.target.value)}
            />
          </Field>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {Object.entries(labels).length === 0 ? (
              <Badge>no labels</Badge>
            ) : (
              Object.entries(labels).map(([key, value]) => <Badge key={key}>{key}:{value}</Badge>)
            )}
          </div>
        </div>
      </div>

      <div className="border-t p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <Button isDisabled={!form.agentId || !form.gatewayAddr || isGenerating} onPress={onGenerate}>
            <TerminalIcon />
            {isGenerating ? "Generating..." : "Generate command"}
          </Button>
          <Button intent="outline" size="sm" isDisabled={!command?.install_command} onPress={onCopy}>
            {copied ? <CheckIcon /> : <CopyIcon />}
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
        <pre className="min-h-24 overflow-auto rounded-lg border bg-muted/20 p-4 font-mono text-xs">
          {command?.install_command ?? "curl -fsSL ... | sudo bash"}
        </pre>
        <div className="mt-3 flex flex-wrap gap-2">
          <Badge>expires: {command?.token_expires_at ?? "-"}</Badge>
          <Badge>artifact: {selectedArtifact?.id ?? command?.artifact.artifact_id ?? "-"}</Badge>
          <Badge>sha256: {selectedArtifact?.sha256 ?? command?.artifact.sha256 ?? "-"}</Badge>
        </div>
        {error && <div className="mt-3 rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm text-danger">{error}</div>}
      </div>
    </div>
  );
}

function CompactCard({ title, badge, children }: { title: string; badge: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border bg-bg p-4">
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="font-semibold">{title}</div>
        <Badge>{badge}</Badge>
      </div>
      <div className="grid gap-3">{children}</div>
    </div>
  );
}

function RecentEnrollments({
  isLoading,
  rows,
}: {
  isLoading: boolean;
  rows: DeployOptionsView["enrollments"];
}) {
  return (
    <div className="mt-4 rounded-lg border bg-bg">
      <div className="flex h-11 items-center border-b px-4 text-sm font-semibold">Recent enrollments</div>
      <Table containerClassName="max-h-72 overflow-auto">
        <TableHeader className="sticky top-0 z-10 bg-muted/10">
          <TableRow>
            <TableHead>Enrollment</TableHead>
            <TableHead>Agent</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Labels</TableHead>
            <TableHead>Expires</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading || rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={5} className="h-24 text-center text-sm text-muted-fg">
                {isLoading ? "Loading..." : "No recent enrollments."}
              </TableCell>
            </TableRow>
          ) : (
            rows.map((row) => (
              <TableRow key={row.id}>
                <TableCell className="font-mono text-xs">{row.id}</TableCell>
                <TableCell className="font-mono text-xs">{row.agent}</TableCell>
                <TableCell><Badge>{row.status}</Badge></TableCell>
                <TableCell className="font-mono text-xs">{row.labels}</TableCell>
                <TableCell className="font-mono text-xs text-muted-fg">{row.expiresAt}</TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="grid gap-1 text-sm">
      <span className="font-medium text-muted-fg">{label}</span>
      {children}
    </label>
  );
}

function activeLinuxArtifact(options: DeployOptionsView) {
  return options.artifacts.find((artifact) => artifact.status === "active" && artifact.platform === "linux/amd64")
    ?? options.artifacts.find((artifact) => artifact.status === "active" && artifact.platform.startsWith("linux/"));
}
