import { useMemo, useRef, useState } from "react";
import {
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type Cell,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";

import { GitBranchIcon, NetworkIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { incidents, type IncidentRecord } from "@/lib/mock-data";
import { cn } from "@/lib/utils";

const severityClass = {
  critical: "border-destructive/40 bg-destructive/10 text-destructive",
  high: "text-status-high",
  medium: "text-status-medium",
} as const;

export function IncidentsPage() {
  return (
    <section className="flex h-full min-h-0 flex-col bg-bg">
      <header className="shrink-0 border-b bg-muted/10 px-6 py-5">
        <h1 className="text-xl font-semibold">威胁管理</h1>
      </header>
      <IncidentTable rows={incidents} />
    </section>
  );
}

function IncidentTable({ rows }: { rows: IncidentRecord[] }) {
  const [sorting, setSorting] = useState<SortingState>([{ id: "alertCount", desc: true }]);
  const parentRef = useRef<HTMLDivElement>(null);
  const columns = useMemo<ColumnDef<IncidentRecord>[]>(
    () => [
      {
        id: "chain",
        header: "攻击链",
        accessorFn: (incident) => incident.chainId,
        cell: ({ row }) => (
          <div>
            <div className="font-mono text-sm font-semibold">{row.original.chainId}</div>
            <div className="mt-1 text-xs text-muted-fg">{row.original.title}</div>
          </div>
        ),
      },
      {
        id: "killChain",
        header: "Kill Chain",
        enableSorting: false,
        cell: ({ row }) => (
          <KillChainDots
            detected={row.original.detectedStages}
            total={row.original.totalStages}
          />
        ),
      },
      {
        id: "actions",
        header: "操作",
        enableSorting: false,
        cell: ({ row }) => <IncidentActions incident={row.original} />,
      },
      {
        id: "hosts",
        header: "涉及主机",
        accessorFn: (incident) => incident.hosts.join(","),
        cell: ({ row }) => <HostChips hosts={row.original.hosts} />,
      },
      {
        accessorKey: "alertCount",
        header: "告警数",
        cell: ({ row }) => (
          <span className="font-semibold tabular-nums">{row.original.alertCount}</span>
        ),
      },
      {
        accessorKey: "severity",
        header: "严重度",
        cell: ({ row }) => (
          <Badge className={cn("rounded-full px-3", severityClass[row.original.severity])}>
            {row.original.severity}
          </Badge>
        ),
      },
    ],
    [],
  );
  // TanStack Table intentionally returns method-heavy state objects; keep it outside React Compiler memoization.
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    data: rows,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getRowId: (row) => row.id,
  });

  return (
    <div className="min-h-0 flex-1 overflow-hidden bg-bg">
      <Table
        containerClassName="h-full overflow-auto"
        containerRef={parentRef}
        className="min-w-[1040px]"
      >
        <TableHeader className="sticky top-0 z-10 bg-muted/10">
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {header.isPlaceholder ? null : (
                    <button
                      className={cn(
                        "flex items-center gap-1 text-left",
                        header.column.getCanSort() && "cursor-pointer hover:text-fg",
                      )}
                      type="button"
                      onClick={header.column.getToggleSortingHandler()}
                    >
                      {flexRender(header.column.columnDef.header, header.getContext())}
                      {header.column.getIsSorted() === "asc" && <span>↑</span>}
                      {header.column.getIsSorted() === "desc" && <span>↓</span>}
                    </button>
                  )}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.map((row) => (
            <IncidentRow
              key={row.id}
              cells={row.getVisibleCells()}
            />
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function IncidentRow({ cells }: { cells: Cell<IncidentRecord, unknown>[] }) {
  return (
    <TableRow className="hover:bg-muted/40">
      {cells.map((cell) => (
        <TableCell key={cell.id}>
          {flexRender(cell.column.columnDef.cell, cell.getContext())}
        </TableCell>
      ))}
    </TableRow>
  );
}

function IncidentActions({ incident }: { incident: IncidentRecord }) {
  return (
    <div className="flex items-center gap-2">
      <Button aria-label={`查看 ${incident.chainId} 攻击链`} intent="plain" size="xs">
        <GitBranchIcon />
        攻击链
      </Button>
      <Button aria-label={`查看 ${incident.chainId} 溯源图`} intent="plain" size="xs">
        <NetworkIcon />
        溯源图
      </Button>
    </div>
  );
}

function HostChips({ hosts }: { hosts: string[] }) {
  return (
    <div className="flex flex-wrap gap-2">
      {hosts.map((host) => (
        <span key={host} className="rounded border bg-bg px-2 py-1 font-mono text-xs text-muted-fg">
          {host}
        </span>
      ))}
    </div>
  );
}

function KillChainDots({ detected, total }: { detected: number; total: number }) {
  return (
    <div className="flex items-center gap-1">
      {Array.from({ length: total }).map((_, index) => {
        const active = index < detected;

        return (
          <span
            // Static index sequence renders a visual phase strip; there is no item id.
            key={index}
            className={cn("size-2.5 rounded-full", active ? "bg-success" : "bg-muted-fg/50")}
          />
        );
      })}
      <span className="ml-2 text-xs text-muted-fg">
        {detected}/{total}
      </span>
    </div>
  );
}
