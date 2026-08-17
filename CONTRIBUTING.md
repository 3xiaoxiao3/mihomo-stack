# Contributing

Thank you for improving Mihomo Stack.

1. Discuss behavior or public configuration changes before implementing them.
2. Update the relevant file in `docs/specs` in the same pull request.
3. Keep subscription URLs, tokens, and real configurations out of tests,
   commits, logs, screenshots, and issues.
4. Add tests for failure and rollback paths, not only the successful path.
5. Run the verification commands from `AGENTS.md`.

Pull requests should explain the observable behavior, assumptions, verification
performed, and remaining risk. Changes that implement proxy protocols,
multi-user RBAC, a database, or a public conversion service are outside the v1
scope unless the system specification is explicitly revised first.
