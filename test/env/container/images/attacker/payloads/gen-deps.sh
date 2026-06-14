#!/usr/bin/env bash
# 生成 ci-runner 的良性依赖包 deps.tar（内含一个良性 tool 脚本）。
set -euo pipefail
OUT="${1:-.}"
TMP="$(mktemp -d)"
cat > "$TMP/tool" <<'EOF'
#!/usr/bin/env bash
# 良性构建工具：仅打印，不做恶意行为。
case "${1:-}" in
  --build) echo "[tool] building..." ;;
  --push)  echo "[tool] pushing to ${3:-registry}" ;;
  *)       echo "[tool] noop" ;;
esac
EOF
chmod +x "$TMP/tool"
tar cf "$OUT/deps.tar" -C "$TMP" tool
echo "[gen-deps] wrote $OUT/deps.tar"
