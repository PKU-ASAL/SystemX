# EDR Rule Gap and Rule System Design

本文记录当前 endpoint EDR 规则体系的差距，并给出一套松耦合、代表性足够、可在平稳 CPU 成本下运行的规则设计。目标不是把端侧做成全量审计系统，而是持续产生局部、低成本、可解释的 signal，为后续图关联和调查提供高价值 terminal 与 hopset。

## 1. 当前状态

当前规则主要覆盖一条较窄的杀伤链主干：

```text
web/runtime 执行 shell
  -> curl/wget 下载
  -> payload 写入高风险路径
  -> payload 执行
  -> shell 或 payload 连接已知 C2
```

已有核心规则：

| Rule | 主要行为 | 当前价值 | 主要缺口 |
|---|---|---|---|
| `web_runtime_spawns_shell` | web runtime spawn shell | 初始访问后的本地执行线索 | 依赖父进程/argv 启发式，覆盖面有限 |
| `download_by_lolbin` | curl/wget connect | 下载器 terminal | 只覆盖 curl/wget，未覆盖 python/perl/nc/socat/openssl 等 |
| `payload_dropped` | payload 路径 file.write/chmod | 落盘 terminal | 强依赖路径前缀 |
| `reverse_shell_pattern` | shell connect C2 | C2 terminal | 强依赖 shell binary + C2 IoC |
| `suspicious_exec_connect` | payload 进程 connect C2 | payload C2 terminal | 依赖 payload identity 或路径 state |
| `payload_lifecycle` | drop/exec/connect 组合 | 局部杀伤链证据 | 依赖前序事件完整采集 |
| `credential_file_read` | credential path read | 凭据访问 terminal | 只覆盖固定凭据路径 |

## 2. 主要差距

### 2.1 战术覆盖差距

| ATT&CK 战术 | 当前覆盖度 | 缺口 |
|---|---:|---|
| Initial Access | 低 | 只有 web runtime spawn shell 类启发式 |
| Execution | 中 | 覆盖 curl/wget 下载、payload 执行，但 interpreter/script 变体不足 |
| Persistence | 很低 | 采集可覆盖 cron/systemd 写入，但缺少明确信号规则 |
| Privilege Escalation | 很低 | 缺少 sudo/pkexec/suid/chmod+s/capability 相关信号 |
| Defense Evasion | 很低 | 缺少 history/log 清理、LD_PRELOAD、ptrace、隐藏目录等信号 |
| Credential Access | 中低 | 仅固定 credential path read |
| Discovery | 低 | 暂无独立 discovery 规则 |
| Lateral Movement | 低 | 暂无 SSH/scp/rsync/remote exec 规则 |
| Command and Control | 中 | 已知 C2 + shell/payload connect 有覆盖，未知 C2 弱 |
| Exfiltration | 很低 | 无数据量/敏感文件外联关联 |

### 2.2 规则表达差距

1. **IoC 依赖较重**  
   `reverse_shell_pattern`、`suspicious_exec_connect`、`payload_lifecycle` 主要依赖 `ioc:c2-ip-feed` / `ioc:c2-control-port-feed`。未知 C2 或内网 C2 覆盖不足。

2. **解释器覆盖不足**  
   当前 shell 识别以 sh/bash 为主，python/perl/ruby/node/php/nc/socat/openssl 等无文件执行和反弹通道覆盖不足。

3. **路径锚点偏窄**  
   payload 主要依赖 `/dev/shm/`、攻击测试前缀、plugin 目录等。`~/.local/bin/`、`/opt/`、`/usr/local/share/` 等真实落点尚未体系化。

4. **terminal 与 context 还不够分层**  
   一些信号既承担 terminal 识别，又承担上下文补全，导致规则间耦合较强。

### 2.3 性能差距

当前 matrix 显示：

- `minimal` CPU 低，但攻击覆盖不稳定；
- `balanced` 能捕获 `apt-fileless-c2` 和 `apt-staged-drop` 主干，但 CPU 偏高；
- `deep` 事件量和 CPU 明显偏高，适合作为调查窗口，不适合常开。

关键原因：

1. `process.exec` 对 `/usr/bin/bash` / `/usr/bin/sh` 的采集非常昂贵，系统和测试工具大量使用 shell；
2. `network.connect` 中保留 shell/interpreter binary 是相对低成本的，因为 connect 频率远低于 exec；
3. `file.read/open` 只能用于固定高价值路径，不能 broad read/open 常开；
4. Tetragon 单 selector 内 binary 与 socket/file 条件是 AND，复杂 OR 表达需要拆多 selector 或留给后续增强。

