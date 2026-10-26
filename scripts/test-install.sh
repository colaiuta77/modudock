#!/usr/bin/env bash
# 임시 파일 경로와 명령 대역으로 네이티브 설치·업데이트의 안전 경계를 검증한다.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ -f scripts/install.sh ]] || { echo 'FAIL: scripts/install.sh is missing' >&2; exit 1; }
source scripts/install.sh
original_prompt=$(declare -f prompt_admin)
test_root=$(mktemp -d /tmp/modudock-install-test.XXXXXX)
trap 'case "$test_root" in /tmp/modudock-install-test.*) rm -rf -- "$test_root";; esac' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
unset MODUDOCK_VERSION MODUDOCK_HEALTH_URL || true

fixture="$test_root/fixture"
mkdir -p "$fixture/packaging"
printf '#!/bin/bash\nexit 0\n' > "$fixture/modudock-linux-amd64"
printf 'new license\n' > "$fixture/LICENSE"
printf 'new notices\n' > "$fixture/third-party-notices.txt"
cp packaging/modudock.service "$fixture/packaging/"
tar -czf "$test_root/modudock-linux-amd64.tar.gz" -C "$fixture" .
(cd "$test_root" && sha256sum modudock-linux-amd64.tar.gz > SHA256SUMS)

curl() {
  local output='' url='' arg
  while (($#)); do
    arg=$1; shift
    case "$arg" in -o|--output) output=$1; shift;; -w|--write-out|--proto|--proto-redir|--tlsv1.2|--retry|--connect-timeout|--max-time) if [[ "$arg" != --tlsv1.2 ]]; then shift; fi;; https://*) url=$arg;; esac
  done
  [[ ${download_failure:-0} == 0 ]] || return 22
  if [[ -z "$output" || "$output" == /dev/null ]]; then printf 'https://github.com/colaiuta77/modudock/releases/tag/v0.1.0'; return; fi
  case "$url" in
    */SHA256SUMS) cp "$test_root/SHA256SUMS" "$output";;
    */modudock-linux-amd64.tar.gz) cp "$test_root/modudock-linux-amd64.tar.gz" "$output"; if [[ ${bad_hash:-0} == 1 ]]; then printf corrupt >> "$output"; fi;;
    *) return 22;;
  esac
  if [[ ${swap_lock:-0} == 1 && "$url" == */SHA256SUMS ]]; then
    ln -sf "$test_root/root-victim" "$DATA_DIR/core.lock"
  fi
}
check_systemd() { :; }
id() { if [[ ${1:-} == -u && $# == 1 ]]; then echo 0; else command id "$@"; fi; }
getent() { if [[ $1 == passwd ]]; then echo 'modudock:x:999:999::/var/lib/modudock:/usr/sbin/nologin'; else command getent "$@"; fi; }
useradd() { echo useradd >> "$events"; }
chown() { :; }
cp() {
  if [[ ${backup_failure:-0} == 1 && "$*" == *"$DATA_DIR"* ]]; then return 28; fi
  command cp "$@"
}
install() {
  local args=()
  while (($#)); do
    case "$1" in -o|-g) shift 2;; *) args+=("$1"); shift;; esac
  done
  command install "${args[@]}"
}
runuser() {
  echo "runuser $*" >> "$events"
  if [[ ${4:-} == bash ]]; then shift 3; "$@"; return; fi
  [[ ${health_failure:-0} == 0 ]]
}
systemctl() {
  echo "$*" >> "$events"
  if [[ "$1" == show ]]; then
    case "$*" in
      *ExecStart*) printf '{ path=%s/modudock ; argv[]=%s/modudock serve --data-dir %s --listen 127.0.0.1:7438 ; ignore_errors=no ; }\n' "$INSTALL_DIR" "$INSTALL_DIR" "${effective_data:-$DATA_DIR}";;
      *User*) echo modudock;;
      *Group*) echo modudock;;
      *LoadState*) if [[ -f "$UNIT_FILE" ]]; then echo loaded; else echo not-found; fi;;
    esac
    return 0
  fi
  case "$1" in is-active) [[ ${active:-0} == 1 ]];; enable) [[ ${start_failure:-0} == 0 ]] || return 1; active=1;; *) return 0;; esac
}
sleep() { :; }
prompt_admin() { ADMIN_USERNAME=admin; ADMIN_PASSWORD='fixture-password-1234'; echo prompted >> "$events"; }

new_case() {
  case_root="$test_root/$1"
  mkdir -p "$case_root/bin" "$case_root/systemd" "$case_root/lock"
  INSTALL_DIR="$case_root/bin"
  DATA_DIR="$case_root/data"
  UNIT_FILE="$case_root/systemd/modudock.service"
  SHARE_DIR="$case_root/share"
  BACKUP_DIR="$case_root/backups"
  LOCK_FILE="$case_root/lock/install.lock"
  events="$case_root/events"
  : > "$events"
  active=0; bad_hash=0; start_failure=0; health_failure=0; download_failure=0; backup_failure=0; swap_lock=0; effective_data=''
}
run_install() { (main "$@") > "$case_root/output" 2>&1; }

