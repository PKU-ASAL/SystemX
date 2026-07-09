'use client';

import { LanguageIcon } from "@heroicons/react/24/outline";

import { ThemeModeButton } from "@/components/theme-mode-button";
import { Button } from "@/components/ui/button";
import { Breadcrumbs, BreadcrumbsItem } from "@/components/ui/breadcrumbs";
import { SidebarNav, SidebarTrigger } from "@/components/ui/sidebar";
import type { ManagerTab } from "@/lib/navigation";

interface AppSidebarNavProps {
  currentTab: ManagerTab;
}

export default function AppSidebarNav({ currentTab }: AppSidebarNavProps) {
  return (
    <SidebarNav>
      <span className="flex items-center gap-x-4">
        <SidebarTrigger className="-ml-2.5 lg:ml-0" />
        <Breadcrumbs className="hidden md:flex">
          <BreadcrumbsItem>SysArmor Manager</BreadcrumbsItem>
          <BreadcrumbsItem>{currentTab.label}</BreadcrumbsItem>
        </Breadcrumbs>
      </span>
      <span className="ml-auto" />
      <Button aria-label="语言设置" intent="plain" size="sq-sm">
        <LanguageIcon />
      </Button>
      <ThemeModeButton />
    </SidebarNav>
  );
}
