# Packages

`packages/` owns product distribution package definitions. It does not contain
the main Go implementation.

Use it for:

- agent distribution manifests and package scripts
- sensor bundle manifests and package scripts
- package schemas shared by Linux and future Windows builds

Keep runtime deployment files in `deployments/`, default product configuration
in `configs/`, and implementation code in `internal/`.

Planned layout:

```text
packages/
  agent/
    linux/
    windows/
  sensors/
    linux/
      tetragon/
    windows/
  manifests/
```
