#!/usr/bin/env bash
set -euo pipefail

base_url="${SYSARMOR_OPENSEARCH_URL:-http://opensearch:9200}"
mapping_dir="${SYSARMOR_OPENSEARCH_MAPPING_DIR:-/opt/sysarmor/opensearch/mappings}"
timeout="${SYSARMOR_OPENSEARCH_INIT_TIMEOUT_SECONDS:-120}"
curl_args=(-fsS --max-time 10)
if [[ -n "${SYSARMOR_OPENSEARCH_USERNAME:-}" ]]; then
  curl_args+=(-u "${SYSARMOR_OPENSEARCH_USERNAME}:${SYSARMOR_OPENSEARCH_PASSWORD:-}")
fi

deadline=$((SECONDS + timeout))
until curl "${curl_args[@]}" "${base_url}/_cluster/health" >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "opensearch initialization timed out" >&2
    exit 1
  fi
  sleep 2
done

exists() {
  curl "${curl_args[@]}" -o /dev/null -w '%{http_code}' "${base_url}/$1" 2>/dev/null | grep -Eq '^(200|201)$'
}

for kind in events signals incidents evidence; do
  physical="sysarmor-${kind}-v1"
  read_alias="sysarmor-${kind}-read"
  write_alias="sysarmor-${kind}-write"

  if ! exists "${physical}"; then
    curl "${curl_args[@]}" -X PUT -H 'Content-Type: application/json' \
      --data-binary "@${mapping_dir}/${kind}-v1.json" "${base_url}/${physical}" >/dev/null
  fi

  for alias in "${read_alias}" "${write_alias}"; do
    if exists "_alias/${alias}"; then
      response="$(curl "${curl_args[@]}" "${base_url}/_alias/${alias}")"
      if [[ "${response}" != *"\"${physical}\""* ]]; then
        echo "alias ${alias} does not point to ${physical}" >&2
        exit 1
      fi
      continue
    fi
    action="{\"actions\":[{\"add\":{\"index\":\"${physical}\",\"alias\":\"${alias}\""
    if [[ "${alias}" == "${write_alias}" ]]; then
      action+=',"is_write_index":true'
    fi
    action+='}}]}'
    curl "${curl_args[@]}" -X POST -H 'Content-Type: application/json' \
      --data-binary "${action}" "${base_url}/_aliases" >/dev/null
  done
done

echo "opensearch indices and aliases are ready"
