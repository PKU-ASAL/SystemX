# SysArmor Manager UI

<div align="center">

**现代化的端点检测与响应（EDR）系统管理界面**

基于 Next.js 15 和 React 19 构建的企业级安全管理平台前端页面

[![Next.js](https://img.shields.io/badge/Next.js-15.4.5-black?logo=next.js)](https://nextjs.org/)
[![React](https://img.shields.io/badge/React-19.1.0-blue?logo=react)](https://reactjs.org/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5-blue?logo=typescript)](https://www.typescriptlang.org/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-4-38B2AC?logo=tailwind-css)](https://tailwindcss.com/)

</div>

## 📋 项目概述

SysArmor 是一个全功能的端点安全管理平台，专为现代企业和学术研究环境设计。它提供了完整的威胁检测、终端管理、日志分析和系统监控功能，支持无代理部署和多 EDR 系统集成。

### 🎯 设计理念

- **学术友好**: 支持无代理部署，适应学术研究环境的限制
- **工业级**: 企业级安全功能，满足生产环境需求
- **现代化**: 基于最新技术栈，提供流畅的用户体验
- **可扩展**: 模块化架构，支持多种 EDR 解决方案集成

## ✨ 核心功能

### 🛡️ 威胁管理

- **实时告警监控** - 基于 OpenSearch 的智能威胁检测
- **攻击事件分析** - 多维度安全事件可视化（事件、图谱、报告）
- **威胁情报集成** - 高级威胁分析和响应建议
- **安全态势感知** - 全局安全状态监控面板

### 🖥️ 终端管理

- **灵活部署选项**
  - 无代理部署（Agentless）- 适用于学术研究环境
  - SysArmor 完整栈部署 - 企业级全功能保护
  - 第三方 EDR 集成 - 兼容 Wazuh 等主流 EDR 解决方案
- **终端生命周期管理** - 从部署到监控的完整流程
- **批量操作支持** - 高效管理大规模终端环境

### 📊 日志管理与分析

- **智能日志搜索** - 基于 Elasticsearch 的高性能查询
- **实时日志流** - 支持实时日志监控和过滤
- **日志模式分析** - AI 驱动的异常模式识别
- **自定义仪表板** - 可配置的日志分析视图

### ⚙️ 基础设施管理

- **Kafka 集群管理**
  - Broker 状态监控和性能指标
  - Topic 管理和消息流量分析
  - Consumer Group 监控和负载均衡
- **分析工作流引擎** - 基于 Vector 的数据处理管道
- **系统健康监控** - 多组件状态监控（数据库、OpenSearch、Kafka、Prometheus、Vector）

### 🔧 系统功能

- **全局搜索** - 跨平台统一搜索体验
- **用户权限管理** - 基于角色的访问控制
- **系统配置** - 灵活的参数配置界面
- **在线帮助文档** - 集成式用户指南

## 🚀 技术栈

### 前端框架

- **Next.js 15** - 全栈 React 框架，支持 App Router
- **React 19** - 最新的 React 版本，支持并发特性
- **TypeScript 5** - 类型安全的 JavaScript 超集

### UI 组件与样式

- **Tailwind CSS 4** - 原子化 CSS 框架
- **Radix UI** - 无障碍的底层 UI 组件
- **Shadcn/ui** - 现代化的组件库
- **Lucide React** & **Tabler Icons** - 丰富的图标库

### 数据处理与可视化

- **TanStack Table** - 高性能表格组件
- **Recharts** - React 图表库
- **React Hook Form** - 高性能表单处理
- **Zod** - TypeScript 优先的模式验证

### 集成与连接

- **Elasticsearch UI** - 搜索界面组件
- **React DatePicker** - 日期时间选择器
- **DND Kit** - 拖拽功能支持

## 🛠️ 快速开始

### 环境要求

```bash
Node.js >= 18.0.0
pnpm >= 8.0.0 (推荐) 或 npm >= 9.0.0
```

### 安装步骤

1. **克隆项目**

```bash
git clone ssh://git@git.pku.edu.cn/oslab/sysarmor-manager-ui.git
cd sysarmor-manager-ui
```

2. **安装依赖**

```bash
pnpm install
```

3. **环境配置**

```bash
# 复制环境变量模板
cp .env.example .env.local

# 编辑环境变量
vim .env.local
```

4. **启动开发服务器**

```bash
pnpm dev
```

5. **访问应用**
   打开浏览器访问 [http://localhost:3000](http://localhost:3000)

### 环境变量配置

项目目前只需要配置一个环境变量。创建 `.env.local` 文件：

```env
# API 基础路径配置（可选，默认为 /api/v1）
NEXT_PUBLIC_API_BASE_URL=/api/v1
```

**说明**：

- `NEXT_PUBLIC_API_BASE_URL`: API 服务的基础路径
  - 开发环境：通常设置为 `/api/v1`（使用 Next.js API 路由）
  - 生产环境：可设置为完整的 API 服务地址，如 `https://api.sysarmor.com/v1`
  - 如果不设置，默认使用 `/api/v1`

## 📁 项目结构

```
sysarmor-manager-ui/
├── app/                          # Next.js App Router
│   ├── (dashboard)/             # 仪表板路由组
│   │   ├── health/              # 系统健康监控
│   │   ├── kafka/               # Kafka 管理
│   │   │   ├── brokers/         # Broker 管理
│   │   │   ├── topics/          # Topic 管理
│   │   │   └── consumers/       # Consumer 管理
│   │   ├── logs/                # 日志管理
│   │   │   ├── search/          # 日志搜索
│   │   │   └── analysis/        # 日志分析
│   │   ├── opensearch/          # OpenSearch 集成
│   │   │   └── alerts/          # 告警管理
│   │   ├── terminal-create/     # 终端创建
│   │   ├── terminal-list/       # 终端列表
│   │   ├── workflows/           # 工作流管理
│   │   │   ├── create/          # 创建工作流
│   │   │   └── templates/       # 工作流模板
│   │   ├── settings/            # 系统设置
│   │   └── help/                # 帮助文档
│   ├── globals.css              # 全局样式
│   └── layout.tsx               # 根布局
├── components/                   # React 组件
│   ├── ui/                      # 基础 UI 组件
│   ├── app-sidebar.tsx          # 应用侧边栏
│   ├── site-header.tsx          # 站点头部
│   ├── dashboard.tsx            # 仪表板组件
│   ├── kafka-*.tsx              # Kafka 相关组件
│   ├── health-status.tsx        # 健康状态组件
│   └── terminal-*.tsx           # 终端管理组件
├── hooks/                       # 自定义 React Hooks
├── lib/                         # 工具函数和 API
│   ├── api.ts                   # API 客户端
│   └── utils.ts                 # 工具函数
├── docs/                        # 文档
│   ├── openapi.json             # API 规范
│   └── 需求文档_v1.md           # 需求文档
└── public/                      # 静态资源
```

## 🎮 可用脚本

```bash
# 开发
pnpm dev          # 启动开发服务器（使用 Turbopack）
pnpm dev:debug    # 启动调试模式

# 构建
pnpm build        # 构建生产版本
pnpm start        # 启动生产服务器

# 代码质量
pnpm lint         # 运行 ESLint 检查
pnpm lint:fix     # 自动修复 ESLint 问题
pnpm type-check   # TypeScript 类型检查

# 其他
pnpm clean        # 清理构建文件
```

## 🔧 开发指南

### 代码规范

- 使用 TypeScript 进行类型安全开发
- 遵循 ESLint 和 Prettier 配置
- 组件使用函数式组件和 Hooks
- 样式使用 Tailwind CSS 原子类

### 组件开发

```tsx
// 示例组件结构
"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";

interface ComponentProps {
  title: string;
  onAction?: () => void;
}

export function ExampleComponent({ title, onAction }: ComponentProps) {
  const [loading, setLoading] = useState(false);

  return (
    <div className="p-4">
      <h2 className="text-lg font-semibold">{title}</h2>
      <Button onClick={onAction} disabled={loading}>
        {loading ? "处理中..." : "执行操作"}
      </Button>
    </div>
  );
}
```

### API 集成

```typescript
// lib/api.ts 中的 API 调用示例
export async function fetchSystemHealth(): Promise<HealthData> {
  const response = await fetch(`${API_BASE_URL}/health`);
  if (!response.ok) {
    throw new Error("Failed to fetch health data");
  }
  return response.json();
}
```

## 🚀 部署指南

### 生产环境部署

1. **构建应用**

```bash
pnpm build
```

2. **启动生产服务器**

```bash
pnpm start
```

### Docker 部署

```dockerfile
FROM node:18-alpine AS base
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production

FROM node:18-alpine AS build
WORKDIR /app
COPY . .
RUN npm ci && npm run build

FROM node:18-alpine AS runtime
WORKDIR /app
COPY --from=base /app/node_modules ./node_modules
COPY --from=build /app/.next ./.next
COPY --from=build /app/public ./public
COPY --from=build /app/package.json ./package.json

EXPOSE 3000
CMD ["npm", "start"]
```

### 环境配置

不同环境的配置文件：

- `.env.local` - 本地开发
- `.env.development` - 开发环境
- `.env.staging` - 测试环境
- `.env.production` - 生产环境

### 提交规范

```
feat: 新功能
fix: 修复问题
docs: 文档更新
style: 代码格式调整
refactor: 代码重构
test: 测试相关
chore: 构建过程或辅助工具的变动
```

## 📄 许可证

本项目为 SysArmor 团队的专有项目，仅供授权用户使用。

---

<div align="center">
Made with ❤️ by SysArmor Team
</div>
