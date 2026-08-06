import { getOverview } from "./api/overview";
import type { ManagerApiClient } from "./api/client";
import type { OverviewSummary } from "./api/types";
import { overviewMetrics, type OverviewMetric } from "./mock-data";

export type LoadOverviewOptions = {
  client: ManagerApiClient;
  dataSource: "api" | "mock";
  signal?: AbortSignal;
};

export async function loadOverviewMetrics({ client, dataSource, signal }: LoadOverviewOptions) {
  if (dataSource === "mock") {
    return overviewMetrics;
  }

  return mapOverviewSummaryToMetrics(await getOverview(client, { signal }));
}

export function mapOverviewSummaryToMetrics(summary: OverviewSummary): OverviewMetric[] {
  return [
    {
      label: "Online agents",
      value: `${summary.agents.online}/${summary.agents.total}`,
      trend: `${summary.agents.degraded} degraded · ${summary.agents.offline} offline`,
    },
    {
      label: "Open events",
      value: formatCount(summary.telemetry.events_24h),
      trend: "last 24h",
    },
    {
      label: "Signals",
      value: formatCount(summary.telemetry.signals_24h),
      trend: "last 24h",
    },
    {
      label: "Incidents",
      value: formatCount(summary.incidents.open),
      trend: `${summary.incidents.critical} critical · ${summary.incidents.high} high`,
    },
  ];
}

function formatCount(value: number) {
  return new Intl.NumberFormat("en-US", { notation: value >= 10000 ? "compact" : "standard" }).format(value);
}
