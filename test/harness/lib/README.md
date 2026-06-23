# Harness Library

`test/harness/lib` contains shared shell glue for suite scripts. Keep these helpers generic: start/wait/query/collect/cleanup mechanics belong here; product assertions belong in `test/suites/<suite>/`.

Current helpers:

- `sa_init_repo_paths`: initialize `ROOT`, `RESULTS`, and `BIN`.
- `sa_make_tmp`: create a namespaced temporary directory.
- `sa_pick_ports`: allocate HTTP/gRPC manager ports and `MGR_URL`.
- `sa_kill_pid_ref` and `sa_cleanup_tmp`: standard process and tmp cleanup.
- `sa_wait_contains` and `sa_wait_url_contains`: retry a command or URL until output contains a string.
- `sa_wait_glob` and `sa_wait_no_glob`: wait for spool/WAL artifacts to appear or drain.
- `sa_build_all` and `sa_build_go_bins`: build project binaries for e2e scripts.
- `sa_manager_ctl`: run `sysarmorctl --manager-url "$MGR_URL" --json manager ...`.
