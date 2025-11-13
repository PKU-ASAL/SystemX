"use client";

import * as React from "react";
import {
  IconServer,
  IconAlertTriangle,
  IconHeart,
  IconShield,
  IconSettings,
  IconHelp,
  IconSearch,
  IconDatabase,
  IconList,
  IconUsers,
  IconNetwork,
  IconEye,
  IconGitBranch,
  IconPlus,
  IconChartBar,
} from "@tabler/icons-react";

import { NavMain } from "@/components/common/nav-main";
import { NavInfrastructure } from "@/components/common/nav-infrastructure";
import { NavSecondary } from "@/components/common/nav-secondary";
import { NavUser } from "@/components/common/nav-user";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";

const data = {
  user: {
    name: "EDR Admin",
    email: "admin@sysarmor.com",
    avatar: "/avatars/admin.jpg",
  },
  // 主要功能分区
  mainFeatures: [
    {
      title: "仪表盘",
      url: "/dashboard",
      icon: IconShield,
    },
    {
      title: "终端管理",
      icon: IconServer,
      items: [
        {
          title: "新建终端",
          url: "/collectors/create",
          icon: IconPlus,
        },
        {
          title: "终端列表",
          url: "/collectors/list",
          icon: IconList,
        },
      ],
    },
    {
      title: "威胁管理",
      icon: IconEye,
      items: [
        {
          title: "攻击告警",
          url: "/threats/alerts",
        },
        {
          title: "攻击溯源图",
          url: "/threats/timeline",
        },
        {
          title: "智能体分析",
          url: "/threats/agent-analysis",
        },
        {
          title: "Sample Data",
          url: "/threats/sample-data",
        },
      ],
    },
    {
      title: "日志管理",
      icon: IconList,
      items: [
        {
          title: "日志搜索",
          url: "/logs/search",
          icon: IconSearch,
        },
        {
          title: "日志分析",
          url: "/logs/analysis",
          icon: IconChartBar,
        },
      ],
    },
  ],
  // 基础设施分区
  infrastructure: [
    {
      title: "系统服务",
      icon: IconDatabase,
      items: [
        {
          title: "消息队列",
          url: "/services/kafka",
        },
        {
          title: "工作流管理",
          url: "/services/flink",
        },
      ],
    },
    {
      title: "系统健康",
      url: "/health",
      icon: IconHeart,
    },
  ],
  // 系统功能分区
  systemFeatures: [
    {
      title: "系统设置",
      url: "/settings",
      icon: IconSettings,
    },
    {
      title: "帮助文档",
      url: "/help",
      icon: IconHelp,
    },
  ],
};

export function AppSidebar({
  onNavigate,
  ...props
}: React.ComponentProps<typeof Sidebar> & {
  onNavigate?: (view: string) => void;
}) {
  return (
    <Sidebar collapsible="offcanvas" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              className="data-[slot=sidebar-menu-button]:!p-1.5"
            >
              <a href="#">
                <IconShield className="!size-5" />
                <span className="text-base font-semibold">SysArmor EDR</span>
              </a>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavMain items={data.mainFeatures} onNavigate={onNavigate} />
        <NavInfrastructure
          items={data.infrastructure}
          onNavigate={onNavigate}
        />
        <NavSecondary
          items={data.systemFeatures}
          onNavigate={onNavigate}
          className="mt-auto"
        />
      </SidebarContent>
      <SidebarFooter>
        <NavUser user={data.user} />
      </SidebarFooter>
    </Sidebar>
  );
}
