# Web

`web/` contains operator-facing UI projects.

Keep Go services under `cmd/` and `internal/`. UI projects should live here with
their own package manager files and build pipeline.

## Manager Console

`web/manager` is the SysArmor Manager Console UI. It is a Next.js React 19
application. API mode uses an authenticated same-origin BFF; the browser does
not connect to Manager directly.

Recommended commands from the repository root:

```bash
make web-install
make auth-init
make deploy
make doctor
```

The compose deployment serves the console at `http://127.0.0.1:4173`. Sign in
with the bootstrap credentials stored under
`deployments/pki/agent-plane-mtls/runtime/`.

For foreground development with live logs:

```bash
make web-dev
```

`make web-dev` requires the same Secret file environment variables as the
compose service and starts the hot-reload server at `http://127.0.0.1:5173`.

To check the production build locally:

```bash
make web-preview
```

This builds the console and serves it in the foreground at
`http://127.0.0.1:4173`.

Direct project commands:

```bash
cd web/manager
pnpm install
pnpm dev
```
