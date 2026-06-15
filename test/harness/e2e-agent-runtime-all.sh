#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"

echo "[e2e-agent-runtime-all] running local runtime gate"
make -C "$ROOT" e2e-agent-all

echo "[e2e-agent-runtime-all] running container managed detection gate"
make -C "$ROOT" e2e-agent-detection-container-all

echo "[e2e-agent-runtime-all] running real owned-container gate"
make -C "$ROOT" e2e-agent-real-tetragon-owned-container

echo "[e2e-agent-runtime-all] running real owned-vm gate"
make -C "$ROOT" e2e-agent-real-tetragon-owned-vm

echo "[e2e-agent-runtime-all] ok"
