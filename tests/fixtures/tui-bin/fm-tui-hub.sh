#!/usr/bin/env bash
set -eu

case "${1:-}" in
  resolve) printf 'tmux\t%%1\n' ;;
  history) printf 'captain: fake hub history\nfirstmate: fake hub answer\n' ;;
  send) printf 'sent\n' ;;
  *) exit 1 ;;
esac
