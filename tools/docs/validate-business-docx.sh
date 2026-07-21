#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DOCX="${1:-}"

if [[ -z "$DOCX" ]]; then
  echo "用法：bash tools/docs/validate-business-docx.sh <docx>" >&2
  exit 2
fi

if [[ ! -s "$DOCX" ]]; then
  echo "[business-docx][ERROR] DOCX 不存在或为空：$DOCX" >&2
  exit 1
fi

command -v unzip >/dev/null 2>&1 || {
  echo "[business-docx][ERROR] unzip 未安装。" >&2
  echo "修复：Ubuntu/Debian 执行 sudo apt install unzip" >&2
  exit 1
}

unzip -t "$DOCX" >/dev/null
python3 "$ROOT/tools/docs/business_docx.py" validate "$DOCX"
echo "[business-docx] 结构校验通过：$DOCX"
