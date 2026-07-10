"use client";

import { useMemo, useState } from "react";

import AppSidebar from "@/components/app-sidebar";
import AppSidebarNav from "@/components/app-sidebar-nav";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { AgentsPage } from "@/features/agents/agents-page";
import { DeployPage } from "@/features/deploy/deploy-page";
import { EventsPage } from "@/features/events/events-page";
import { IncidentsPage } from "@/features/incidents/incidents-page";
import { OverviewPage } from "@/features/overview/overview-page";
import {
  DEFAULT_MANAGER_TAB,
  managerTabs,
  type ManagerTabId,
} from "@/lib/navigation";

const pageByTab: Record<ManagerTabId, React.ReactNode> = {
  overview: <OverviewPage />,
  deploy: <DeployPage />,
  agents: <AgentsPage />,
  events: <EventsPage />,
  incidents: <IncidentsPage />,
};

export function ManagerConsole() {
  const [activeTab, setActiveTab] = useState<ManagerTabId>(DEFAULT_MANAGER_TAB);
  const currentTab = useMemo(
    () => managerTabs.find((tab) => tab.id === activeTab) ?? managerTabs[0],
    [activeTab],
  );

  return (
    <SidebarProvider className="min-h-svh bg-bg">
      <AppSidebar activeTab={activeTab} onTabChange={setActiveTab} collapsible="dock" />
      <SidebarInset>
        <AppSidebarNav currentTab={currentTab} />
        <div className="min-h-0 flex-1 overflow-hidden">{pageByTab[activeTab]}</div>
      </SidebarInset>
    </SidebarProvider>
  );
}
