#!/bin/sh
set -eu

source_dir="/run/secrets"
target_dir="/run/guardian-secrets"

mkdir -p "$target_dir"
chmod 0711 "$target_dir"
for name in guardian_admin_token mihomo_controller_secret primary_subscription_url; do
  source_path="${source_dir}/${name}"
  target_path="${target_dir}/${name}"
  if [ ! -r "$source_path" ]; then
    echo "Required secret ${name} is not readable" >&2
    exit 1
  fi
  rm -f "$target_path"
  cp "$source_path" "$target_path"
  chown guardian:guardian "$target_path"
  chmod 0400 "$target_path"
done

exec su-exec guardian:guardian /usr/local/bin/mihomo-guardian "$@"
