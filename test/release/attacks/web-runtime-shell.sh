#!/usr/bin/env bash
set -euo pipefail

container="${1:?container required}"
marker="${2:?marker required}"
docker exec "$container" curl -fsS "http://127.0.0.1:3000/rce?marker=$marker"
