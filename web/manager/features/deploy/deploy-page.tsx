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
import {
  createAgentInstallCommand,
  loadDeployOptions,
  mapDeployOptionsToView,
  type DeployOptionsView,
} from "@/lib/deploy-data";
import type { DeployAgentCommandResponse } from "@/lib/api/types";

const managerApiClient = createDefaultManagerApiClient();
const managerDataSource = getManagerDataSource();

type DeployFormState = {
  platform: string;
  tenantId: string;
  agentId: string;
  hostId: string;
  labels: string;
  artifactId: string;
  ttl: string;
  gatewayAddr: string;
  gatewaySNI: string;
};

const initialForm: DeployFormState = {
  platform: "linux/amd64",
  tenantId: "default",
  agentId: "",
  hostId: "",
  labels: "env=prod",
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
          artifactId: current.artifactId || view.artifacts.find((item) => item.status === "active")?.id || "",
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

  const selectedArtifact = useMemo(
    () => options?.artifacts.find((artifact) => artifact.id === form.artifactId),
    [form.artifactId, options],
  );

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
    const [os, arch] = form.platform.split("/");

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
          labels: parseLabels(form.labels),
        },
      });
      setCommand(nextCommand);
      setForm((current) => ({
        ...current,
        platform: `${os}/${arch}`,
      }));
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
      <div className="shrink-0 border-b bg-muted/10 p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <PackagePlusIcon className="size-5 text-muted-fg" />
            <div className="text-sm font-semibold">Agent deployment</div>
          </div>
          <Button intent="outline" size="sm" onPress={reloadOptions}>
            <RefreshCwIcon />
            Refresh
          </Button>
        </div>
      </div>
      <div className="grid min-h-0 flex-1 grid-cols-[380px_minmax(0,1fr)] overflow-hidden max-xl:grid-cols-1">
        <div className="min-h-0 overflow-auto border-r bg-muted/5 p-4 max-xl:border-r-0 max-xl:border-b">
          <div className="grid gap-3">
            <Field label="Platform">
              <select
                className="h-9 rounded-lg border bg-bg px-3 text-sm outline-none"
                value={form.platform}
                onChange={(event) => updateForm("platform", event.target.value)}
              >
                {(options?.platforms ?? [{ id: "linux/amd64", label: "linux/amd64" }]).map((platform) => (
                  <option key={platform.id} value={platform.id}>
                    {platform.label}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Tenant">
              <Input value={form.tenantId} onChange={(event) => updateForm("tenantId", event.target.value)} />
            </Field>
            <Field label="Agent ID">
              <Input
                placeholder="agent-prod-001"
                value={form.agentId}
                onChange={(event) => updateForm("agentId", event.target.value)}
              />
            </Field>
            <Field label="Host ID">
              <Input
                placeholder="prod-api-01"
                value={form.hostId}
                onChange={(event) => updateForm("hostId", event.target.value)}
              />
            </Field>
            <Field label="Labels">
              <Input
                placeholder="env=prod,role=api"
                value={form.labels}
                onChange={(event) => updateForm("labels", event.target.value)}
              />
            </Field>
            <Field label="Artifact">
              <select
                className="h-9 rounded-lg border bg-bg px-3 text-sm outline-none"
                value={form.artifactId}
                onChange={(event) => updateForm("artifactId", event.target.value)}
              >
                <option value="">No artifact selected</option>
                {(options?.artifacts ?? []).map((artifact) => (
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
                onChange={(event) => updateForm("ttl", event.target.value)}
              >
                <option value="1h">1h</option>
                <option value="24h">24h</option>
                <option value="168h">7d</option>
              </select>
            </Field>
            <Field label="Gateway">
              <Input
                value={form.gatewayAddr}
                onChange={(event) => updateForm("gatewayAddr", event.target.value)}
              />
            </Field>
            <Field label="Gateway SNI">
              <Input value={form.gatewaySNI} onChange={(event) => updateForm("gatewaySNI", event.target.value)} />
            </Field>
            <Button className="mt-1" isDisabled={!form.agentId || !form.gatewayAddr || isGenerating} onPress={generateCommand}>
              <TerminalIcon />
              {isGenerating ? "Generating..." : "Generate command"}
            </Button>
            {error && <div className="rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm text-danger">{error}</div>}
          </div>
        </div>
        <div className="flex min-h-0 flex-col overflow-hidden">
          <div className="shrink-0 border-b p-4">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <div className="text-sm font-semibold">Install command</div>
              <div className="flex items-center gap-2">
                {command?.script_url && (
                  <a className={buttonStyles({ intent: "outline", size: "sm" })} href={command.script_url}>
                    <DownloadIcon />
                    Script
                  </a>
                )}
                <Button intent="outline" size="sm" isDisabled={!command?.install_command} onPress={copyCommand}>
                  {copied ? <CheckIcon /> : <CopyIcon />}
                  {copied ? "Copied" : "Copy"}
                </Button>
              </div>
            </div>
            <pre className="min-h-24 overflow-auto rounded-lg border bg-muted/20 p-4 font-mono text-xs text-fg">
              {command?.install_command ?? "Generate a command to create an enrollment token."}
            </pre>
            <div className="mt-3 flex flex-wrap gap-2">
              <Badge>expires: {command?.token_expires_at ?? "-"}</Badge>
              <Badge>artifact: {selectedArtifact?.id ?? command?.artifact.artifact_id ?? "-"}</Badge>
              <Badge>sha256: {selectedArtifact?.sha256 ?? command?.artifact.sha256 ?? "-"}</Badge>
            </div>
          </div>
          <div className="grid min-h-0 flex-1 grid-rows-2 overflow-hidden">
            <DeployTable
              title="Artifacts"
              isLoading={isLoading}
              columns={["Version", "Platform", "Status", "SHA256", "Uploaded"]}
              rows={(options?.artifacts ?? []).map((artifact) => [
                artifact.version,
                artifact.platform,
                artifact.status,
                artifact.sha256,
                artifact.createdAt,
              ])}
            />
            <DeployTable
              title="Recent enrollments"
              isLoading={isLoading}
              columns={["Enrollment", "Agent", "Status", "Labels", "Expires", "Used"]}
              rows={(options?.enrollments ?? []).map((enrollment) => [
                enrollment.id,
                enrollment.agent,
                enrollment.status,
                enrollment.labels,
                enrollment.expiresAt,
                enrollment.usedAt,
              ])}
            />
          </div>
        </div>
      </div>
    </section>
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

function DeployTable({
  title,
  isLoading,
  columns,
  rows,
}: {
  title: string;
  isLoading: boolean;
  columns: string[];
  rows: string[][];
}) {
  return (
    <div className="min-h-0 overflow-hidden border-b">
      <div className="flex h-10 items-center border-b px-4 text-sm font-semibold">{title}</div>
      <Table containerClassName="h-[calc(100%-2.5rem)] overflow-auto">
        <TableHeader className="sticky top-0 z-10 bg-muted/10">
          <TableRow>
            {columns.map((column) => (
              <TableHead key={column}>{column}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading || rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={columns.length} className="h-24 text-center text-sm text-muted-fg">
                {isLoading ? "Loading..." : "No records."}
              </TableCell>
            </TableRow>
          ) : (
            rows.map((row, rowIndex) => (
              <TableRow key={`${title}-${rowIndex}`}>
                {row.map((cell, cellIndex) => (
                  <TableCell key={`${title}-${rowIndex}-${cellIndex}`} className="font-mono text-xs">
                    {cell}
                  </TableCell>
                ))}
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}

function parseLabels(raw: string) {
  const labels: Record<string, string> = {};

  for (const token of raw.split(",")) {
    const [key, value] = token.split("=");
    if (key?.trim() && value?.trim()) {
      labels[key.trim()] = value.trim();
    }
  }

  return labels;
}
