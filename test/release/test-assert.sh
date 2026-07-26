#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
ln -s "$HERE/testdata/fake-docker.sh" "$TMP/docker"

run_assert() {
  PATH="$TMP:$PATH" FAKE_MARKER="${FAKE_MARKER:-marker-1}" DETECTION_TIMEOUT=0 ISOLATION_TIMEOUT=0 \
    "$HERE/assert.sh" "$@"
}

for scenario in web-runtime-shell download-by-lolbin reverse-shell suspicious-exec-connect payload-lifecycle; do
  run_assert detected fake "$scenario" marker-1 "$TMP/$scenario.jsonl"
done

for case_name in wrong-severity wrong-terminal missing-ref missing-behavior wrong-port; do
  if FAKE_CASE="$case_name" run_assert detected fake payload-lifecycle marker-1 "$TMP/$case_name.jsonl" >/dev/null 2>&1; then
    echo "assertion accepted invalid case: $case_name" >&2
    exit 1
  fi
done

run_assert absent fake external-marker "$TMP/absent.jsonl"
if FAKE_CASE=external-marker FAKE_MARKER=external-marker run_assert absent fake external-marker "$TMP/present.jsonl" >/dev/null 2>&1; then
  echo "isolation assertion accepted external marker" >&2
  exit 1
fi

echo "[release-assert-test] ok"
