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
} from "@tabler/icons-react";

import { NavMain } from "@/components/nav-main";
import { NavInfrastructure } from "@/components/nav-infrastructure";
import { NavSecondary } from "@/components/nav-secondary";
import { NavUser } from "@/components/nav-user";
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
      title: "Dashboard",
      url: "/dashboard",
      icon: IconShield,
    },
    {
      title: "终端管理",
      icon: IconServer,
      items: [
        {
          title: "新建终端",
          url: "/terminal-create",
        },
        {
          title: "终端列表",
          url: "/terminal-list",
        },
      ],
    },
    {
      title: "威胁管理",
      icon: IconEye,
      items: [
        {
          title: "攻击告警",
          url: "/opensearch/alerts",
        },
        {
          title: "攻击溯源图",
          url: "/attack-timeline",
        },
      ],
    },
    {
      title: "日志管理",
      icon: IconList,
      items: [
        {
          title: "日志查询",
          url: "/logs/search",
        },
        {
          title: "日志分析",
          url: "/logs/analysis",
        },
      ],
    },
  ],
  // 基础设施分区
  infrastructure: [
    {
      title: "消息队列",
      icon: IconDatabase,
      items: [
        {
          title: "Brokers",
          url: "/kafka/brokers",
        },
        {
          title: "Topics",
          url: "/kafka/topics",
        },
      ],
    },
    {
      title: "分析工作流",
      icon: IconGitBranch,
      items: [
        {
          title: "工作流列表",
          url: "/workflows",
        },
        {
          title: "模板管理",
          url: "/workflows/templates",
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
    {
      title: "全局搜索",
      url: "/search",
      icon: IconSearch,
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
