# UI热更新开发环境

## 概述

SysArmor UI现在支持热更新开发环境，让你在开发过程中无需重新构建Docker镜像就能看到代码变化。

## 快速开始

### 1. 启动开发环境

```bash
# 启动UI开发环境（支持热更新）
make dev-ui

# 或者直接使用docker compose
cd apps/ui
docker compose -f docker-compose.dev.yml up -d --build
```

### 2. 查看日志

```bash
# 查看开发环境日志
make dev-ui-logs

# 或者直接使用docker compose
cd apps/ui
docker compose -f docker-compose.dev.yml logs -f ui-dev
```

### 3. 停止开发环境

```bash
# 停止UI开发环境
make dev-ui-stop

# 或者直接使用docker compose
cd apps/ui
docker compose -f docker-compose.dev.yml down
```

## 开发环境特性

### ✅ 支持的热更新文件类型

- **React组件**: `components/` 目录下的所有 `.tsx` 文件
- **页面文件**: `app/` 目录下的所有页面组件
- **工具函数**: `lib/` 目录下的工具函数和API客户端
- **自定义Hook**: `hooks/` 目录下的自定义React Hook
- **类型定义**: `types/` 目录下的TypeScript类型文件
- **工具函数**: `utils/` 目录下的工具函数
- **静态资源**: `public/` 目录下的图片、图标等
- **配置文件**: `next.config.ts`, `tsconfig.json`, `postcss.config.mjs` 等

### 🚀 热更新机制

1. **文件监听**: 使用Docker volume挂载源代码目录
2. **自动重编译**: Next.js开发服务器自动检测文件变化
3. **浏览器刷新**: 支持Fast Refresh，保持组件状态
4. **错误显示**: 编译错误会在浏览器中实时显示

### ⚙️ 环境配置

开发环境使用以下配置：

```yaml
environment:
  - NODE_ENV=development
  - WATCHPACK_POLLING=true      # 启用文件轮询
  - CHOKIDAR_USEPOLLING=true    # 启用文件系统轮询
  - NEXT_TELEMETRY_DISABLED=1   # 禁用遥测
```

## 开发工作流

### 1. 日常开发

```bash
# 1. 启动开发环境
make dev-ui

# 2. 打开浏览器访问 http://localhost:3000

# 3. 修改代码文件，保存后自动刷新

# 4. 查看日志（可选）
make dev-ui-logs

# 5. 开发完成后停止
make dev-ui-stop
```

### 2. 调试流程

```bash
# 查看实时日志
make dev-ui-logs

# 如果遇到问题，重启开发环境
make dev-ui-stop
make dev-ui
```

## 文件结构

```
apps/ui/
├── docker-compose.dev.yml    # 开发环境Docker配置
├── Dockerfile.dev           # 开发环境Dockerfile
├── docker-compose.yml       # 生产环境Docker配置
├── Dockerfile              # 生产环境Dockerfile
├── app/                    # 页面文件（热更新）
├── components/             # React组件（热更新）
├── lib/                   # 工具函数（热更新）
├── hooks/                 # 自定义Hook（热更新）
├── types/                 # 类型定义（热更新）
├── utils/                 # 工具函数（热更新）
└── public/                # 静态资源（热更新）
```

## 性能优化

### 1. 文件监听优化

开发环境启用了轮询模式以确保在Docker容器中正常工作：

```yaml
environment:
  - WATCHPACK_POLLING=true
  - CHOKIDAR_USEPOLLING=true
```

### 2. 缓存策略

- **node_modules**: 不挂载，使用容器内的版本
- **.next**: 不挂载，避免构建缓存冲突
- **源代码**: 只读挂载，提高安全性

## 故障排除

### 1. 热更新不工作

```bash
# 检查容器状态
docker ps | grep ui-dev

# 查看日志
make dev-ui-logs

# 重启开发环境
make dev-ui-stop
make dev-ui
```

### 2. 端口冲突

如果3000端口被占用：

```bash
# 修改 .env 文件
UI_PORT=3001

# 重启开发环境
make dev-ui-stop
make dev-ui
```

### 3. 网络连接问题

确保后端服务正在运行：

```bash
# 检查所有服务状态
make status

# 检查API连接
curl http://localhost:3000/api/v1/health
```

## 生产环境切换

开发完成后，切换回生产环境：

```bash
# 停止开发环境
make dev-ui-stop

# 启动生产环境
docker compose up -d ui
```

## 最佳实践

1. **开发时使用**: `make dev-ui` 启动热更新环境
2. **测试时使用**: `docker compose up -d ui` 启动生产环境
3. **代码提交前**: 确保在生产环境中测试通过
4. **长期开发**: 定期重启开发环境清理缓存
5. **性能测试**: 使用生产环境进行性能测试

## 技术实现

### Docker Volume挂载

```yaml
volumes:
  # 源代码目录（只读挂载）
  - ./app:/app/app:ro
  - ./components:/app/components:ro
  - ./lib:/app/lib:ro
  # 配置文件
  - ./next.config.ts:/app/next.config.ts:ro
  # 排除目录
  - /app/node_modules
  - /app/.next
```

### Next.js开发服务器

```dockerfile
# 使用开发服务器命令
CMD ["sh", "-c", "corepack enable pnpm && pnpm dev"]
```

这个配置确保了最佳的开发体验，同时保持了与生产环境的一致性。
