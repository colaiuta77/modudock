#!/usr/bin/env bash
# 임시 파일 경로와 명령 대역으로 네이티브 설치·업데이트의 안전 경계를 검증한다.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ -f scripts/install.sh ]] || { echo 'FAIL: scripts/install.sh is missing' >&2; exit 1; }
source scripts/install.sh
original_prompt=$(declare -f prompt_admin)
mktemp() {
  if [[ -n ${MODUDOCK_TEST_NOEXEC_TMP:-} && ${@: -1} == /tmp/modudock-install.XXXXXX ]]; then
    command mktemp -d "$MODUDOCK_TEST_NOEXEC_TMP/modudock-install.XXXXXX"
  else command mktemp "$@"; fi
}
test_root=$(mktemp -d /tmp/modudock-install-test.XXXXXX)
trap 'case "$test_root" in /tmp/modudock-install-test.*) rm -rf -- "$test_root";; esac' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
unset MODUDOCK_VERSION MODUDOCK_HEALTH_URL || true
MODUDOCK_LISTEN=127.0.0.1:7438
MODUDOCK_PUBLIC_URL=''
MODUDOCK_DSM_ROOT=/volume1/modudock

fixture="$test_root/fixture"
mkdir -p "$fixture/packaging"
cp scripts/fixtures/modudock-release.sh "$fixture/modudock-linux-amd64"
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
id() { if [[ "$*" == '-u modudock' ]]; then echo 999; elif [[ ${1:-} == -u && $# == 1 ]]; then echo 0; else command id "$@"; fi; }
getent() { if [[ $1 == passwd ]]; then echo 'modudock:x:999:999::/var/lib/modudock:/usr/sbin/nologin'; else command getent "$@"; fi; }
useradd() { echo useradd >> "$events"; }
chown() { :; }
stat() {
  if [[ "$*" == "-c %u $INSTALL_DIR" ]]; then echo "${staging_owner:-0}"; else command stat "$@"; fi
}
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
  if [[ ${4:-} == id && ${5:-} == -u ]]; then echo 999; return; fi
  if [[ ${4:-} == bash ]]; then shift 3; "$@"; return; fi
  [[ ${health_failure:-0} == 0 ]]
}
systemctl() {
  echo "$*" >> "$events"
  if [[ "$*" == 'is-active --quiet logrotate.timer' ]]; then [[ ${rotation_enabled:-1} == 1 ]]; return; fi
  local arg property
  for arg in "$@"; do
    case "$arg" in
      --value|--now) printf "systemctl: unrecognized option '%s'\n" "$arg" >&2; return 1;;
      --property=*) property=${arg#*=};;
    esac
  done
  if [[ "$1" == show ]]; then
    [[ ${failed_property:-} != "$property" ]] || return 1
    case "$property" in
      Version) echo 'Version=219';;
      ExecStart)
        if [[ -n ${effective_config:-} ]]; then
          printf 'ExecStart={ path=%s/modudock ; argv[]=%s/modudock serve --config=%s %s ; ignore_errors=no ; }\n' "$INSTALL_DIR" "$INSTALL_DIR" "$effective_config" "${effective_flags:-}"
        else printf 'ExecStart={ path=%s/modudock ; argv[]=%s/modudock serve --data-dir=%s --listen=%s --public-url=https://panel.example.test ; ignore_errors=no ; }\n' "$INSTALL_DIR" "$INSTALL_DIR" "${effective_data:-$DATA_DIR}" "${effective_listen:-127.0.0.1:7438}"; fi;;
      User|Group) printf '%s=modudock\n' "$property";;
      LoadState) if [[ -f "$UNIT_FILE" ]]; then echo 'LoadState=loaded'; else echo 'LoadState=not-found'; fi;;
      RootDirectory) printf 'RootDirectory=%s\n' "${effective_root:-}";;
    esac
    return 0
  fi
  case "$1" in is-active) [[ ${active:-0} == 1 ]];; start) [[ ${start_failure:-0} == 0 ]] || return 1; active=1;; *) return 0;; esac
}
sleep() { :; }
prompt_admin() { ADMIN_USERNAME=admin; ADMIN_PASSWORD='fixture-password-1234'; echo prompted >> "$events"; }
logrotate() { :; }
command() {
  if [[ "$*" == '-v logrotate' && ${logrotate_installed:-1} == 0 ]]; then return 1; fi
  builtin command "$@"
}

