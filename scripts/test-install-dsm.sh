#!/usr/bin/env bash
# DSM 도구만 있는 환경의 설치 경로·권한 전환·서비스 파일 안전성을 검증한다.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/install.sh
test_root=$(mktemp -d /tmp/modudock-dsm-test.XXXXXX)
trap 'case "$test_root" in /tmp/modudock-dsm-test.*) rm -rf -- "$test_root";; esac' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
DSM_VERSION_FILE="$test_root/VERSION"
MOUNTS_FILE="$test_root/mounts"
GROUP_FILE="$test_root/group"
printf 'majorversion="7"\nos_name="DSM"\n' > "$DSM_VERSION_FILE"
printf '/dev/test /volume1 btrfs rw 0 0\n' > "$MOUNTS_FILE"
printf 'modudock:x:1234:\n' > "$GROUP_FILE"
[[ $(detect_platform) == dsm ]] || fail 'DSM was not detected'
configure_platform
[[ "$PLATFORM" == dsm && "$INSTALL_DIR" == /volume1/modudock/bin && "$DATA_DIR" == /volume1/modudock/data && "$BACKUP_DIR" == /volume1/modudock/backups ]] || fail 'DSM persistent paths incorrect'
if (MODUDOCK_DSM_VOLUME=/tmp/unmounted; configure_platform) >/dev/null 2>&1; then fail 'non-volume root accepted'; fi
if (MODUDOCK_DSM_VOLUME=/volume2; configure_platform) >/dev/null 2>&1; then fail 'unmounted volume accepted'; fi
printf 'server:/share /volume2 nfs rw 0 0\n' >> "$MOUNTS_FILE"
if (MODUDOCK_DSM_VOLUME=/volume2; configure_platform) >/dev/null 2>&1; then fail 'network volume accepted for SQLite'; fi

WORK_DIR="$test_root/work"
mkdir -p "$WORK_DIR"
cp packaging/modudock.service "$WORK_DIR/modudock.service"
prepare_service_unit
grep -q '^ExecStart=/volume1/modudock/bin/modudock serve --data-dir /volume1/modudock/data --listen 127.0.0.1:7438$' "$WORK_DIR/modudock.service" || fail 'DSM ExecStart incorrect'
grep -q '^ReadWritePaths=/volume1/modudock/data$' "$WORK_DIR/modudock.service" || fail 'DSM writable path missing'
grep -q '^RequiresMountsFor=/volume1$' "$WORK_DIR/modudock.service" || fail 'volume dependency missing'
if grep -q '^StateDirectory' "$WORK_DIR/modudock.service"; then fail 'Linux state directory leaked into DSM unit'; fi

events="$test_root/events"
: > "$events"
su() {
  printf '%s\n' "$*" >> "$events"
  [[ "$1" == -s && "$3" == -g && "$4" == modudock && "$5" == -c && "$7" == modudock ]] || fail 'su invocation did not select shell/group/user'
  bash -c "$6"
}
marker="$test_root/must-not-exist"
input="\$(touch '$marker') ; * space ' quote"
[[ $(as_service printf '%s' "$input") == "$input" ]] || fail 'service arguments changed'
[[ ! -e "$marker" ]] || fail 'service argument injection'
if grep -q runuser "$events"; then fail 'DSM called runuser'; fi

id() {
  case "$*" in '-u modudock') echo "${mock_uid:-999}";; '-Gn modudock') echo "${mock_groups:-users modudock}";; *) command id "$@";; esac
}
mock_uid=0
if (validate_service_account) >/dev/null 2>&1; then fail 'root service identity accepted'; fi
mock_uid=999; mock_groups='users administrators'
if (validate_service_account) >/dev/null 2>&1; then fail 'DSM administrator account accepted'; fi
mock_groups='users modudock'
validate_service_account
printf '' > "$GROUP_FILE"
synogroup() {
  [[ "$*" == '--add modudock modudock' ]] || fail 'group creation changed unrelated members'
  printf 'modudock:x:1234:modudock\n' > "$GROUP_FILE"
}
ensure_dsm_group
printf 'modudock:x:0:\n' > "$GROUP_FILE"
if (ensure_dsm_group) >/dev/null 2>&1; then fail 'root group accepted'; fi
echo 'PASS DSM installer: detection, mounted volume, native paths, systemd rendering, su group/argument isolation, root/admin rejection.'
