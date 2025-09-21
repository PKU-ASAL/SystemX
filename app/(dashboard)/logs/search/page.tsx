"use client";

import { IconSearch } from "@tabler/icons-react";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

export default function LogSearchPage() {
  return (
    <div className="@container/main flex flex-1 flex-col overflow-hidden">
      <div className="flex h-full bg-gray-50">
        {/* 主内容区域 */}
        <div className="flex-1 bg-white flex flex-col min-w-0">
          {/* Header */}
          <div className="flex items-center justify-between px-4 lg:px-6 py-4 border-b">
            <div className="space-y-2">
              <Breadcrumb>
                <BreadcrumbList>
                  <BreadcrumbItem>
                    <BreadcrumbLink
                      href="#"
                      onClick={(e) => {
                        e.preventDefault();
                        if (typeof window !== "undefined") {
                          window.history.pushState(null, "", "/dashboard");
                          window.location.reload();
                        }
                      }}
                    >
                      Dashboard
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator />
                  <BreadcrumbItem>
                    <BreadcrumbPage>日志查询</BreadcrumbPage>
                  </BreadcrumbItem>
                </BreadcrumbList>
              </Breadcrumb>
              <p className="text-muted-foreground text-sm">
                搜索和查询系统日志、安全事件和审计记录
              </p>
            </div>
          </div>

          {/* 内容区域 */}
          <div className="flex-1 overflow-auto">
            <div className="max-w-4xl mx-auto p-4 lg:p-6">
              <div className="text-center py-12">
                <IconSearch className="h-16 w-16 mx-auto text-muted-foreground mb-4" />
                <h2 className="text-xl font-semibold mb-2">日志查询</h2>
                <p className="text-muted-foreground">
                  日志查询功能正在开发中，敬请期待...
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
