# Contributing To SysArmor

SysArmor welcomes focused bug reports and pull requests. Before starting a larger change, open an issue
to confirm the problem, intended outcome, and ownership boundary. Report security vulnerabilities through
the private process in [SECURITY.md](SECURITY.md), not through a public issue.

## Development Workflow

Create feature and fix branches from `dev`, then open pull requests back to `dev`. Release branches are
short-lived and are merged into `main` only after release-candidate acceptance. Do not commit directly to
`dev` or `main`.

Keep changes narrow and follow the surrounding code style. Use Conventional Commits such as `feat:`,
`fix:`, `docs:`, and `test:`; keep each commit focused on one concern. The
[development guide](docs/development/development.md) describes repository boundaries, build commands, and
contracts that must stay synchronized.

## Verification

Add or update tests for behavior changes and run the smallest suite that fully covers the change. Shared
contracts and user-facing workflows require broader regression coverage. See the
[testing guide](docs/development/testing.md) for Product, Effectiveness, and Performance suites.

At minimum, run:

```bash
make test-unit
git diff --check
```

Document any relevant test that cannot be run and explain the remaining risk in the pull request.
