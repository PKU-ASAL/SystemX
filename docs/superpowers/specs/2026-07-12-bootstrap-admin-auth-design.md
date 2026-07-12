# Bootstrap Admin Authentication Design

## 目的与结论

SysArmor 第一版提供一个部署时配置的 bootstrap admin，供本地和小规模部署登录 Manager UI。认证不是 SysArmor 的核心业务，因此本期不建立用户、身份映射或 Session 数据表。

浏览器认证由 Auth.js 负责，Next.js 作为 BFF 调用 Manager。Manager 只验证 BFF 签发的短期内部 JWT，并将声明归一为现有 `Principal`。业务 API 不感知密码、Cookie、Auth.js 或未来的 OIDC Provider。

## 范围

本期实现：

- 一个固定主体 `bootstrap-admin`，租户为 `default`，角色为 `admin`。
- Auth.js Credentials 登录和加密 HttpOnly Cookie Session。
- 登录页、退出操作以及未登录页面保护。
- Next.js 服务端 BFF，为 Manager 请求签发五分钟有效的 RS256 内部 JWT。
- Manager 只信任配置的内部 JWT issuer、audience 和公钥。
- bootstrap 密码文件、Auth.js Session secret 和 BFF JWT 密钥的部署配置。
- 认证失败限速、通用错误响应和必要安全日志。
- UI API 调用统一经过 BFF，不向浏览器暴露 Manager 地址或凭据。

本期不实现：

- `users`、`identities`、`accounts` 或 `sessions` 表。
- 多个本地用户、注册、密码重置和用户管理。
- OIDC Provider 配置、账号关联或 Provider Token 持久化。
- API Key。CLI 自动化身份另行设计；bootstrap 密码不得用于 CLI。
- 单个 Session 的远程撤销和登录设备管理。

## 身份模型

系统不定义 `LocalUser` 或 `OIDCUser`。认证来源是认证边界的实现细节，不进入业务模型。

第一版唯一的交互式主体为：

```text
subject:  bootstrap-admin
tenant:   default
roles:    [admin]
```

Manager 延续统一请求身份：

```go
type Principal struct {
    Subject  string
    TenantID string
    Roles    []string
}
```

Manager 授权只读取 `Principal`，不读取 Cookie、用户名或认证模式。

未来 OIDC 接入时，Auth.js 使用 `(issuer, subject)` 形成稳定 `Principal.Subject`，再进入相同的 Session、BFF JWT 和 Manager 授权链路。邮箱只用于显示，不能作为稳定身份键。

## 组件边界

### Auth.js

Auth.js 只负责浏览器身份边界：

- Credentials Provider 接收用户名和密码。
- 从只读 Secret 文件加载 bootstrap 用户名和密码。
- 使用恒定时间比较，用户名错误和密码错误返回相同结果。
- 创建加密、HttpOnly、SameSite Cookie Session。
- 处理 CSRF、登录、注销和 Session 过期。
- Session 中只保存固定主体、租户和角色，不保存输入密码。

不配置 Auth.js Adapter，因此不会创建任何认证表。

### Next.js BFF

BFF 是浏览器访问 Manager 的唯一入口：

- 在服务端验证 Auth.js Session。
- 未登录请求返回统一 `401` JSON 错误。
- 为每次上游调用签发最长五分钟的 RS256 JWT。
- 将 `sub`、`tenant_id` 和 `roles` 从受信任 Session 映射到 JWT。
- 将 JWT 放入服务端到 Manager 的 `Authorization` 请求头。
- 透传 Manager 的成功 JSON，并规范化网络故障和非 JSON 错误。

浏览器不能读取内部 JWT、BFF 私钥或 Manager 内部地址。

### Manager

Manager 保持单一职责：

- 验证 RS256 签名、issuer、audience、expiration 和必要声明。
- 创建请求级 `Principal`。
- 根据 tenant 和 role 授权。
- `/healthz` 保持匿名，其余 `/api/` 路径 fail closed。

删除 `local|oidc` 这种认证来源模式。Manager 只配置一个受信任内部 issuer；OIDC 以后仍由 Auth.js 消化，不改变 Manager。

### CLI 与 Agent

- Agent 继续使用 mTLS，与浏览器用户认证完全隔离。
- bootstrap 密码只允许用于 UI 登录。
- 当前 `sysarmorctl auth token` 在 BFF 上线后从正常部署流程移除；测试所需签名能力保留在测试 helper，不形成生产登录路径。
- 可撤销 API Key 属于后续独立设计，不在本期顺带实现。

## 数据流

登录：

