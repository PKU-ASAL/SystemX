#!/usr/bin/env bash
# Stop a VM environment: vm-endpoint or vm-topology.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
ENV_NAME="${1:-${ENV:-vm-endpoint}}"
BEST_EFFORT="${SYSARMOR_VM_STOP_BEST_EFFORT:-0}"

case "$ENV_NAME" in
  vm-endpoint|vm-topology) ;;
  *) echo "[stop-vm][ERROR] unsupported VM ENV=$ENV_NAME" >&2; exit 2 ;;
esac

env_domains() {
  case "$ENV_NAME" in
    vm-endpoint)
      printf '%s\n' vm-endpoint_node-a
      ;;
    vm-topology)
      printf '%s\n' vm-topology_mgr vm-topology_node-a vm-topology_attacker
      ;;
  esac
}

cleanup_libvirt_domains() {
  local domain
  while IFS= read -r domain; do
    [[ -n "$domain" ]] || continue
    if ! virsh dominfo "$domain" >/dev/null 2>&1; then
      continue
    fi
    echo "[stop-vm] removing stale libvirt domain $domain"
    virsh destroy "$domain" >/dev/null 2>&1 || true
    virsh undefine "$domain" --managed-save --remove-all-storage >/dev/null 2>&1 \
      || virsh undefine "$domain" --managed-save >/dev/null 2>&1 \
      || true
  done < <(env_domains)
}

has_libvirt_domains() {
  local domain
  while IFS= read -r domain; do
    [[ -n "$domain" ]] || continue
    if virsh dominfo "$domain" >/dev/null 2>&1; then
      return 0
    fi
  done < <(env_domains)
  return 1
}

cd "$ROOT/environments/$ENV_NAME"
if [[ "$BEST_EFFORT" == "1" ]]; then
  tmp_log="$(mktemp)"
  if ! vagrant destroy -f >"$tmp_log" 2>&1; then
    echo "[stop-vm][WARN] vagrant destroy failed for ENV=$ENV_NAME; continuing in best-effort cleanup" >&2
    tail -5 "$tmp_log" >&2 || true
    cleanup_libvirt_domains
  else
    cat "$tmp_log"
  fi
  rm -f "$tmp_log"
else
  tmp_log="$(mktemp)"
  if ! vagrant destroy -f >"$tmp_log" 2>&1; then
    echo "[stop-vm][WARN] vagrant destroy failed for ENV=$ENV_NAME; trying libvirt fallback cleanup" >&2
    tail -5 "$tmp_log" >&2 || true
    cleanup_libvirt_domains
    if has_libvirt_domains; then
      echo "[stop-vm][ERROR] libvirt fallback cleanup left domains for ENV=$ENV_NAME" >&2
      rm -f "$tmp_log"
      exit 1
    fi
  else
    cat "$tmp_log"
  fi
  rm -f "$tmp_log"
fi
for net in env0 vagrant-libvirt; do
  virsh net-destroy "$net" 2>/dev/null || true
  virsh net-undefine "$net" 2>/dev/null || true
done
echo "[stop-vm] done ENV=$ENV_NAME"
