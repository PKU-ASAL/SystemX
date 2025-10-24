"use client";

import { useState, useEffect } from "react";
import { useRouter, usePathname } from "next/navigation";
import { IconChevronRight, type Icon } from "@tabler/icons-react";
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@/components/ui/sidebar";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

export function NavMain({
  items,
  onNavigate,
}: {
  items: {
    title: string;
    url?: string;
    icon?: Icon;
    items?: {
      title: string;
      url: string;
    }[];
  }[];
  onNavigate?: (view: string) => void;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const [openItems, setOpenItems] = useState<string[]>([]);
  const [isClient, setIsClient] = useState(false);

  useEffect(() => {
    setIsClient(true);

    // Auto-expand parent items if current path matches any sub-item
    const itemsToOpen: string[] = [];
    items.forEach((item) => {
      if (item.items) {
        const hasActiveSubItem = item.items.some(
          (subItem) => pathname === subItem.url
        );
        if (hasActiveSubItem) {
          itemsToOpen.push(item.title);
        }
      }
    });
    setOpenItems(itemsToOpen);
  }, [pathname, items]);

  const toggleItem = (title: string) => {
    setOpenItems((prev) =>
      prev.includes(title)
        ? prev.filter((item) => item !== title)
        : [...prev, title]
    );
  };

  const isActive = (url?: string) => {
    if (!url) return false;
    return pathname === url;
  };

  const hasActiveSubItem = (subItems?: { title: string; url: string }[]) => {
    if (!subItems) return false;
    return subItems.some((subItem) => pathname === subItem.url);
  };
  return (
    <SidebarGroup>
      <SidebarGroupContent>
        <SidebarMenu>
          {items.map((item) => (
            <Collapsible
              key={item.title}
              asChild
              open={isClient ? openItems.includes(item.title) : false}
              onOpenChange={() => isClient && toggleItem(item.title)}
              className="group"
            >
              <SidebarMenuItem>
                {item.items ? (
                  <>
                    <CollapsibleTrigger asChild>
                      <SidebarMenuButton
                        tooltip={item.title}
                        className={
                          hasActiveSubItem(item.items)
                            ? "bg-sidebar-accent text-sidebar-accent-foreground"
                            : ""
                        }
                      >
                        {item.icon && <item.icon />}
                        <span>{item.title}</span>
                        <IconChevronRight className="ml-auto transition-transform duration-200 group-data-[state=open]:rotate-90" />
                      </SidebarMenuButton>
                    </CollapsibleTrigger>
                    <CollapsibleContent>
                      <SidebarMenuSub>
                        {item.items.map((subItem) => (
                          <SidebarMenuSubItem key={subItem.title}>
                            <SidebarMenuSubButton
                              asChild
                              isActive={isActive(subItem.url)}
                            >
                              <a
                                href={subItem.url}
                                onClick={(e) => {
                                  e.preventDefault();
                                  router.push(subItem.url);
                                }}
                              >
                                {subItem.title}
                              </a>
                            </SidebarMenuSubButton>
                          </SidebarMenuSubItem>
                        ))}
                      </SidebarMenuSub>
                    </CollapsibleContent>
                  </>
                ) : (
                  <SidebarMenuButton
                    tooltip={item.title}
                    asChild
                    isActive={isActive(item.url)}
                  >
                    <a
                      href={item.url}
                      onClick={(e) => {
                        e.preventDefault();
                        if (item.url) {
                          router.push(item.url);
                        }
                      }}
                    >
                      {item.icon && <item.icon />}
                      <span>{item.title}</span>
                    </a>
                  </SidebarMenuButton>
                )}
              </SidebarMenuItem>
            </Collapsible>
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}
