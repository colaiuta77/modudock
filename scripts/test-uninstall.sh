#!/usr/bin/env bash
# 임시 설치 환경에서 서비스 제거와 설정·데이터 보존 및 실패 안전성을 검증한다.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/install.sh
test_root=$(mktemp -d /tmp/modudock-uninstall-test.XXXXXX)
trap 'case "$test_root" in /tmp/modudock-uninstall-test.*) rm -rf -- "$test_root";; esac' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
id() { if [[ "$*" == -u ]]; then echo 0; else command id "$@"; fi; }
curl() { fail 'uninstall attempted a download'; }
prompt_admin() { fail 'uninstall requested administrator input'; }
service_account_exists() { fail 'uninstall inspected service account'; }
configure_platform() {
  PLATFORM=${fixture_platform:-linux}
  if [[ "$PLATFORM" == dsm ]]; then
    [[ ${MODUDOCK_DSM_ROOT:-} == /volume2/apps/modudock ]] || fail 'custom DSM root not discovered'
  fi
}
install() {
  local args=()
  while (($#)); do case "$1" in -o|-g) shift 2;; *) args+=("$1"); shift;; esac; done
  command install "${args[@]}"
}
cp() { [[ ${copy_failure:-0} == 0 ]] || return 1; command cp "$@"; }
systemctl() {
  printf '%s\n' "$*" >> "$events"
  if [[ "$1" == show ]]; then
    local property=${2#--property=}
    [[ ${property_failure:-} != "$property" ]] || return 1
    case "$property" in
      Version) echo Version=219;;
      LoadState) if [[ -f "$UNIT_FILE" ]]; then echo LoadState=loaded; else echo LoadState=not-found; fi;;
      FragmentPath) printf 'FragmentPath=%s\n' "$UNIT_FILE";;
      ExecStart) printf 'ExecStart={ path=%s ; argv[]=%s serve --data-dir /retained/data --listen 192.168.1.100:7438 ; ignore_errors=no ; }\n' "${effective_binary:-$INSTALL_DIR/modudock}" "${effective_binary:-$INSTALL_DIR/modudock}";;
      User|Group) printf '%s=modudock\n' "$property";;
      RootDirectory|RootImage|BindPaths|BindReadOnlyPaths|TemporaryFileSystem) printf '%s=%s\n' "$property" "$([[ ${mapping_property:-} != "$property" ]] || printf /isolated/path)";;
    esac
    return 0
  fi
  [[ ${command_failure:-} != "$1" ]]
}
detect_platform() { printf '%s' "${fixture_platform:-linux}"; }
new_case() {
  case_root="$test_root/$1"
  INSTALL_DIR="$case_root/bin"; UNIT_FILE="$case_root/systemd/modudock.service"
  DATA_DIR="$case_root/data"; CONFIG_FILE="$case_root/config/config.yaml"
  BACKUP_DIR="$case_root/backups"; LOCK_FILE="$case_root/lock/install.lock"
  LOGROTATE_FILE="$case_root/logrotate/modudock"; SHARE_DIR="$case_root/share"; LOG_DIR="$case_root/logs"
  mkdir -p "$INSTALL_DIR" "${UNIT_FILE}.d" "$DATA_DIR/plugins" "${CONFIG_FILE%/*}" "${LOGROTATE_FILE%/*}" "$SHARE_DIR" "$LOG_DIR"
  printf binary > "$INSTALL_DIR/modudock"; printf legacy-unit-settings > "$UNIT_FILE"
  printf override-settings > "${UNIT_FILE}.d/proxy.conf"
  printf database > "$DATA_DIR/core.db"; printf secret > "$DATA_DIR/auth.key"
  printf plugin > "$DATA_DIR/plugins/data"; printf yaml-settings > "$CONFIG_FILE"
  printf rotation > "$LOGROTATE_FILE"; printf notice > "$SHARE_DIR/LICENSE"; printf log > "$LOG_DIR/service.log"
  events="$case_root/events"; : > "$events"
  fixture_platform=linux; command_failure=''; property_failure=''; copy_failure=0; effective_binary=''; mapping_property=''
  unset MODUDOCK_DSM_ROOT MODUDOCK_DSM_VOLUME
}
retained() {
  [[ $(cat "$CONFIG_FILE") == yaml-settings && $(cat "$DATA_DIR/core.db") == database && $(cat "$DATA_DIR/auth.key") == secret && $(cat "$DATA_DIR/plugins/data") == plugin ]] || fail 'config or data changed'
  [[ $(cat "${UNIT_FILE}.d/proxy.conf") == override-settings && $(cat "$LOGROTATE_FILE") == rotation && $(cat "$SHARE_DIR/LICENSE") == notice && $(cat "$LOG_DIR/service.log") == log ]] || fail 'retained settings/logs/share changed'
}
run_uninstall() { (main uninstall) > "$case_root/output" 2>&1; }

