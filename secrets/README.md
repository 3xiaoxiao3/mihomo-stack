# Runtime secrets

Create these three files before starting Compose:

- `guardian_admin_token.txt`: a random administrator token of at least 16
  characters. `openssl rand -hex 32` is recommended.
- `mihomo_controller_secret.txt`: a separate random controller secret.
- `primary_subscription_url.txt`: the complete subscription URL, with one
  optional trailing newline.

Keep the files out of Git. On Unix, use mode `0600` when the container runtime
can read owner-only bind mounts. Never paste their values into issues or logs.
