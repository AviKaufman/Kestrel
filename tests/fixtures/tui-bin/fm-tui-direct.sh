#!/usr/bin/env bash
set -eu
case "${1:-}" in
  list) ;;
  peek) printf 'bounded direct capture\n' ;;
  *) exit 1 ;;
esac
