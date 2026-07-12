#!/bin/sh
set -eu

target=/app/secrets
mkdir -p "$target"

copy_secret() {
  source_path="$1"
  target_path="$target/$2"
  cp "$source_path" "$target_path"
  chown node:node "$target_path"
  chmod 400 "$target_path"
}

copy_secret "$AUTH_SECRET_FILE" auth-secret
copy_secret "$SYSARMOR_BOOTSTRAP_ADMIN_USERNAME_FILE" bootstrap-admin-username
copy_secret "$SYSARMOR_BOOTSTRAP_ADMIN_PASSWORD_FILE" bootstrap-admin-password
copy_secret "$SYSARMOR_BFF_JWT_PRIVATE_KEY_FILE" manager-jwt-private.pem

export AUTH_SECRET_FILE="$target/auth-secret"
export SYSARMOR_BOOTSTRAP_ADMIN_USERNAME_FILE="$target/bootstrap-admin-username"
export SYSARMOR_BOOTSTRAP_ADMIN_PASSWORD_FILE="$target/bootstrap-admin-password"
export SYSARMOR_BFF_JWT_PRIVATE_KEY_FILE="$target/manager-jwt-private.pem"

exec su node -s /bin/sh -c 'exec node server.js'
