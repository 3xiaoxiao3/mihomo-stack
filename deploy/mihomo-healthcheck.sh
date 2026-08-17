#!/bin/sh
set -eu

secret_file="${MIHOMO_SECRET_FILE:-/run/secrets/mihomo_controller_secret}"
[ -r "$secret_file" ] || exit 1
secret="$(tr -d '\r\n' < "$secret_file")"
wget -q -O /dev/null --header="Authorization: Bearer ${secret}" http://127.0.0.1:9090/version