[[ $(detect_arch x86_64) == amd64 ]] || fail amd64
[[ $(detect_arch aarch64) == arm64 ]] || fail arm64
if (detect_arch armv7l) >/dev/null 2>&1; then fail '32-bit architecture accepted'; fi
if (MODUDOCK_VERSION='../bad'; release_version) >/dev/null 2>&1; then fail 'invalid version accepted'; fi
[[ $(MODUDOCK_VERSION=v1.2.3; release_version) == v1.2.3 ]] || fail 'pinned version'
[[ $(release_version) == v0.1.0 ]] || fail 'latest release redirect'

new_case fresh
run_install || { cat "$case_root/output"; fail 'fresh install'; }
[[ -x "$INSTALL_DIR/modudock" && -f "$UNIT_FILE" && -d "$DATA_DIR" ]] || fail 'installation files missing'
grep -q '^prompted$' "$events" || fail 'fresh administrator was not requested'
grep -q '^enable --now modudock$' "$events" || fail 'service not enabled'
[[ -f "$SHARE_DIR/LICENSE" ]] || fail 'license missing'
if grep -q fixture-password "$case_root/output" "$events"; then fail 'administrator password was logged'; fi

if ! (exec 3<>/dev/tty) 2>/dev/null; then
  new_case no-terminal
  if (eval "$original_prompt"; main) > "$case_root/output" 2>&1; then fail 'first install without terminal accepted'; fi
  [[ ! -e "$INSTALL_DIR/modudock" && ! -e "$DATA_DIR" ]] || fail 'no-terminal failure changed installation'
fi

new_case upgrade
mkdir -p "$DATA_DIR/plugins"
printf database > "$DATA_DIR/core.db"
printf key > "$DATA_DIR/auth.key"
printf plugin > "$DATA_DIR/plugins/data"
printf '#!/bin/bash\necho old\n' > "$INSTALL_DIR/modudock"
chmod 755 "$INSTALL_DIR/modudock"
printf custom-unit > "$UNIT_FILE"
active=1
run_install || { cat "$case_root/output"; fail upgrade; }
[[ $(cat "$UNIT_FILE") == custom-unit ]] || fail 'existing service overwritten'
backup=$(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d | head -1)
[[ $(cat "$backup/data/core.db") == database && $(cat "$backup/data/auth.key") == key && $(cat "$backup/data/plugins/data") == plugin ]] || fail 'full data backup missing'
grep -q old "$backup/modudock" || fail 'old binary not backed up'
if grep -q '^prompted$' "$events"; then fail 'update requested a new administrator'; fi

for failure in bad_hash download_failure; do
  new_case "$failure"
  printf old > "$INSTALL_DIR/modudock"
  declare "$failure=1"
  if run_install; then fail "$failure accepted"; fi
  [[ $(cat "$INSTALL_DIR/modudock") == old ]] || fail 'failed download replaced binary'
  if grep -q '^stop ' "$events"; then fail 'failed download stopped service'; fi
done

new_case symlink
ln -s "$test_root/elsewhere" "$DATA_DIR"
if run_install; then fail 'data symlink accepted'; fi
[[ ! -e "$test_root/elsewhere" ]] || fail 'symlink destination modified'

new_case swapped-lock
mkdir -p "$DATA_DIR"; printf existing > "$DATA_DIR/core.db"
printf old > "$INSTALL_DIR/modudock"
printf protected > "$test_root/root-victim"
swap_lock=1
if run_install; then fail 'lock swapped during download was accepted'; fi
[[ $(cat "$INSTALL_DIR/modudock") == old && $(cat "$test_root/root-victim") == protected ]] || fail 'swapped lock changed protected files'

new_case custom-data
mkdir -p "$DATA_DIR"; printf existing > "$DATA_DIR/core.db"
printf old > "$INSTALL_DIR/modudock"; printf custom > "$UNIT_FILE"
effective_data=/srv/custom-modudock
if run_install; then fail 'custom data directory was accepted'; fi
[[ $(cat "$INSTALL_DIR/modudock") == old ]] || fail 'unsupported unit replaced binary'
if grep -q '^stop ' "$events"; then fail 'unsupported unit stopped service'; fi

new_case backup-failure
mkdir -p "$DATA_DIR"; printf existing > "$DATA_DIR/core.db"
printf old > "$INSTALL_DIR/modudock"
active=1; backup_failure=1
if run_install; then fail 'backup failure reported success'; fi
[[ $(cat "$INSTALL_DIR/modudock") == old ]] || fail 'backup failure replaced binary'

new_case data-in-use
mkdir -p "$DATA_DIR"; printf existing > "$DATA_DIR/core.db"
printf old > "$INSTALL_DIR/modudock"
(
  exec 7<>"$DATA_DIR/core.lock"
  flock -n 7
  if run_install; then fail 'locked data accepted'; fi
  [[ $(cat "$INSTALL_DIR/modudock") == old ]] || fail 'locked data replaced binary'
)

new_case failed-start
mkdir -p "$DATA_DIR"; printf existing > "$DATA_DIR/core.db"
start_failure=1
if run_install; then fail 'service start failure reported success'; fi
[[ -d "$BACKUP_DIR" ]] || fail 'failure recovery backup missing'

new_case failed-health
mkdir -p "$DATA_DIR"; printf existing > "$DATA_DIR/core.db"
active=1; health_failure=1
if run_install; then fail 'health failure reported success'; fi
[[ -f "$DATA_DIR/core.db" ]] || fail 'health failure deleted data'
echo 'PASS native installer: architectures, version pinning, fresh install, no-terminal failure, password privacy, preserved service, full backup, hash/download/backup failures, data lock, symlink rejection, startup/health failures.'
