# Web

`web/` is reserved for operator-facing UI projects, such as a future manager UI.

Keep Go services under `cmd/` and `internal/`. UI projects should live here with
their own package manager files and build pipeline.

## Manager Console

`web/manager` is the SysArmor Manager Console UI. It is a Next.js React 19
application using mock data to validate the manager workflows before the screens
are connected to the manager HTTP API.

Recommended commands from the repository root:

```bash
make web-install
make web-up
make web-status
make web-stop
```

`make web-up` builds the console and starts a Next.js production server in the
background at `http://127.0.0.1:4173`, with logs in
`.run/manager-console.log`.

For foreground development with live logs:

```bash
make web-dev
```

`make web-dev` starts the Next.js hot-reload development server at
`http://127.0.0.1:5173`.

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
