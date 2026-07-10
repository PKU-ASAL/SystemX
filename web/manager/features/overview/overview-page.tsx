"use client";

import { useEffect, useState } from "react";
import { ActivityIcon, RefreshCwIcon, ServerIcon, ShieldAlertIcon, SignalIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { createDefaultManagerApiClient, getManagerDataSource } from "@/lib/api";
import type { OverviewMetric } from "@/lib/mock-data";
import { loadOverviewMetrics } from "@/lib/overview-data";

const metricIcons = [ServerIcon, ActivityIcon, SignalIcon, ShieldAlertIcon];
const managerApiClient = createDefaultManagerApiClient();
const managerDataSource = getManagerDataSource();

export function OverviewPage() {
  const [metrics, setMetrics] = useState<OverviewMetric[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();

    loadOverviewMetrics({
      client: managerApiClient,
      dataSource: managerDataSource,
      signal: controller.signal,
    })
      .then((nextMetrics) => {
        setMetrics(nextMetrics);
      })
      .catch((nextError: unknown) => {
        if (controller.signal.aborted) return;
        setError(nextError instanceof Error ? nextError.message : "Failed to load overview");
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      });

    return () => controller.abort();
  }, [reloadKey]);

  function reloadOverview() {
    setIsLoading(true);
    setError(null);
    setReloadKey((value) => value + 1);
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4 lg:p-6">
      <section className="grid gap-3 md:grid-cols-4">
        {(isLoading || error ? overviewMetricPlaceholders : metrics).map((metric, index) => {
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
                <div className="text-2xl font-semibold">{isLoading ? "-" : metric.value}</div>
              </CardContent>
            </Card>
          );
        })}
      </section>
      {error ? (
        <Card>
          <CardContent className="flex items-center justify-between gap-3 p-4">
            <span className="text-sm text-muted-fg">{error}</span>
            <Button intent="outline" size="sm" onPress={reloadOverview}>
              <RefreshCwIcon />
              Retry
            </Button>
          </CardContent>
        </Card>
      ) : null}

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

const overviewMetricPlaceholders: OverviewMetric[] = [
  { label: "Online agents", value: "-", trend: "loading" },
  { label: "Open events", value: "-", trend: "loading" },
  { label: "Signals", value: "-", trend: "loading" },
  { label: "Incidents", value: "-", trend: "loading" },
];
