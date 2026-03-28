#!/usr/bin/env bash
# Pipeline logging utilities
# All output goes to stderr — stdout is reserved for data.

readonly _CLR_RESET='\033[0m'
readonly _CLR_RED='\033[0;31m'
readonly _CLR_GREEN='\033[0;32m'
readonly _CLR_YELLOW='\033[0;33m'
readonly _CLR_BLUE='\033[0;34m'
readonly _CLR_GRAY='\033[0;90m'

_log() {
  local level="$1" color="$2"
  shift 2
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf "${_CLR_GRAY}%s${_CLR_RESET} ${color}[PIPELINE %-7s]${_CLR_RESET} %s\n" \
    "$ts" "$level" "$*" >&2
}

log_info()    { _log "INFO"    "$_CLR_BLUE"   "$@"; }
log_success() { _log "SUCCESS" "$_CLR_GREEN"  "$@"; }
log_warn()    { _log "WARN"    "$_CLR_YELLOW" "$@"; }
log_error()   { _log "ERROR"   "$_CLR_RED"    "$@"; }

log_section() {
  echo "" >&2
  printf "${_CLR_BLUE}══════════════════════════════════════════════════════════${_CLR_RESET}\n" >&2
  printf "${_CLR_BLUE}  %s${_CLR_RESET}\n" "$*" >&2
  printf "${_CLR_BLUE}══════════════════════════════════════════════════════════${_CLR_RESET}\n" >&2
}
