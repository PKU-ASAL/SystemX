"use client";

import { useEffect, useMemo, useState } from "react";
import { RefreshCwIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { FieldMetadataPopover } from "@/components/field-metadata-popover";
import { QueryAutocompleteInput } from "@/components/query-autocomplete-input";
import {
  SearchToolbar,
  searchToolbarInlineSelectClass,
  searchToolbarRunButtonClass,
} from "@/components/search-toolbar";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { createDefaultManagerApiClient, getManagerDataSource } from "@/lib/api";
import { loadAgentRecords } from "@/lib/agents-data";
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
      <TableCell colSpan={7} className="h-32 text-center text-sm text-muted-fg">
        {message}
      </TableCell>
    </TableRow>
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
