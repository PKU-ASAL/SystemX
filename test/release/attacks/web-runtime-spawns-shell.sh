#!/usr/bin/env bash
set -euo pipefail

marker="${1:?marker required}"
/bin/sh -c "/bin/sh -c ': # node $marker'"
