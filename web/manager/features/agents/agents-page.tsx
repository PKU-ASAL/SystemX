import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { agents } from "@/lib/mock-data";

export function AgentsPage() {
  return (
    <section className="flex h-full min-h-0 flex-col bg-bg">
      <header className="shrink-0 border-b px-6 py-5">
        <h1 className="text-xl font-semibold">Agent inventory</h1>
      </header>
      <div className="min-h-0 flex-1 overflow-hidden">
        <Table containerClassName="h-full" className="min-w-[720px]">
          <TableHeader className="sticky top-0 z-10 bg-bg">
            <TableRow className="text-xs text-muted-foreground">
              <TableHead>Agent</TableHead>
              <TableHead>Host</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Policy</TableHead>
              <TableHead>Version</TableHead>
              <TableHead>Last seen</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {agents.map((agent) => (
              <TableRow key={agent.id} className="hover:bg-muted/40">
                <TableCell className="font-mono text-xs text-muted-foreground">{agent.id}</TableCell>
                <TableCell className="font-medium">{agent.host}</TableCell>
                <TableCell>
                  <Badge>{agent.status}</Badge>
                </TableCell>
                <TableCell>{agent.policy}</TableCell>
                <TableCell className="font-mono text-xs">{agent.version}</TableCell>
                <TableCell className="text-muted-foreground">{agent.lastSeen}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  );
}
