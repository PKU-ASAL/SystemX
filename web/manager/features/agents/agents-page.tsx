"use client";

import { useMemo, useState } from "react";
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
import { agents, filterAgents } from "@/lib/mock-data";
import { getSearchFieldsForIndexPattern } from "@/lib/opensearch-fields";

const agentIndexPattern = "agents-*";

export function AgentsPage() {
  const [query, setQuery] = useState("");
  const searchFields = useMemo(() => getSearchFieldsForIndexPattern(agentIndexPattern), []);
  const filteredAgents = useMemo(() => filterAgents(agents, query), [query]);

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
              <Button className={searchToolbarRunButtonClass} size="md">
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
            {filteredAgents.map((agent) => (
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
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
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
