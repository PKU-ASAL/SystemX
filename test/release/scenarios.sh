#!/usr/bin/env bash

release_scenarios() {
  printf '%s\n' web-runtime-shell download-by-lolbin payload-lifecycle
}

scenario_rule() {
  case "$1" in
    web-runtime-shell) printf '%s\n' web_runtime_spawns_shell ;;
    download-by-lolbin) printf '%s\n' download_by_lolbin ;;
    payload-lifecycle) printf '%s\n' payload_lifecycle ;;
    *) return 1 ;;
  esac
}

scenario_severity() {
  case "$1" in
    web-runtime-shell|payload-lifecycle) printf '%s\n' high ;;
    download-by-lolbin) printf '%s\n' medium ;;
    *) return 1 ;;
  esac
}

scenario_behaviors() {
  case "$1" in
    web-runtime-shell) printf '%s\n' process.exec ;;
    download-by-lolbin) printf '%s\n' network.connect ;;
    payload-lifecycle) printf '%s\n' 'file.write process.exec network.connect' ;;
    *) return 1 ;;
  esac
}

scenario_ports() {
  case "$1" in
    web-runtime-shell) return 0 ;;
    download-by-lolbin) printf '%s\n' 8080 ;;
    payload-lifecycle) printf '%s\n' '8080 8443' ;;
    *) return 1 ;;
  esac
}

scenario_attack() {
  case "$1" in
    web-runtime-shell) printf '%s\n' web-runtime-shell.sh ;;
    download-by-lolbin) printf '%s\n' download-by-lolbin.sh ;;
    payload-lifecycle) printf '%s\n' payload-lifecycle.sh ;;
    *) return 1 ;;
  esac
}
