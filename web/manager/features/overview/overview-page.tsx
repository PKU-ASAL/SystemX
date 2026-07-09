import { ActivityIcon, ServerIcon, ShieldAlertIcon, SignalIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { overviewMetrics } from "@/lib/mock-data";

const metricIcons = [ServerIcon, ActivityIcon, SignalIcon, ShieldAlertIcon];

export function OverviewPage() {
  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4 lg:p-6">
      <section className="grid gap-3 md:grid-cols-4">
        {overviewMetrics.map((metric, index) => {
          const Icon = metricIcons[index] ?? ActivityIcon;

          return (
            <Card key={metric.label}>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Icon />
                  {metric.label}
                </CardTitle>
                <CardDescription>{metric.trend}</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-semibold">{metric.value}</div>
              </CardContent>
            </Card>
          );
        })}
      </section>

      <section className="grid gap-4 lg:grid-cols-[1.3fr_0.7fr]">
        <Card>
          <CardHeader>
            <CardTitle>Manager API readiness</CardTitle>
            <CardDescription>后续对接 manager HTTP API 的模块边界</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 md:grid-cols-3">
            {["Inventory", "Detection", "Response"].map((item) => (
              <div key={item} className="rounded-lg border bg-muted/40 p-3">
                <div className="text-sm font-medium">{item}</div>
                <div className="mt-2 text-xs text-muted-foreground">
                  Mock first, typed client next.
                </div>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Runtime posture</CardTitle>
            <CardDescription>面向值班人员的快速判断</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2">
            <Badge>agent heartbeat stable</Badge>
            <Badge>signals indexed</Badge>
            <Badge>2 active incidents</Badge>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