## 3. 设计原则

### 3.1 Lineage 是一等公民

所有事件和信号必须保留：

- `lineage_id`
- `process.stable_id`
- `parent.stable_id`
- `process.binary`
- `process.argv`
- `file.path` 或 `socket.addr/socket.port`

规则输出必须包含 `EventRefs` 和 `Entities`，terminal 信号必须带 EvidenceBundle。即使常开 policy 不采全量 process.exec，也要确保 terminal 的事件本身足以定位 subject process 和 object entity。

### 3.2 用选择性省成本，不砍维度

端侧常开规则不追求全量 provenance，而是采集：

1. **高价值 terminal**：C2 connect、payload write/chmod、credential read、persistence write；
2. **少量局部上下文**：payload path exec、shell/interpreter network connect、credential/persistence path file operation；
3. **调查窗口再打开 deep hopset**：fork/exit/broad exec/open 只在 incident-deep 中短期开启。

### 3.3 规则松耦合

每条规则只负责一个清晰语义：

- terminal 规则：判断“这个事件本身是否高风险”；
- context 规则：补全文件、进程、网络节点特征；
- lifecycle 规则：仅组合已有 terminal/context，不重新承担底层检测逻辑。

这样单条规则失效不会拖垮整条链路。

### 3.4 CPU budget 优先级

常开 `balanced` 规则必须满足：

1. 不 broad collect shell/interpreter `process.exec`；
2. `network.connect` 必须有 socket addr/port 或 binary 限定；
3. `file.read/write/chmod/open` 必须有 path prefix 限定；
4. `process.fork/exit` 不进入 balanced；
5. high-frequency behavior 没有 pushdown selector 时必须 hard warning 或拒绝。

## 4. 推荐规则族

### 4.1 Terminal: External Fetch by LOLBin or Interpreter

**规则名**：`external_fetch_by_lolbin`

**目标**：发现下载器行为。

**事件**：`network.connect`

**常开采集**：

- process binary prefix: curl/wget/python/perl/ruby/node/php/nc/socat/openssl/bash/sh
- socket: known C2 addr/port 或 suspicious egress port

**匹配逻辑**：

- curl/wget connect 到 C2 或外部下载端口；
- interpreter argv 中包含 URL、`/dev/tcp`、`urllib`、`requests`、`LWP::Simple`、`Net::HTTP` 等下载语义；
- 输出 socket entity + process entity。

**CPU 约束**：只基于 `network.connect`，不要求 broad exec。

### 4.2 Terminal: Suspicious Payload Drop

**规则名**：`payload_dropped`

**目标**：发现高风险落盘。

**事件**：`file.write`, `file.chmod`

**常开采集**：

- file path prefix: payload path、persistence path、plugin path、tmp attack prefix；
- 可选 binary prefix 不作为必要条件。

**匹配逻辑**：

- 写入或 chmod 高风险路径；
- 文件扩展名或 shebang/argv 后续可补充分数，但不作为必要条件。

**CPU 约束**：必须 file.path.prefix 下推。

### 4.3 Terminal: Reverse Shell or Interactive C2 Channel

**规则名**：`reverse_shell_pattern`

**目标**：发现 shell/interpreter 直接连接 C2 或疑似外联控制通道。

**事件**：`network.connect`

**常开采集**：

- process binary prefix: sh/bash/dash/zsh/ksh/ash/python/perl/ruby/node/php/nc/socat/openssl
- socket: C2 addr/port 或 suspicious egress port

**匹配逻辑**：

- shell 连接已知 C2，terminal；
- interpreter argv 含 `/dev/tcp`、`socket`、`subprocess`、`pty.spawn`、`os.dup2` 等反弹 shell 特征，terminal；
- nc/socat/openssl 连接 C2 且 argv 含 exec/sh/pty/pipe 语义，terminal。

**CPU 约束**：只采 network.connect，不采所有 interpreter exec。

### 4.4 Terminal: Persistence Write

**规则名**：`persistence_artifact_write`

**目标**：发现持久化植入。

**事件**：`file.write`, `file.chmod`

**常开采集**：

- `/etc/cron*`
- `/etc/systemd/system`
- `/etc/init.d`
- `/etc/rc*.d`
- `/etc/ld.so.preload`
- `/root/.ssh/authorized_keys`
- `/home/*/.ssh/authorized_keys`
- `/etc/profile`, `/etc/profile.d/`, shell rc files

