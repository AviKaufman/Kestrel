#!/usr/bin/env bash
set -eu

case "${1:-}" in
  resolve) printf 'tmux\t%%1\n' ;;
  send) printf 'sent\n' ;;
  *) exit 1 ;;
esac