```text
Browser -> Auth.js Credentials -> password Secret validation
        -> encrypted HttpOnly Session Cookie
```

业务请求：

```text
Browser -> Next.js BFF -> validate Session -> sign 5-minute JWT
        -> Manager middleware -> Principal -> API authorization
```

注销：

```text
Browser -> Auth.js signOut -> delete Session Cookie
```

未来 OIDC：

```text
OIDC Provider -> Auth.js -> same Session -> same BFF JWT -> same Principal
```

## 配置与 Secret

部署提供以下文件路径：

```text
SYSARMOR_BOOTSTRAP_ADMIN_USERNAME_FILE
SYSARMOR_BOOTSTRAP_ADMIN_PASSWORD_FILE
AUTH_SECRET_FILE
SYSARMOR_BFF_JWT_PRIVATE_KEY_FILE
SYSARMOR_MANAGER_JWT_PUBLIC_KEY_FILE
SYSARMOR_MANAGER_JWT_ISSUER=sysarmor-bff
SYSARMOR_MANAGER_JWT_AUDIENCE=sysarmor-manager
```

用户名也通过文件提供，使凭据配置统一由 Secret 管理。Secret 内容不得进入镜像、Git、命令行参数或日志。启动时缺少文件、文件为空、权限不安全或密钥不合法必须立即失败。

密码轮换通过替换 Secret 并重启 UI 完成。Session 派生密钥同时包含认证 Secret 和 bootstrap 凭据版本摘要，因此凭据轮换后旧 Session 全部失效。摘要不能记录或暴露。

本地初始化工具生成随机初始密码和密钥文件，并只在首次创建时将密码显示给终端。重复执行不得覆盖现有 Secret。

## 安全与错误处理

- 登录错误统一为“用户名或密码错误”，不暴露用户名是否存在。
- 登录端点按来源地址和固定账号进行限速。第一版本地单实例使用进程内有界限速器；多实例或公网部署必须在反向代理层增加限速。
- Cookie 在生产启用 `Secure`、`HttpOnly` 和 `SameSite=Lax`；开发环境允许 HTTP，但不能放宽 HttpOnly。
- BFF JWT 固定使用 RS256、明确 issuer/audience、五分钟过期时间，并拒绝缺失声明。
- UI 面向浏览器的错误采用统一 JSON envelope；认证内部原因只写安全日志，不返回客户端。
- 日志不得记录密码、Session Cookie、Authorization header 或 JWT 正文。
- BFF 转发只允许预定义 Manager API 路径和方法，不接受任意目标 URL，避免开放代理。

## Session 取舍

Auth.js 使用无数据库的加密 Cookie Session。它满足单 bootstrap admin 的登录、过期和注销需求，并避免引入用户与 Session 持久化。

本期接受以下限制：

- 不能只撤销某一个 Session。
- 不能查看登录设备列表。
- 全局撤销通过轮换 Auth.js secret、bootstrap 凭据或其版本完成。

当且仅当出现逐 Session 撤销或设备管理需求时，再优先引入 Redis Session，而不是提前建立 PostgreSQL 用户体系。

## 测试与验收

单元测试覆盖：

- 正确和错误 bootstrap 凭据。
- 恒定的通用登录失败结果。
- 未登录 BFF 请求返回 `401`。
- 已登录请求生成具有正确声明和五分钟上限的 JWT。
- Manager 接受有效 BFF JWT，拒绝错误 issuer、audience、签名和过期 JWT。
- BFF 不允许任意路径、方法或目标地址。
- 密码轮换导致旧 Session 失效。
- 登录限速达到阈值后拒绝请求。

集成验收覆盖：

- 全新部署生成 Secret 后，bootstrap admin 能登录 UI 并读取 Manager 数据。
- 浏览器网络请求中不存在 Manager JWT、私钥和 bootstrap 密码。
- 注销后受保护页面和 BFF API 不可访问。
- 不提供凭据或 JWT 配置时服务 fail fast，而不是匿名运行。
- PostgreSQL schema 中没有用户、身份、账号或 Session 表。
- `make doctor` 验证认证 Secret、UI、BFF 和 Manager 受保护 API 链路。

## 演进规则

未来增加 OIDC 时，只增加 Auth.js Provider 和声明到内部身份的映射，不改变 Manager API 的认证协议。第一版仍不自动创建用户表；角色优先从可信 OIDC group 配置映射。

只有出现 SysArmor 内部用户属性、独立授权、负责人引用或单用户禁用需求时，才设计 `users` 与 `identities`。只有出现逐 Session 撤销需求时，才设计服务端 Session 存储。这些能力必须分别立项，不能借 OIDC 接入顺带引入。