new_case() {
  case_root="$test_root/$1"
  mkdir -p "$case_root/bin" "$case_root/systemd" "$case_root/lock"
  INSTALL_DIR="$case_root/bin"
  DATA_DIR="$case_root/data"
  UNIT_FILE="$case_root/systemd/modudock.service"
  SHARE_DIR="$case_root/share"
  BACKUP_DIR="$case_root/backups"
  CONFIG_FILE="$case_root/config/config.yaml"
  LOG_DIR="$case_root/logs"
  LOGROTATE_FILE="$case_root/logrotate/modudock"
  LOCK_FILE="$case_root/lock/install.lock"
  events="$case_root/events"
  : > "$events"
  active=0; bad_hash=0; start_failure=0; health_failure=0; download_failure=0; backup_failure=0; swap_lock=0; effective_data=''; effective_root=''; failed_property=''; effective_config=''; effective_flags=''; effective_listen=''; rotation_enabled=1; logrotate_installed=1; staging_owner=0
}
run_install() { (main "$@") > "$case_root/output" 2>&1; }

new_case rotation-boundaries
rotation_enabled=0
if rotation_available; then fail 'inactive timer accepted as working rotation'; fi
rotation_enabled=1
rotation_available || fail 'active timer with logrotate not recognized'
logrotate_installed=0
if rotation_available; then fail 'missing logrotate executable accepted'; fi

new_case network-validation
for value in 'https://example.test/path' 'https://user@example.test' 'https://example.test?' 'https://example.test#' 'https://example.test/' 'https://example.test:99999' 'https://example.test/%n' 'http://bad host'; do
 if (PUBLIC_URL="$value"; validate_network) >/dev/null 2>&1; then fail "invalid public URL accepted: $value"; fi
done
for value in '127.0.0.1:0' '127.0.0.1:65536' 'bad address' '127.0.0.1:7438%n'; do
 if (LISTEN="$value"; PUBLIC_URL=''; validate_network) >/dev/null 2>&1; then fail "invalid listen accepted: $value"; fi