**匹配逻辑**：

- 非 trusted admin binary 写入；
- 写入后 chmod executable 或 systemctl/cron reload 可提升严重度；
- 输出 file entity + process entity。

**CPU 约束**：仅 path prefix file.write/chmod。

### 4.5 Terminal: Credential or Secret Read

**规则名**：`credential_file_read`

**目标**：发现凭据访问。

**事件**：`file.read`, `file.open`

**常开采集**：

- `/etc/shadow`, `/etc/sudoers`
- `/root/.ssh/`, `/home/*/.ssh/`
- `/run/secrets/`, `/var/run/secrets/`
- `/var/lib/kubelet/pods/*/secrets`
- `/proc/*/environ` 可进入 deep 或高风险模式

**匹配逻辑**：

- 非 trusted admin binary 读取；
- 与 payload lineage 或 C2 lineage 相邻时提高 severity；
- 对相同 process/path suppress，避免重复刷屏。

**CPU 约束**：只 path prefix read，不 broad open。

### 4.6 Terminal: Privilege Escalation Artifact

**规则名**：`privilege_escalation_artifact`

**目标**：发现提权相关文件/权限变化。

**事件**：`file.chmod`, `file.write`, 可选 `process.exec`

**常开采集**：

- chmod/write 高风险路径；
- file access mode 中出现 suid/sgid 需要后端支持时再启用；
- sudo/pkexec exec 可放入 deep 或按 argv 限定启用。

**匹配逻辑**：

- chmod 后文件可执行且位于 payload/persistence 路径；
- 写 `/etc/sudoers` 或 `/etc/sudoers.d/`；
- `pkexec`, `sudo`, `su` 由非交互 shell/payload lineage 触发。

**CPU 约束**：优先 file path；不要常开全量 sudo exec。

### 4.7 Context: Local Discovery Burst

**规则名**：`local_discovery_burst`

**目标**：低优先级补充 discovery 上下文，不直接 terminal。

**事件**：`process.exec`

**采集建议**：balanced 默认不开。deep 或 incident window 开。

**匹配逻辑**：

- 同 lineage 短窗口内出现 `id`, `whoami`, `uname`, `hostname`, `ps`, `ip`, `ss`, `netstat`, `find`, `ls` 等多个 discovery 命令；
- 只有当同 lineage 已存在 payload/C2/credential terminal 时才作为 context signal 输出。

**CPU 约束**：禁止常开 broad exec。

### 4.8 Context: Lateral Movement Attempt

**规则名**：`lateral_movement_attempt`

**目标**：发现 SSH/scp/rsync 远程移动。

**事件**：`network.connect`, 可选 `process.exec`

**采集建议**：

- network.connect binary prefix: ssh/scp/sftp/rsync
- socket port: 22/2222/3389/5985/5986 等

**匹配逻辑**：

- payload lineage 或 credential-read lineage 后出现内网 SSH；
- connect 到 private subnet 且 argv 指向非标准 key 文件；
- 输出 process + socket entity。

**CPU 约束**：network.connect + port/binary 限定，默认低成本。

### 4.9 Lifecycle: Attack Chain Correlator

**规则名**：`payload_lifecycle`

**目标**：组合 terminal/context，形成可解释局部子图。

**输入**：

- `external_fetch_by_lolbin`
- `payload_dropped`
- `reverse_shell_pattern`
- `persistence_artifact_write`
- `credential_file_read`
- `lateral_movement_attempt`

**匹配逻辑**：

- 同 lineage 或 parent/child stable_id 近邻；
- 同 file entity 或 process stable_id；
- 时间窗口可配置；
- 输出 EvidenceBundle，包含所有 event_refs 和 entities。

**CPU 约束**：纯内存状态机，不增加 sensor 采集量。

## 5. Policy 分层建议

### 5.1 minimal-high-signal

只保留最确定 terminal：

- `network.connect`: shell/interpreter/lolbin -> C2 addr/port
- `file.write/chmod`: payload/persistence path
- 可选 `file.read`: credential path（如 CPU 允许）

不采：

- broad process.exec
- process.fork/exit
- broad file.open/read

### 5.2 edr-balanced

目标是常开、CPU 平稳、代表性足够：

