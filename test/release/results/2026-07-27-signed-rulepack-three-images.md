# 签名策略包三镜像 Release 验收记录

## 结论

提交 `9c2d08f0540893238fcd4f083d684e4dfd0c6a41` 构建的 standalone
签名策略包在 Ubuntu 22.04、Ubuntu 24.04 和 Debian 12 上完成验收：安装和启动正常，
五个攻击场景全部检出，namespace/self 隔离通过，全部 Signal EventRef 可解析。

本轮不将矩阵通过等同于正式 Release Go：默认启用 `credential_file_read` 后，每镜像新增
4 至 8 条 Signal，其中 1 至 2 条来自测试自身的 `/healthz` curl 读取 `/etc/passwd`，
使 Precision 降至 0.9091 至 0.9474；health 中 detection `lastApplyStatus` 也为
`degraded`。两项均在发布前如实保留。

## 制品

- 日期：2026-07-27（Asia/Shanghai）
- 源码提交：`9c2d08f0540893238fcd4f083d684e4dfd0c6a41`
- Release 版本：`local-rulepack-final`
- 包 SHA-256：`5f84c0a926c595559eeaf48c772b6a9669e68f66bcaa0c339b95ef7cc37e2463`
- 内容签名 key ID：`acceptance-final-20260727`
- 默认内容数量：9
- Sensor：Tetragon `v1.7.0`
- Scope：`namespace/self`

签名私钥只存在于本地临时目录，未加入仓库。三个镜像使用同一包和同一 `install.sh`，
构建均启用 `--no-cache`。

## 方法

Ubuntu 22.04 和 Ubuntu 24.04 使用：

```bash
URL=http://127.0.0.1:18080/install.sh \
RUN_ID=local-rulepack-final-three-images \
FRESH_DOWNLOAD=1 RELEASE_PROXY_URL= \
IMAGES='ubuntu2204 ubuntu2404 debian12' \
bash test/release/run.sh
```

Debian 首次在拉取 `debian:12` manifest 时遇到 Docker Hub IPv6 连接超时，尚未进入
SysArmor 安装。保留首次失败日志后，使用完全相同制品无缓存重试：

```bash
URL=http://127.0.0.1:18080/install.sh \
RUN_ID=local-rulepack-final-debian12-retry \
FRESH_DOWNLOAD=1 RELEASE_PROXY_URL= \
IMAGES='debian12' \
bash test/release/run.sh
```

每个镜像验证安装、签名默认内容加载、Agent/Tetragon health、五个攻击场景、完整
Event/Signal 引用，以及 sibling container 和 host marker 隔离。

## 结果

| 镜像 | Health | 场景 | 隔离 | Signal | Credential | Precision | Recall |
| --- | --- | ---: | --- | ---: | ---: | ---: | ---: |
| Ubuntu 22.04 | 通过 | 5/5 | 通过 | 22 | 7 | 0.9091 | 1.0000 |
| Ubuntu 24.04 | 通过 | 5/5 | 通过 | 19 | 4 | 0.9474 | 1.0000 |
| Debian 12 | 通过 | 5/5 | 通过 | 23 | 8 | 0.9130 | 1.0000 |

三个 health 快照均满足：

- 顶层 `status=ok`
- Tetragon running 且 collection policy loaded
- `defaultManifestVersion=local-rulepack-final`
- 9 个内容 ref 均带 version 和 digest
- `scope.type=namespace` 且 `scope.selector=self`

三个 health 的 detection `lastApplyStatus=degraded`，矩阵现有 ready 断言只检查顶层状态、
scope 和 sensor 状态，因此该字段没有使测试失败。

## Signal 分布

| Rule ID | Ubuntu 22.04 | Ubuntu 24.04 | Debian 12 |
| --- | ---: | ---: | ---: |
| `credential_file_read` | 7 | 4 | 8 |
| `download_by_lolbin` | 3 | 3 | 3 |
| `payload_dropped` | 4 | 4 | 4 |
| `payload_lifecycle` | 2 | 2 | 2 |
| `reverse_shell_pattern` | 1 | 1 | 1 |
| `suspicious_exec_connect` | 2 | 2 | 2 |
| `web_runtime_spawns_shell` | 3 | 3 | 3 |

攻击链相关六条规则在三个镜像上的数量完全一致。`credential_file_read` 的差异来自
curl 和 Bash 读取 `/etc/passwd`；当前规则明确要求 curl、Shell 和未知程序保持敏感，
因此这些 Signal 没有被 sudo 基线放行。其中 `/healthz` curl 分别贡献 2、1、2 条无攻击
marker 的 FP。

当前统计脚本将引用 Event 中含 `sysarmor-` marker 的 Signal 计为 TP。由于部分
credential Signal 与攻击请求共享 marker，本报告保留脚本原始 Precision，同时不把它解释为
credential 规则本身的独立精度。

## Evidence

- 三个镜像缺失 EventRef：均为 0
- 五个声明攻击场景：均命中预期 rule、severity、behavior 和端口
- sibling container marker：未采集
- host marker：未采集
- Debian 首次失败：基础镜像 registry 连接超时，未创建业务容器

生成证据不提交：

- `test/.results/release/local-rulepack-final-three-images/`
- `test/.results/release/local-rulepack-final-debian12-retry/`

## 回归

最终提交通过：

```text
go test ./... -count=1
go test ./internal/sensors/linux/tetragon ./internal/endpoint/normalize ./internal/endpoint/detection -count=1
bash test/suites/product/endpoint/standalone-release-package.sh
bash test/suites/product/endpoint/unified-agent-installation.sh
bash test/suites/product/endpoint/release-container-e2e-contract.sh
bash test/release/test-assert.sh
```

sudo 基线测试覆盖直接命令、绝对路径、`-u/--user`、未知选项、sudoedit、Shell 伪装、
引号和 Unicode 空白参数边界；只有边界可信且实际目标 basename 为 `sysarmorctl` 时放行。
