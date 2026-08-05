'use client';

import { ArrowRightStartOnRectangleIcon, BoltIcon, CircleStackIcon, ShieldCheckIcon } from "@heroicons/react/24/outline";

import { logoutAction } from "@/app/login/actions";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarItem,
  SidebarLabel,
  SidebarRail,
  SidebarSection,
  SidebarSectionGroup,
} from "@/components/ui/sidebar";
import { managerTabs, type ManagerTabId } from "@/lib/navigation";

interface AppSidebarProps extends React.ComponentProps<typeof Sidebar> {
  activeTab: ManagerTabId;
  onTabChange: (tab: ManagerTabId) => void;
}

export default function AppSidebar({ activeTab, onTabChange, ...props }: AppSidebarProps) {
  return (
    <Sidebar {...props}>
      <SidebarHeader>
        <div className="flex items-center gap-x-2">
          <div className="flex size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sm font-semibold text-sidebar-primary-fg">
            SA
          </div>
          <SidebarLabel className="font-medium">
            SysArmor <span className="text-muted-fg">Manager</span>
          </SidebarLabel>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <SidebarSectionGroup>
          <SidebarSection label="Console">
            {managerTabs.map((tab) => (
              <SidebarItem
                key={tab.id}
                tooltip={tab.label}
                isCurrent={activeTab === tab.id}
                onPress={() => onTabChange(tab.id)}
              >
                <tab.icon />
                <SidebarLabel>{tab.label}</SidebarLabel>
              </SidebarItem>
            ))}
          </SidebarSection>
          <SidebarSection label="Data plane">
            <SidebarItem tooltip="OpenSearch" badge="2 idx">
              <CircleStackIcon />
              <SidebarLabel>OpenSearch</SidebarLabel>
            </SidebarItem>
            <SidebarItem tooltip="Policy">
              <ShieldCheckIcon />
              <SidebarLabel>Policy</SidebarLabel>
            </SidebarItem>
          </SidebarSection>
        </SidebarSectionGroup>
      </SidebarContent>

      <SidebarFooter className="justify-start">
        <div className="flex min-w-0 items-center gap-x-2">
          <BoltIcon className="size-4 text-muted-fg" />
          <SidebarLabel>
            <Badge>manager online</Badge>
          </SidebarLabel>
        </div>
        <form action={logoutAction}>
          <Button intent="plain" size="sm" type="submit" className="w-full justify-start">
            <ArrowRightStartOnRectangleIcon />
            退出登录
          </Button>
        </form>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
