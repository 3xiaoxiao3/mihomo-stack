# Security policy

## Supported versions

Security fixes are applied to the latest released minor version. Pre-release
builds and modified third-party images are not supported.

## Reporting a vulnerability

Use GitHub's private security advisory feature for the repository that hosts
this project. Do not open a public issue before a fix is available. Include:

- the affected version and platform;
- a minimal reproduction without real subscription URLs or tokens;
- the expected impact and required attacker access;
- whether upstream Mihomo or MetaCubeXD is also affected.

Never send production configuration files. Replace server names, credentials,
UUIDs, subscription parameters, and controller secrets with synthetic values.

## Security boundary

Guardian protects configuration activation and management access. It assumes
the host, container runtime, mounted secret files, and administrator browser are
trusted. It does not protect against a compromised host root account, malicious
Mihomo binary, malicious Docker daemon, or a subscription whose nodes are
operated by an adversary.

The default Compose file publishes the proxy listener to all interfaces and the
Controller only to loopback. Operators are responsible for host firewall rules
and should bind the proxy listener to `127.0.0.1` when LAN access is unnecessary.
