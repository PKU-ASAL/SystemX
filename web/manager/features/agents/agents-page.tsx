"use client";

import { useEffect, useMemo, useState } from "react";
import { CheckIcon, CopyIcon, EyeIcon, RefreshCwIcon, Trash2Icon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { FieldMetadataPopover } from "@/components/field-metadata-popover";
import { QueryAutocompleteInput } from "@/components/query-autocomplete-input";
import {
  SearchToolbar,
  searchToolbarInlineSelectClass,
  searchToolbarRunButtonClass,
} from "@/components/search-toolbar";
import { Button, buttonStyles } from "@/components/ui/button";
import {
  Sheet,
  SheetBody,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { createDefaultManagerApiClient, getManagerDataSource } from "@/lib/api";
import { buildAgentUninstallCommand, loadAgentRecords } from "@/lib/agents-data";
import { filterAgents, type AgentRecord } from "@/lib/mock-data";
import { getSearchFieldsForIndexPattern } from "@/lib/opensearch-fields";

const agentIndexPattern = "agents-*";
const managerApiClient = createDefaultManagerApiClient();
const managerDataSource = getManagerDataSource();

export function AgentsPage() {
  const [query, setQuery] = useState("");
  const [records, setRecords] = useState<AgentRecord[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [copiedAgentId, setCopiedAgentId] = useState<string | null>(null);
  const searchFields = useMemo(() => getSearchFieldsForIndexPattern(agentIndexPattern), []);
  const filteredAgents = useMemo(() => filterAgents(records, query), [query, records]);

  useEffect(() => {
    const controller = new AbortController();

    loadAgentRecords({
      client: managerApiClient,
      dataSource: managerDataSource,
      signal: controller.signal,
    })
      .then((nextRecords) => {
        setRecords(nextRecords);
      })
      .catch((nextError: unknown) => {
        if (controller.signal.aborted) return;
        setError(nextError instanceof Error ? nextError.message : "Failed to load agents");
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      });

    return () => controller.abort();
  }, [reloadKey]);

  function reloadAgents() {
    setIsLoading(true);
    setError(null);
    setReloadKey((value) => value + 1);
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-bg">
      <header className="shrink-0 border-b bg-muted/10 p-4">
        <SearchToolbar
          controls={
            <>
              <InlineIndexPattern />
              <QueryAutocompleteInput
                query={query}
                fields={searchFields}
                placeholder="agent.id: agent-prod-001 or status:healthy"
                ariaLabel="Agent query"
                onQueryChange={setQuery}
              />
              <Button className={searchToolbarRunButtonClass} size="md" onPress={reloadAgents}>
                <RefreshCwIcon />
                Run
              </Button>
            </>
          }
          meta={
            <>
              <Badge>{filteredAgents.length} agents</Badge>
              <FieldMetadataPopover fields={searchFields} />
            </>
          }
        />
      </header>
      <div className="min-h-0 flex-1 overflow-hidden bg-bg">
        <Table containerClassName="h-full overflow-auto" className="min-w-[900px]">
          <TableHeader className="sticky top-0 z-10 bg-muted/10">
            <TableRow>
              <TableHead>Agent</TableHead>
              <TableHead>Host</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Policy</TableHead>
              <TableHead>Version</TableHead>
              <TableHead>Registered</TableHead>
              <TableHead>Last seen</TableHead>
              <TableHead>Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading || error || filteredAgents.length === 0 ? (
              <TableStateRow
                message={
                  isLoading
                    ? "Loading agents..."
                    : error
                      ? error
                      : records.length === 0
                        ? "No agents have enrolled yet."
                        : "No agents match the current query."
                }
              />
            ) : (
              filteredAgents.map((agent) => (
                <TableRow key={agent.id} className="hover:bg-muted/40">
                  <TableCell className="font-mono text-sm font-semibold">{agent.id}</TableCell>
                  <TableCell className="font-medium">{agent.host}</TableCell>
                  <TableCell>
                    <Badge>{agent.status}</Badge>
                  </TableCell>
                  <TableCell>{agent.policy}</TableCell>
                  <TableCell className="font-mono text-xs">{agent.version}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-fg">{agent.registeredAt}</TableCell>
                  <TableCell className="text-muted-fg">{agent.lastSeen}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <AgentDetailsSheet agent={agent} />
                      <AgentUninstallSheet
                        agent={agent}
                        copied={copiedAgentId === agent.id}
                        onCopy={() => {
                          void navigator.clipboard?.writeText(buildAgentUninstallCommand(agent.id));
                          setCopiedAgentId(agent.id);
                        }}
                      />
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </section>
  );
}

function TableStateRow({ message }: { message: string }) {
  return (
    <TableRow>
      <TableCell colSpan={8} className="h-32 text-center text-sm text-muted-fg">
        {message}
      </TableCell>
    </TableRow>
  );
}

function AgentDetailsSheet({ agent }: { agent: AgentRecord }) {
  return (
    <Sheet>
      <SheetTrigger
        className={buttonStyles({ intent: "plain", size: "sq-xs" })}
        aria-label={`View details for ${agent.id}`}
      >
        <EyeIcon />
      </SheetTrigger>
      <SheetContent className="sm:max-w-[440px]" aria-label="Agent details">
        <SheetHeader>
          <SheetTitle>Agent details</SheetTitle>
        </SheetHeader>
        <SheetBody className="gap-4">
          <DetailGrid
            rows={[
              ["Agent", agent.id],
              ["Host", agent.host],
              ["Status", agent.status],
              ["Policy", agent.policy],
              ["Version", agent.version],
              ["Registered", agent.registeredAt],
              ["Last seen", agent.lastSeen],
            ]}
          />
          <div className="rounded-lg border bg-muted/20 p-3">
            <div className="mb-2 text-xs font-semibold uppercase text-muted-fg">Events query</div>
            <code className="font-mono text-xs">agent.id:{agent.id}</code>
          </div>
        </SheetBody>
      </SheetContent>
    </Sheet>
  );
}

function AgentUninstallSheet({
  agent,
  copied,
  onCopy,
}: {
  agent: AgentRecord;
  copied: boolean;
  onCopy: () => void;
}) {
  const command = buildAgentUninstallCommand(agent.id);

  return (
    <Sheet>
      <SheetTrigger
        className={buttonStyles({ intent: "plain", size: "sq-xs" })}
        aria-label={`Uninstall ${agent.id}`}
      >
        <Trash2Icon />
      </SheetTrigger>
      <SheetContent className="sm:max-w-[520px]" aria-label="Uninstall agent">
        <SheetHeader>
          <SheetTitle>Uninstall agent</SheetTitle>
        </SheetHeader>
        <SheetBody className="gap-4">
          <DetailGrid rows={[["Agent", agent.id], ["Host", agent.host], ["Status", agent.status]]} />
          <pre className="overflow-auto rounded-lg border bg-muted/20 p-4 font-mono text-xs">{command}</pre>
          <Button className="self-start" intent="outline" size="sm" onPress={onCopy}>
            {copied ? <CheckIcon /> : <CopyIcon />}
            {copied ? "Copied" : "Copy command"}
          </Button>
        </SheetBody>
      </SheetContent>
    </Sheet>
  );
}

function DetailGrid({ rows }: { rows: Array<[string, string]> }) {
  return (
    <dl className="grid grid-cols-[120px_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
      {rows.map(([label, value]) => (
        <div key={label} className="contents">
          <dt className="text-muted-fg">{label}</dt>
          <dd className="min-w-0 truncate font-medium">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function InlineIndexPattern() {
  return (
    <label className={searchToolbarInlineSelectClass}>
      <span className="shrink-0 text-xs font-medium text-muted-fg">Index</span>
      <select
        aria-label="Agent index pattern"
        className="h-11 min-w-0 flex-1 bg-transparent text-sm font-medium outline-none"
        value={agentIndexPattern}
        disabled
      >
        <option value={agentIndexPattern}>{agentIndexPattern}</option>
      </select>
    </label>
  );
}
