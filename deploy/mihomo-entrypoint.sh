#!/bin/sh
set -eu

data_dir="${MIHOMO_DATA_DIR:-/data}"
config_path="${MIHOMO_CONFIG_PATH:-${data_dir}/config.yaml}"
secret_file="${MIHOMO_SECRET_FILE:-/run/secrets/mihomo_controller_secret}"

umask 077
mkdir -p "$data_dir"

if [ ! -r "$secret_file" ]; then
  echo "Mihomo controller secret file is not readable" >&2
  exit 1
fi

secret="$(tr -d '\r\n' < "$secret_file")"
if [ -z "$secret" ]; then
  echo "Mihomo controller secret is empty" >&2
  exit 1
fi

if [ ! -s "$config_path" ]; then
  escaped_secret="$(printf '%s' "$secret" | sed "s/'/''/g")"
  temporary="${config_path}.bootstrap.tmp"
  {
    printf '%s\n' 'mixed-port: 7890'
    printf '%s\n' 'allow-lan: true'
    printf '%s\n' 'mode: rule'
    printf '%s\n' 'log-level: info'
    printf '%s\n' 'external-controller: 0.0.0.0:9090'
    printf '%s\n' 'external-ui: /app/dashboard'
    printf "secret: '%s'\n" "$escaped_secret"
    printf '%s\n' 'rules:' '  - MATCH,DIRECT'
  } > "$temporary"
  mv "$temporary" "$config_path"
fi

chown -R guardian:guardian "$data_dir"
exec su-exec guardian:guardian /usr/local/bin/mihomo -d "$data_dir" -f "$config_path"