done
LISTEN=192.168.1.50:7440; PUBLIC_URL=https://panel.example.test
validate_network
[[ $(listener_health_url) == http://192.168.1.50:7440 ]] || fail 'specific-IP health target incorrect'
LISTEN=0.0.0.0:7441
[[ $(listener_health_url) == http://127.0.0.1:7441 ]] || fail 'wildcard health target incorrect'
ip() { printf '2: docker0    inet 172.17.0.1/16 scope global docker0\n3: vethdeadbeef    inet 172.18.0.1/16 scope global vethdeadbeef\n4: br-abcdef    inet 172.19.0.1/16 scope global br-abcdef\n5: br0    inet 192.168.1.50/24 scope global br0\n'; }
[[ $(detect_ipv4) == 192.168.1.50 ]] || fail 'Docker candidate accepted or DSM bridge rejected'
unset -f ip
LISTEN=127.0.0.1:7438; PUBLIC_URL=''

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
grep -q '^enable modudock$' "$events" || fail 'service not enabled'
grep -q '^start modudock$' "$events" || fail 'service not started'
[[ -f "$SHARE_DIR/LICENSE" ]] || fail 'license missing'
[[ -f "$CONFIG_FILE" ]] || fail 'YAML config missing'
[[ -z $(find "$INSTALL_DIR" -maxdepth 1 -name '.modudock-install.*' -print) ]] || fail 'successful install leaked staging directory'
[[ $(stat -c %a "$CONFIG_FILE") == 640 && $(stat -c %a "${CONFIG_FILE%/*}") == 750 ]] || fail 'config permissions are not read-only for service'
grep -q "^$LOG_DIR/service.log {" "$LOGROTATE_FILE" || fail 'operational rotation rule missing'
if grep -q core.jsonl "$LOGROTATE_FILE"; then fail 'UI log is double-rotated'; fi
grep -q "^ExecStart=$INSTALL_DIR/modudock serve --config $CONFIG_FILE$" "$UNIT_FILE" || fail 'fresh service does not use config'
grep -q '^listen: "127.0.0.1:7438"$' "$CONFIG_FILE" || fail 'fresh default not loopback'
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

new_case configured-upgrade
mkdir -p "$DATA_DIR" "${CONFIG_FILE%/*}"
printf database > "$DATA_DIR/core.db"
printf 'data_dir: "%s"\nlisten: "192.168.1.50:7442"\npublic_url: "https://panel.example.test"\n' "$DATA_DIR" > "$CONFIG_FILE"
cp "$CONFIG_FILE" "$case_root/original-config"
printf custom-config-unit > "$UNIT_FILE"
effective_config=$CONFIG_FILE
effective_flags='--listen 192.168.1.51:7443'
active=1
run_install || { cat "$case_root/output"; fail 'config upgrade'; }
cmp "$CONFIG_FILE" "$case_root/original-config" || fail 'existing config overwritten'
[[ $(cat "$UNIT_FILE") == custom-config-unit ]] || fail 'config unit overwritten'
backup=$(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d | head -1)
cmp "$backup/config.yaml" "$CONFIG_FILE" || fail 'config backup missing'
grep -q 'healthcheck --url http://192.168.1.51:7443' "$events" || fail 'config/CLI listener health target ignored'
if grep -q '^prompted$' "$events"; then fail 'config update prompted for administrator'; fi

new_case configured-missing-data
mkdir -p "${CONFIG_FILE%/*}"
printf 'data_dir: "%s"\n' "$DATA_DIR" > "$CONFIG_FILE"
printf existing > "$UNIT_FILE"; printf old > "$INSTALL_DIR/modudock"
effective_config=$CONFIG_FILE
if run_install; then fail 'missing configured DB accepted'; fi
[[ ! -d "$DATA_DIR" && $(cat "$INSTALL_DIR/modudock") == old ]] || fail 'missing DB update changed installation'
if grep -q '^stop ' "$events"; then fail 'missing DB update stopped service'; fi

new_case explicit-network
MODUDOCK_LISTEN=192.168.1.50:7450 MODUDOCK_PUBLIC_URL=https://panel.example.test run_install || { cat "$case_root/output"; fail 'explicit network'; }
grep -q '^listen: "192.168.1.50:7450"$' "$CONFIG_FILE" || fail 'explicit listener missing'
grep -q 'healthcheck --url http://192.168.1.50:7450' "$events" || fail 'healthcheck used public URL or loopback'
grep -q 'Access: https://panel.example.test' "$case_root/output" || fail 'HTTPS access hint missing'

new_case explicit-health
MODUDOCK_LISTEN=0.0.0.0:7450 MODUDOCK_HEALTH_URL=http://127.0.0.1:9999 run_install || { cat "$case_root/output"; fail 'explicit health'; }
grep -q 'healthcheck --url http://127.0.0.1:9999' "$events" || fail 'explicit health URL ignored'

new_case no-rotation
rotation_enabled=0
run_install || { cat "$case_root/output"; fail 'no-rotation install'; }
grep -q '^log_file: ""$' "$CONFIG_FILE" || fail 'unrotated file logging enabled'
[[ -f "$LOGROTATE_FILE" && ! -f "$LOG_DIR/service.log" ]] || fail 'inactive rotation rule missing or unrotated log created'

new_case old-config-binary
printf '#!/bin/bash\nexit 0\n' > "$fixture/modudock-linux-amd64"
tar -czf "$test_root/modudock-linux-amd64.tar.gz" -C "$fixture" .
(cd "$test_root" && sha256sum modudock-linux-amd64.tar.gz > SHA256SUMS)
if run_install; then fail 'old binary accepted for YAML installation'; fi
[[ ! -e "$DATA_DIR" && ! -e "$CONFIG_FILE" && ! -e "$INSTALL_DIR/modudock" ]] || fail 'old-binary rejection changed installation'
if grep -q '^stop ' "$events"; then fail 'old-binary rejection stopped service'; fi
cp scripts/fixtures/modudock-release.sh "$fixture/modudock-linux-amd64"
tar -czf "$test_root/modudock-linux-amd64.tar.gz" -C "$fixture" .
(cd "$test_root" && sha256sum modudock-linux-amd64.tar.gz > SHA256SUMS)

new_case unavailable-systemd
failed_property=Version
if run_install; then fail 'unavailable systemd accepted'; fi
[[ ! -e "$INSTALL_DIR/modudock" && ! -e "$DATA_DIR" ]] || fail 'unavailable systemd changed installation'

for property in ExecStart User Group RootDirectory RootImage BindPaths BindReadOnlyPaths TemporaryFileSystem LoadState; do
  new_case "failed-property-$property"
  mkdir -p "$DATA_DIR"; printf existing > "$DATA_DIR/core.db"
  printf old > "$INSTALL_DIR/modudock"
  if [[ "$property" != LoadState ]]; then printf custom-unit > "$UNIT_FILE"; fi
  active=1; failed_property=$property
  if run_install; then fail "failed $property query accepted"; fi
  [[ $(cat "$INSTALL_DIR/modudock") == old ]] || fail 'failed systemd query replaced binary'
  if grep -q '^stop ' "$events"; then fail 'failed systemd query stopped service'; fi
done

new_case mapped-root
mkdir -p "$DATA_DIR"; printf existing > "$DATA_DIR/core.db"
printf old > "$INSTALL_DIR/modudock"; printf custom-unit > "$UNIT_FILE"
effective_root=/srv/other-root
if run_install; then fail 'filesystem mapping accepted'; fi
[[ $(cat "$INSTALL_DIR/modudock") == old ]] || fail 'filesystem mapping replaced binary'
if grep -q '^stop ' "$events"; then fail 'filesystem mapping stopped service'; fi

for failure in bad_hash download_failure; do
  new_case "$failure"
  printf old > "$INSTALL_DIR/modudock"
  declare "$failure=1"
  if run_install; then fail "$failure accepted"; fi
  [[ $(cat "$INSTALL_DIR/modudock") == old ]] || fail 'failed download replaced binary'
  [[ -z $(find "$INSTALL_DIR" -maxdepth 1 -name '.modudock-install.*' -print) ]] || fail 'failed install leaked staging directory'
  if grep -q '^stop ' "$events"; then fail 'failed download stopped service'; fi
done

for unsafe in owner mode; do
  new_case "unsafe-staging-$unsafe"
  printf old > "$INSTALL_DIR/modudock"
  if [[ "$unsafe" == owner ]]; then staging_owner=1001; else chmod 777 "$INSTALL_DIR"; fi
  if run_install; then fail "unsafe staging $unsafe accepted"; fi
  [[ $(cat "$INSTALL_DIR/modudock") == old ]] || fail 'unsafe staging changed binary'
  if grep -q '^stop ' "$events"; then fail 'unsafe staging stopped service'; fi
done

new_case symlink
ln -s "$test_root/elsewhere" "$DATA_DIR"
if run_install; then fail 'data symlink accepted'; fi
[[ ! -e "$test_root/elsewhere" ]] || fail 'symlink destination modified'

new_case lock-parent-link
rmdir "$case_root/lock"
mkdir "$case_root/elsewhere"
ln -s "$case_root/elsewhere" "$case_root/lock"
if run_install; then fail 'symbolic-link lock directory accepted'; fi
[[ ! -e "$case_root/elsewhere/install.lock" ]] || fail 'lock directory escape created a file'

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

# DSM에서는 일반 배포판의 계정·권한 도구를 호출하지 않는지 전체 흐름으로 확인한다.
eval "$(declare -f configure_platform | sed '1s/configure_platform/configure_platform_real/')"
configure_platform() {
  configure_platform_real
  NATIVE_ROOT="$case_root/native"
  DSM_VOLUME="$case_root"
  INSTALL_DIR="$NATIVE_ROOT/bin"; DATA_DIR="$NATIVE_ROOT/data"
  SHARE_DIR="$NATIVE_ROOT/share"; BACKUP_DIR="$NATIVE_ROOT/backups"
  CONFIG_FILE="$NATIVE_ROOT/config/config.yaml"; LOG_DIR="$NATIVE_ROOT/logs"
}
DSM_VERSION_FILE="$test_root/DSM_VERSION"
MOUNTS_FILE="$test_root/mounts"
GROUP_FILE="$test_root/group"
printf 'majorversion="7"\nos_name="DSM"\n' > "$DSM_VERSION_FILE"
printf '/dev/test /volume1 btrfs rw 0 0\n' > "$MOUNTS_FILE"
printf 'modudock:x:1234:\n' > "$GROUP_FILE"
id() {
  case "$*" in
    '-u') echo 0;; '-u modudock') if [[ ${missing_dsm_user:-0} == 0 ]]; then echo 999; else return 1; fi;;
    '-Gn modudock') echo 'users modudock';; *) command id "$@";;
  esac
}
getent() { fail 'DSM called getent'; }
useradd() { fail 'DSM called useradd'; }
runuser() { fail 'DSM called runuser'; }
synogroup() { fail 'existing DSM group was changed'; }
su() {
  [[ "$1" == -s && "$3" == -g && "$4" == modudock && "$5" == -c && "$7" == modudock ]] || fail 'invalid DSM su arguments'
  [[ ${su_failure:-0} == 0 ]] || return 1
  if [[ "$6" == 'id -u '* ]]; then echo 999; else bash -c "$6"; fi
}
stat() {
  if [[ "$*" == "-c %u $case_root/native" || "$*" == "-c %u $INSTALL_DIR" ]]; then echo 0; else command stat "$@"; fi
}
new_case dsm-fresh
run_install || { cat "$case_root/output"; fail 'DSM fresh install'; }
[[ -x "$case_root/native/bin/modudock" && -d "$case_root/native/data" ]] || fail 'DSM files not installed to native volume root'
grep -Eq "^ReadWritePaths=$case_root/native/data( |$)" "$UNIT_FILE" || fail 'DSM unit is not writable in the data directory'

