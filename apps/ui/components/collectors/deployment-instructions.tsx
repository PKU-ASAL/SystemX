"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Copy } from "lucide-react";

interface DeploymentInstructionsProps {
  collectorId: string;
  className?: string;
}

export function DeploymentInstructions({
  collectorId,
  className = "",
}: DeploymentInstructionsProps) {
  return (
    <div className={`space-y-4 ${className}`}>
      <div className="flex items-center gap-2">
        <Label className="text-base font-medium">部署指令</Label>
        <Badge variant="secondary" className="text-xs">
          复制粘贴执行
        </Badge>
      </div>

      {/* 安装指令 */}
      <div className="space-y-4 p-4 bg-gray-50 rounded-lg border">
        <div className="flex items-center justify-between">
          <Label className="text-base font-medium">安装指令</Label>
          <Badge variant="default" className="text-xs">
            安装 SysArmor Agent
          </Badge>
        </div>

        <div className="space-y-3">
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-gray-700">
                1. 下载安装脚本
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => {
                  const downloadCmd = `wget -O sysarmor-install.sh "{API_BASE_URL}/api/v1/scripts/setup-terminal.sh?collector_id=${collectorId}"`;
                  navigator.clipboard.writeText(downloadCmd);
                }}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
            <div className="bg-black text-green-400 p-3 rounded font-mono text-sm overflow-x-auto">
              {`wget -O sysarmor-install.sh "{API_BASE_URL}/api/v1/scripts/setup-terminal.sh?collector_id=${collectorId}"`}
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-gray-700">
                2. 添加执行权限
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => {
                  navigator.clipboard.writeText("chmod +x sysarmor-install.sh");
                }}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
            <div className="bg-black text-green-400 p-3 rounded font-mono text-sm">
              chmod +x sysarmor-install.sh
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-gray-700">
                3. 执行安装脚本
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => {
                  const installCmd = "sudo ./sysarmor-install.sh";
                  navigator.clipboard.writeText(installCmd);
                }}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
            <div className="bg-black text-green-400 p-3 rounded font-mono text-sm">
              sudo ./sysarmor-install.sh
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-gray-700">
                4. 一键执行（完整命令）
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => {
                  const fullCmd = `wget -O sysarmor-install.sh "{API_BASE_URL}/api/v1/scripts/setup-terminal.sh?collector_id=${collectorId}" && chmod +x sysarmor-install.sh && sudo ./sysarmor-install.sh`;
                  navigator.clipboard.writeText(fullCmd);
                }}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
            <div className="bg-black text-green-400 p-3 rounded font-mono text-sm overflow-x-auto">
              {`wget -O sysarmor-install.sh "{API_BASE_URL}/api/v1/scripts/setup-terminal.sh?collector_id=${collectorId}" && chmod +x sysarmor-install.sh && sudo ./sysarmor-install.sh`}
            </div>
          </div>
        </div>
      </div>

      {/* 卸载指令 */}
      <div className="space-y-4 p-4 bg-red-50 rounded-lg border border-red-200">
        <div className="flex items-center justify-between">
          <Label className="text-base font-medium text-red-800">卸载指令</Label>
          <Badge variant="destructive" className="text-xs">
            谨慎操作
          </Badge>
        </div>

        <div className="space-y-3">
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-red-700">
                1. 下载卸载脚本
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => {
                  const downloadCmd = `wget -O sysarmor-uninstall.sh "{API_BASE_URL}/api/v1/scripts/uninstall-terminal.sh?collector_id=${collectorId}"`;
                  navigator.clipboard.writeText(downloadCmd);
                }}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
            <div className="bg-black text-red-400 p-3 rounded font-mono text-sm overflow-x-auto">
              {`wget -O sysarmor-uninstall.sh "{API_BASE_URL}/api/v1/scripts/uninstall-terminal.sh?collector_id=${collectorId}"`}
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-red-700">
                2. 添加执行权限
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => {
                  navigator.clipboard.writeText(
                    "chmod +x sysarmor-uninstall.sh"
                  );
                }}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
            <div className="bg-black text-red-400 p-3 rounded font-mono text-sm">
              chmod +x sysarmor-uninstall.sh
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-red-700">
                3. 执行卸载脚本
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => {
                  const uninstallCmd = "sudo ./sysarmor-uninstall.sh";
                  navigator.clipboard.writeText(uninstallCmd);
                }}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
            <div className="bg-black text-red-400 p-3 rounded font-mono text-sm">
              sudo ./sysarmor-uninstall.sh
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-red-700">
                4. 一键卸载（完整命令）
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2"
                onClick={() => {
                  const fullUninstallCmd = `wget -O sysarmor-uninstall.sh "{API_BASE_URL}/api/v1/scripts/uninstall-terminal.sh?collector_id=${collectorId}" && chmod +x sysarmor-uninstall.sh && sudo ./sysarmor-uninstall.sh`;
                  navigator.clipboard.writeText(fullUninstallCmd);
                }}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
            <div className="bg-black text-red-400 p-3 rounded font-mono text-sm overflow-x-auto">
              {`wget -O sysarmor-uninstall.sh "{API_BASE_URL}/api/v1/scripts/uninstall-terminal.sh?collector_id=${collectorId}" && chmod +x sysarmor-uninstall.sh && sudo ./sysarmor-uninstall.sh`}
            </div>
          </div>
        </div>

        <div className="flex items-start gap-2 p-3 bg-red-100 rounded border border-red-300">
          <div className="w-4 h-4 rounded-full bg-red-500 flex-shrink-0 mt-0.5"></div>
          <div className="text-sm text-red-800">
            <strong>警告：</strong>卸载操作将完全移除 SysArmor
            终端及其所有数据，此操作不可逆。请确保您真的需要卸载该终端。
          </div>
        </div>
      </div>
    </div>
  );
}
