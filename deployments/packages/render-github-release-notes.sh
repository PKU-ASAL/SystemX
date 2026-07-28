#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: render-github-release-notes.sh VERSION OWNER/REPOSITORY rc|ga" >&2
}

if [[ $# -ne 3 ]]; then
  usage
  exit 2
fi

version="$1"
repository="$2"
release_type="$3"
source_sha="${SOURCE_SHA:-${GITHUB_SHA:-unknown}}"

[[ "$repository" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]] || {
  echo "[release-notes][ERROR] invalid repository: $repository" >&2
  exit 2
}

case "$release_type" in
  rc)
    [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+-rc\.[1-9][0-9]*$ ]] || {
      echo "[release-notes][ERROR] invalid RC version: $version" >&2
      exit 2
    }
    release_label="release candidate"
    ;;
  ga)
    [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
      echo "[release-notes][ERROR] invalid GA version: $version" >&2
      exit 2
    }
    release_label="release"
    ;;
  *)
    echo "[release-notes][ERROR] release type must be rc or ga" >&2
    exit 2
    ;;
esac

cat <<EOF
SysArmor \`$version\` $release_label from commit \`$source_sha\`.

## Online installation

\`\`\`bash
curl -fsSL https://github.com/$repository/releases/download/$version/install.sh | sudo bash
\`\`\`

## Container image installation

\`\`\`dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends bash ca-certificates curl util-linux
RUN curl -fsSL https://github.com/$repository/releases/download/$version/install.sh \\
    | bash -s -- --profile linux-container
ENTRYPOINT ["/usr/local/bin/sysarmor-container-entrypoint"]
\`\`\`

Run the container with privileged mode, the host cgroup namespace, host BTF and bpffs mounts, and a restart policy:

\`\`\`bash
docker run -d --privileged --cgroupns=host --restart unless-stopped \\
  -v /sys/kernel/btf/vmlinux:/sys/kernel/btf/vmlinux:ro \\
  -v /sys/fs/bpf:/sys/fs/bpf <image>
\`\`\`

## Verify build provenance

\`\`\`bash
gh attestation verify sysarmor-agent-linux-amd64-$version.tar.gz --repo $repository
\`\`\`

## What's changed

[View commits for $version](https://github.com/$repository/commits/$version)
EOF
