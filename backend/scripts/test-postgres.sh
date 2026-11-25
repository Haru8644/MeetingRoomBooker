#!/usr/bin/env bash
set -euo pipefail

export REQUIRE_POSTGRES_TESTS=1
go test -count=1 -v -run 'Postgres$' ./... | awk '
  { print }
  /^--- PASS: / { passed++ }
  /^[[:space:]]*--- SKIP: / { skipped++ }
  END {
    printf "PostgreSQL suites passed: %d; skipped tests (including subtests): %d\n", passed, skipped
    if (passed == 0 || skipped > 0) exit 1
  }
'
