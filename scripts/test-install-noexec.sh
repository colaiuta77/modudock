#!/usr/bin/env bash
# 격리된 noexec 마운트에서 설치 검증과 실행 실패 안내를 실제로 검증한다.
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ ${1:-} != --isolated ]]; then
  exec unshare --mount --propagation private "$BASH" "$0" --isolated
fi
test_root=$(mktemp -d /tmp/modudock-noexec-test.XXXXXX)
trap 'status=$?; umount "$test_root" || true; case "$test_root" in /tmp/modudock-noexec-test.*) rm -rf -- "$test_root";; esac; exit "$status"' EXIT
mount -t tmpfs -o noexec,nosuid,nodev tmpfs "$test_root"
MODUDOCK_TEST_NOEXEC_TMP="$test_root" bash scripts/test-install.sh

source scripts/install.sh
WORK_DIR="$test_root/probe"
mkdir -m 0700 "$WORK_DIR"
cp scripts/fixtures/modudock-release.sh "$WORK_DIR/candidate"
CANDIDATE="$WORK_DIR/candidate"
if (check_candidate_config) > "$test_root/output" 2>&1; then
  echo 'FAIL: noexec candidate unexpectedly ran' >&2; exit 1
fi
grep -q 'Permission denied' "$test_root/output"
if grep -q 'lacks YAML config support' "$test_root/output"; then
  cat "$test_root/output" >&2
  echo 'FAIL: execution denied was reported as missing config support' >&2; exit 1
fi
echo 'PASS noexec installer: real denied execution, executable staging and accurate failure diagnosis.'
