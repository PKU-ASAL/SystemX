import {
  ActivityIcon,
  LayoutDashboardIcon,
  RadioTowerIcon,
  ShieldAlertIcon,
  type LucideIcon,
} from "lucide-react";

export type ManagerTabId = "overview" | "agents" | "events" | "incidents";

export interface ManagerTab {
  id: ManagerTabId;
  label: string;
  description: string;
  icon: LucideIcon;
}

export const DEFAULT_MANAGER_TAB: ManagerTabId = "overview";

export const managerTabs: ManagerTab[] = [
  {
    id: "overview",
    label: "Overview",
    description: "运行态势与关键指标",
    icon: LayoutDashboardIcon,
  },
  {
    id: "agents",
    label: "Agents",
    description: "终端接入、版本与健康状态",
    icon: RadioTowerIcon,
  },
  {
    id: "events",
    label: "Events",
    description: "检索 OpenSearch event 与 signal index",
    icon: ActivityIcon,
  },
  {
    id: "incidents",
    label: "Incidents",
    description: "威胁情报、溯源图与攻击链",
    icon: ShieldAlertIcon,
  },
];

export function getManagerTabById(tabId: string) {
  return managerTabs.find((tab) => tab.id === tabId);
}