new_case linux
run_uninstall || { cat "$case_root/output"; fail 'Linux uninstall'; }
[[ ! -e "$INSTALL_DIR/modudock" && ! -e "$UNIT_FILE" ]] || fail 'binary/service still installed'
retained
backup=$(find "$BACKUP_DIR" -name modudock.service -type f)
[[ $(cat "$backup") == legacy-unit-settings && $(cat "${backup%/*}/service-overrides/proxy.conf") == override-settings ]] || fail 'legacy service settings not backed up'
[[ $(stat -c %a "${backup%/*}") == 700 ]] || fail 'service backup not private'
grep -q '^stop modudock$' "$events" && grep -q '^disable modudock$' "$events" && grep -q '^daemon-reload$' "$events" || fail 'service registration not removed'
run_uninstall || { cat "$case_root/output"; fail 'repeated uninstall'; }
retained

for failure in stop disable; do
  new_case "$failure-failure"; command_failure=$failure
  if run_uninstall; then fail "$failure failure ignored"; fi
  [[ -f "$UNIT_FILE" && -f "$INSTALL_DIR/modudock" ]] || fail "$failure failure deleted installation"
  retained
done
new_case backup-failure; copy_failure=1
if run_uninstall; then fail 'backup failure ignored'; fi
[[ -f "$UNIT_FILE" && -f "$INSTALL_DIR/modudock" ]] || fail 'backup failure deleted installation'
if grep -q '^stop ' "$events"; then fail 'backup failure stopped service'; fi

new_case foreign-service; effective_binary=/usr/bin/unrelated
if run_uninstall; then fail 'foreign executable accepted'; fi
[[ -f "$UNIT_FILE" && -f "$INSTALL_DIR/modudock" ]] || fail 'foreign service changed installation'
if grep -q '^stop ' "$events"; then fail 'foreign service stopped'; fi

new_case symlink
mv "$INSTALL_DIR/modudock" "$case_root/victim"; ln -s "$case_root/victim" "$INSTALL_DIR/modudock"
if run_uninstall; then fail 'binary symlink accepted'; fi
[[ $(cat "$case_root/victim") == binary && -f "$UNIT_FILE" ]] || fail 'symlink victim changed'

new_case locked
mkdir -p "${LOCK_FILE%/*}"; exec 8<>"$LOCK_FILE"; flock -n 8
if run_uninstall; then fail 'concurrent installer lock ignored'; fi
exec 8>&-
[[ -f "$UNIT_FILE" && -f "$INSTALL_DIR/modudock" ]] || fail 'locked uninstall deleted files'

for property in Version LoadState ExecStart FragmentPath User Group RootDirectory RootImage BindPaths BindReadOnlyPaths TemporaryFileSystem; do
  new_case "property-$property"; property_failure=$property
  if run_uninstall; then fail "failed $property inspection ignored"; fi
  [[ -f "$UNIT_FILE" && -f "$INSTALL_DIR/modudock" ]] || fail 'inspection failure deleted files'
done

for property in RootDirectory RootImage BindPaths BindReadOnlyPaths TemporaryFileSystem; do
  new_case "mapping-$property"; mapping_property=$property
  if run_uninstall; then fail "filesystem mapping $property accepted"; fi
  [[ -f "$UNIT_FILE" && -f "$INSTALL_DIR/modudock" ]] || fail 'filesystem mapping deleted host files'
  if grep -q '^stop ' "$events"; then fail 'mapped service stopped'; fi
  retained
done

new_case dsm-custom; fixture_platform=dsm; effective_binary=/volume2/apps/modudock/bin/modudock
run_uninstall || { cat "$case_root/output"; fail 'custom DSM uninstall'; }
[[ ! -e "$INSTALL_DIR/modudock" && ! -e "$UNIT_FILE" ]] || fail 'DSM targets not removed'
retained

new_case dsm-root-mismatch; fixture_platform=dsm; effective_binary=/volume2/apps/modudock/bin/modudock
MODUDOCK_DSM_ROOT=/volume1/modudock
if run_uninstall; then fail 'mismatched DSM root accepted'; fi
[[ -f "$UNIT_FILE" && -f "$INSTALL_DIR/modudock" ]] || fail 'root mismatch deleted files'
if grep -q '^stop ' "$events"; then fail 'root mismatch stopped service'; fi

new_case bad-arguments
if (main uninstall extra) > "$case_root/output" 2>&1; then fail 'extra arguments accepted'; fi
[[ -f "$UNIT_FILE" && -f "$INSTALL_DIR/modudock" ]] || fail 'bad arguments deleted files'
echo 'PASS native uninstall: Linux/DSM, preserved config/data/settings, private unit backup, repeat removal, failure boundaries and shared lock.'