- minimal 全部内容；
- `network.connect`: 扩展到 interpreter/lolbin/shell + C2/suspicious port；
- `file.read`: credential/secret path；
- `file.write/chmod`: payload/persistence path；
- `process.exec`: 仅 payload path 和明确高风险路径，不常开 `/usr/bin/bash` / `/usr/bin/sh`。

关键建议：**从 balanced 的 process.exec 中移除 `/usr/bin/bash` 和 `/usr/bin/sh`，但保留在 network.connect 中**。这样仍能抓反弹 shell，同时避免 shell exec 风暴。

### 5.3 incident-deep

只在调查窗口打开：

- balanced 全部内容；
- broad process.exec；
- process.fork/exit；
- file.open/read 覆盖 `/etc/`, `/run/secrets/`, credential paths；
- discovery/lateral movement context rules。

deep 不追求低 CPU，追求短时完整 hopset。

## 6. Ground Truth 和场景设计建议

场景标签必须只标端侧能低成本、局部、可解释检出的 signal。

### 6.1 标签原则

- `required` 只给稳定可采集事件；
- 不把引擎内部状态（例如 terminal true/false）作为 required 条件；
- required signal 应该对应 terminal，而不是上下文补全；
- context signal 可以 optional；
- benign 场景不能触碰 C2、payload path、persistence path、credential path。

### 6.2 需要新增的代表性场景

| Scenario | 覆盖规则 | 目的 |
|---|---|---|
| `apt-python-fileless-c2` | interpreter reverse shell | 覆盖非 bash 无文件反弹 |
| `apt-persistence-systemd` | persistence_artifact_write | 覆盖 systemd 持久化 |
| `apt-credential-then-c2` | credential_file_read + C2 | 覆盖凭据访问后外联 |
| `apt-ssh-lateral` | lateral_movement_attempt | 覆盖横向移动 |
| `benign-package-install` | false positive guard | 合法 curl/wget/package manager 下载不应触发 C2 signal |
| `benign-admin-ssh` | false positive guard | 合法 ssh/rsync 不应被当作横向移动 |

## 7. 落地路线

### Phase 1: 先压 balanced CPU

1. 从 balanced `process.exec` 去掉 `/usr/bin/bash`、`/usr/bin/sh`；
2. 保留它们在 `network.connect`；
3. 清理测试环境中 benign 场景期间 curl C2 的后台进程；
4. 修复 Tetragon live policy 切换后偶发缺 network.connect 的 timing 问题；
5. 重跑 3x3 matrix。

### Phase 2: 补 terminal 规则

1. 新增 `persistence_artifact_write`；
2. 扩展 `download_by_lolbin` 到 interpreter/transfer tools；
3. 扩展 `reverse_shell_pattern` 到 interpreter/nc/socat/openssl；
4. 对未知 C2 增加 suspicious egress port + suspicious binary 组合。

### Phase 3: 补 context 和 lifecycle

1. 新增 `lateral_movement_attempt`；
2. 新增 `local_discovery_burst`，只在已有 terminal 邻域内输出；
3. 增强 `payload_lifecycle`，以 terminal 为锚点合并局部 hopset。

### Phase 4: 扩展评测矩阵

1. 攻击场景至少覆盖 fileless、staged、persistence、credential、lateral；
2. benign 场景覆盖 CI、本地构建、包安装、管理员 SSH；
3. matrix 默认保持小而稳定，stress/cost 单独跑。

## 8. 成功标准

| 指标 | balanced 目标 | deep 目标 |
|---|---:|---:|
| steady CPU | < 10% | 不强约束，调查窗口可接受 |
| workload CPU | < 10-15% | 可接受 30%+ 短时 |
| dropped events | 0 | 0 |
| parse errors | 0 | 0 |
| malicious signal recall | > 0.8 | > 0.9 |
| benign terminal false positives | 0 | 0 或极低 |
| signal explainability | 每个 terminal 有 entities + event_refs + evidence | 同左 |

## 9. 当前判断

当前规则体系已经能验证一条主干攻击链，但还不足以覆盖“大部分攻击场景”。它更像一个高质量 MVP：

- 对 `curl/wget + payload drop + bash C2` 覆盖较好；
- 对 persistence、privilege escalation、defense evasion、lateral movement、exfiltration 覆盖不足；
- CPU 目标还未达成，balanced 的 shell exec 采集需要收敛；
- 测试环境需要清理，避免 benign 场景触碰 C2。

下一步应先把 balanced CPU 压稳，再扩展规则族。否则规则越补越多，CPU 会先失控。
