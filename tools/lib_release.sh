#!/usr/bin/env bash

release_version() {
  if [[ -n "${VERSION:-}" ]]; then
    printf '%s' "${VERSION}"
    return
  fi

  if git describe --tags --always --dirty >/dev/null 2>&1; then
    git describe --tags --always --dirty
    return
  fi

  printf '%s' "dev"
}

release_commit() {
  if [[ -n "${COMMIT:-}" ]]; then
    printf '%s' "${COMMIT}"
    return
  fi

  if git rev-parse --short HEAD >/dev/null 2>&1; then
    git rev-parse --short HEAD
    return
  fi

  printf '%s' "unknown"
}

release_date() {
  if [[ -n "${DATE:-}" ]]; then
    printf '%s' "${DATE}"
    return
  fi

  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

release_ldflags() {
  printf '%s' "-s -w -X translategemma-ui/internal/version.Version=$(release_version) -X translategemma-ui/internal/version.Commit=$(release_commit) -X translategemma-ui/internal/version.Date=$(release_date)"
}
