#!/usr/bin/env bash
# node-a 入口：模拟可被 RCE 的 web 运行时长驻父进程。
# 真实攻击由 `docker exec node-a bash -c '...'` 注入，模拟 RCE 后的子进程链。
# 容器拓扑中 node-a 即"受害端点"，tetragon 监控其谱系。
exec -a java-web bash -c 'while true; do sleep 30; done'
