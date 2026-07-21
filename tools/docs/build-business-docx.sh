#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE="${1:-$ROOT/docs/business/sysarmor-project-proposal.zh-CN.md}"
OUTPUT="${2:-$ROOT/dist/docs/sysarmor-project-proposal.zh-CN.docx}"
TEMPLATE="$ROOT/docs/business/templates/project-proposal-reference.docx"
FILTER="$ROOT/tools/docs/business-docx.lua"
FORMATTER="$ROOT/tools/docs/business_docx.py"
CONTRACT="$ROOT/tools/docs/business_docx_contract.py"
VALIDATOR="$ROOT/tools/docs/validate-business-docx.sh"
TOC_UPDATER="$ROOT/tools/docs/update-business-toc.py"
UNO_PYTHON="${UNO_PYTHON:-/usr/bin/python3}"
WORK_DIR=""

cleanup() {
  [[ -z "$WORK_DIR" ]] || rm -rf -- "$WORK_DIR"
}

fail() {
  echo "[business-docx][ERROR] $1" >&2
  [[ -z "${2:-}" ]] || echo "修复：$2" >&2
  exit 1
}

require_command() {
  local command="$1"
  local repair="$2"
  command -v "$command" >/dev/null 2>&1 || fail "$command 未安装。" "$repair"
}

require_pandoc_version() {
  local version major minor
  version="$(pandoc --version | head -n 1 | awk '{print $2}')"
  IFS=. read -r major minor _ <<< "$version"
  if [[ ! "$major" =~ ^[0-9]+$ || ! "$minor" =~ ^[0-9]+$ ]] \
    || (( major < 2 || (major == 2 && minor < 9) )); then
    fail "Pandoc 版本必须不低于 2.9，当前为 ${version:-未知}。" \
      "Ubuntu/Debian 请安装新版 pandoc，或从 pandoc.org 获取官方发行包"
  fi
}

preflight() {
  require_command pandoc "Ubuntu/Debian 执行 sudo apt install pandoc"
  require_pandoc_version
  require_command ffmpeg "Ubuntu/Debian 执行 sudo apt install ffmpeg"
  require_command python3 "Ubuntu/Debian 执行 sudo apt install python3"
  require_command zip "Ubuntu/Debian 执行 sudo apt install zip"
  require_command unzip "Ubuntu/Debian 执行 sudo apt install unzip"
  require_command xmllint "Ubuntu/Debian 执行 sudo apt install libxml2-utils"
  require_command libreoffice "Ubuntu/Debian 执行 sudo apt install libreoffice-writer python3-uno"
  [[ -x "$UNO_PYTHON" ]] || fail "找不到系统 Python：$UNO_PYTHON" "设置 UNO_PYTHON 为可导入 uno 模块的 Python 路径"
  "$UNO_PYTHON" -c 'import uno' >/dev/null 2>&1 \
    || fail "$UNO_PYTHON 无法导入 LibreOffice UNO 模块。" "Ubuntu/Debian 执行 sudo apt install python3-uno"
  [[ -f "$SOURCE" ]] || fail "建议书源文件不存在：$SOURCE"
  [[ -f "$TEMPLATE" ]] || fail "Word 参考模板不存在：$TEMPLATE" "重新检出仓库或生成 reference.docx"
  [[ -f "$FILTER" && -f "$FORMATTER" && -f "$CONTRACT" && -f "$VALIDATOR" && -f "$TOC_UPDATER" ]] || fail "DOCX 构建工具不完整。"
}

rasterize_diagrams() {
  local target_dir="$1"
  local name
  mkdir -p "$target_dir/diagrams"
  for name in \
    dynamic-game-policy-loop \
    endpoint-cloud-collaboration \
    efficiency-balance-information-refinement \
    technical-roadmap-nsfc-overview; do
    local source="$ROOT/docs/business/diagrams/$name.svg"
    local target="$target_dir/diagrams/$name.png"
    [[ -f "$source" ]] || fail "技术路线图不存在：$source"
    ffmpeg -loglevel error -y -i "$source" -vf "scale=2400:-1:flags=lanczos" -frames:v 1 "$target" \
      || fail "无法转换技术路线图：$source" "确认 ffmpeg 已启用 librsvg"
    python3 "$FORMATTER" png-dpi "$target"
  done
}

build_docx() {
  local work_dir="$1"
  local source_dir
  source_dir="$(cd "$(dirname "$SOURCE")" && pwd)"
  pandoc "$SOURCE" \
    --from=markdown \
    --standalone \
    --toc \
    --toc-depth=3 \
    --reference-doc="$TEMPLATE" \
    --lua-filter="$FILTER" \
    --resource-path="$work_dir:$source_dir" \
    --output="$work_dir/raw.docx"
  python3 "$FORMATTER" finalize --input "$work_dir/raw.docx" --output "$work_dir/final.docx"
  "$UNO_PYTHON" "$TOC_UPDATER" "$work_dir/final.docx"
  python3 "$FORMATTER" update-fields "$work_dir/final.docx"
  bash "$VALIDATOR" "$work_dir/final.docx"
}

main() {
  preflight
  local output_dir
  output_dir="$(dirname "$OUTPUT")"
  mkdir -p "$output_dir"
  WORK_DIR="$(mktemp -d "$output_dir/.business-docx.XXXXXX")"
  trap cleanup EXIT
  rasterize_diagrams "$WORK_DIR"
  build_docx "$WORK_DIR"
  chmod 0644 "$WORK_DIR/final.docx"
  mv "$WORK_DIR/final.docx" "$OUTPUT"
  echo "[business-docx] 已生成：$OUTPUT"
}

main "$@"