new_case dsm-upgrade
mkdir -p "$case_root/native/bin" "$case_root/native/data/plugins"
chmod 755 "$case_root/native"
printf database > "$case_root/native/data/core.db"
printf key > "$case_root/native/data/auth.key"
printf plugin > "$case_root/native/data/plugins/data"
printf '#!/bin/bash\necho old\n' > "$case_root/native/bin/modudock"
chmod 755 "$case_root/native/bin/modudock"
printf custom-dsm-unit > "$UNIT_FILE"
active=1
run_install || { cat "$case_root/output"; fail 'DSM upgrade'; }
[[ $(cat "$UNIT_FILE") == custom-dsm-unit ]] || fail 'existing DSM unit overwritten'
backup=$(find "$case_root/native/backups" -mindepth 1 -maxdepth 1 -type d | head -1)
[[ $(cat "$backup/data/core.db") == database && $(cat "$backup/data/auth.key") == key && $(cat "$backup/data/plugins/data") == plugin ]] || fail 'DSM full backup missing'
grep -q old "$backup/modudock" || fail 'DSM previous binary backup missing'

new_case dsm-denied-su
mkdir -p "$case_root/native/bin" "$case_root/native/data"
chmod 755 "$case_root/native"
printf database > "$case_root/native/data/core.db"
printf old > "$case_root/native/bin/modudock"
printf existing > "$UNIT_FILE"
active=1; su_failure=1
if run_install; then fail 'DSM rejected su was accepted'; fi
if grep -q '^stop ' "$events"; then fail 'DSM service stopped before su preflight'; fi
su_failure=0
new_case dsm-no-account
missing_dsm_user=1
if run_install; then fail 'DSM installer accepted missing service account'; fi
[[ ! -e "$case_root/native" ]] || fail 'missing DSM account changed installation files'
echo 'PASS DSM full installation flow without getent/useradd/runuser; missing account fails before installation writes.'
