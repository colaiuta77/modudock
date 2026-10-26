#!/usr/bin/env bash
# 검증한 Linux 릴리스를 전용 계정과 systemd 서비스로 설치하고 기존 데이터를 보존한다.
set -euo pipefail
set +x
umask 077

REPOSITORY=colaiuta77/modudock
INSTALL_DIR=/usr/local/bin
DATA_DIR=/var/lib/modudock
UNIT_FILE=/etc/systemd/system/modudock.service
SHARE_DIR=/usr/local/share/modudock
BACKUP_DIR=/var/backups/modudock
LOCK_FILE=/run/modudock-installer/install.lock
WORK_DIR=''
BACKUP_PATH=''
STAGED_BINARY=''
DATA_LOCK_PID=''
DATA_LOCK_INPUT=''
PLATFORM=linux
DSM_VERSION_FILE=/etc.defaults/VERSION
MOUNTS_FILE=/proc/mounts
GROUP_FILE=/etc/group
DSM_VOLUME=''
NATIVE_ROOT=''

log() { printf '%s\n' "[ModuDock] $*" >&2; }
die() { log "$*"; exit 1; }
detect_arch() {
  case "$1" in
    x86_64|amd64) printf amd64;;
    aarch64|arm64) printf arm64;;
    *) die "Unsupported CPU architecture: $1 (amd64/arm64 only).";;
  esac
}
detect_platform() {
  if [[ -f "$DSM_VERSION_FILE" ]] && grep -q '^os_name="DSM"$' "$DSM_VERSION_FILE"; then printf dsm; else printf linux; fi
}
configure_platform() {
  PLATFORM=$(detect_platform)
  if [[ "$PLATFORM" == dsm ]]; then
    [[ $(awk -F '"' '$1=="majorversion=" {print $2}' "$DSM_VERSION_FILE") == 7 ]] || die 'DSM 7 is required.'
    DSM_VOLUME=${MODUDOCK_DSM_VOLUME:-/volume1}
    [[ "$DSM_VOLUME" =~ ^/volume[1-9][0-9]*$ ]] || die 'MODUDOCK_DSM_VOLUME must be a mounted /volumeN path.'
    awk -v volume="$DSM_VOLUME" '$2==volume {type=$3} END {exit !(type=="btrfs" || type=="ext4")}' "$MOUNTS_FILE" || die 'The selected DSM volume must be a mounted local btrfs/ext4 volume.'
    PATH="/usr/syno/sbin:/usr/syno/bin:$PATH"
    NATIVE_ROOT="$DSM_VOLUME/modudock"
    INSTALL_DIR="$NATIVE_ROOT/bin"
    DATA_DIR="$NATIVE_ROOT/data"
    SHARE_DIR="$NATIVE_ROOT/share"
    BACKUP_DIR="$NATIVE_ROOT/backups"
  fi
}
systemd_property() {
  local property=$1 output
  shift
  output=$(systemctl show --property="$property" "$@") || return 1
  [[ -z "$output" || "$output" == "$property="* ]] || return 1
  printf '%s\n' "${output#"$property="}"
}
check_systemd() {
  systemd_property Version >/dev/null || die 'A running systemd manager is required.'
}
service_account_exists() {
  if [[ "$PLATFORM" == dsm ]]; then id -u modudock >/dev/null 2>&1; else getent passwd modudock >/dev/null; fi
}
validate_service_account() {
  local uid groups
  uid=$(id -u modudock) || die 'Create a non-administrator modudock account in DSM Control Panel, deny access to unrelated shared folders, then rerun the installer.'
  [[ "$uid" =~ ^[0-9]+$ && "$uid" != 0 ]] || die 'The modudock service account must not be root.'
  if [[ "$PLATFORM" == dsm ]]; then
    groups=" $(id -Gn modudock) "
    [[ "$groups" != *' administrators '* ]] || die 'The modudock service account must not be a DSM administrator.'
  fi
}
ensure_dsm_group() {
  local gid
  gid=$(awk -F: '$1=="modudock" {print $3}' "$GROUP_FILE")
  if [[ -z "$gid" ]]; then
    synogroup --add modudock modudock || die 'Could not create the dedicated DSM group.'
    gid=$(awk -F: '$1=="modudock" {print $3}' "$GROUP_FILE")
  fi
  [[ "$gid" =~ ^[0-9]+$ && "$gid" != 0 ]] || die 'The modudock group must exist and must not be root.'
}
as_service() {
  if [[ "$PLATFORM" == dsm ]]; then
    local command
    printf -v command '%q ' "$@"
    su -s "$BASH" -g modudock -c "$command" modudock 9>&-
  else
    runuser -u modudock -- "$@" 9>&-
  fi
}
verify_service_execution() {
  [[ $(as_service id -u </dev/null) == "$(id -u modudock)" ]] || die 'Cannot switch to the service account non-interactively. Ensure it is enabled and does not require a password change.'
}
prepare_install_lock() {
  local directory=${LOCK_FILE%/*}
  [[ ! -L "$directory" ]] || die 'Installer lock directory must not be a symbolic link.'
  [[ ! -e "$directory" || (-d "$directory" && -O "$directory") ]] || die 'Installer lock directory must be owned by the installer user.'
  install -d -m 0700 "$directory"
  [[ ! -L "$LOCK_FILE" && (! -e "$LOCK_FILE" || (-f "$LOCK_FILE" && -O "$LOCK_FILE")) ]] || die 'Installer lock must be an owned regular file.'
  exec 9<>"$LOCK_FILE"
  flock -n 9 || die 'Another ModuDock installation is running.'
}
prepare_service_unit() {
  if [[ "$PLATFORM" == dsm ]]; then
    awk -v volume="$DSM_VOLUME" -v data="$DATA_DIR" -v binary="$INSTALL_DIR/modudock" '
      /^Description=/ {print; print "RequiresMountsFor=" volume; next}
      /^StateDirectory/ {next}
      /^WorkingDirectory=/ {print "WorkingDirectory=" data; next}
      /^ExecStart=/ {print "ExecStart=" binary " serve --data-dir " data " --listen 127.0.0.1:7438"; next}
      /^ProtectSystem=/ {print; print "ReadWritePaths=" data; next}
      {print}
    ' "$WORK_DIR/modudock.service" > "$WORK_DIR/modudock-dsm.service"
    mv "$WORK_DIR/modudock-dsm.service" "$WORK_DIR/modudock.service"
  fi
}
prepare_native_root() {
  [[ "$PLATFORM" == dsm ]] || return 0
  [[ ! -L "$DSM_VOLUME" && ! -L "$NATIVE_ROOT" ]] || die 'DSM native root must not be a symbolic link.'
  if [[ -e "$NATIVE_ROOT" ]]; then
    [[ -d "$NATIVE_ROOT" && $(stat -c %u "$NATIVE_ROOT") == 0 ]] || die 'Existing DSM native root must be a root-owned directory.'
    local mode
    mode=$(stat -c %a "$NATIVE_ROOT")
    (( (8#$mode & 0022) == 0 )) || die 'DSM native root must not be writable by group or others.'
  else
    install -d -m 0755 -o root -g root "$NATIVE_ROOT"
  fi
}
https() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --tlsv1.2 --retry 3 --connect-timeout 15 --max-time 300 "$@"
}
release_version() {
  local version=${MODUDOCK_VERSION:-} url
  if [[ -z "$version" ]]; then
    url=$(https -o /dev/null -w '%{url_effective}' "https://github.com/$REPOSITORY/releases/latest") || die 'Cannot find a published release. Publish a vX.Y.Z GitHub Release first.'
    [[ "$url" == "https://github.com/$REPOSITORY/releases/tag/"* ]] || die 'Unexpected latest release URL.'
    version=${url##*/}
  fi
  [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || die 'MODUDOCK_VERSION must be a tag such as v0.1.0.'
  printf '%s' "$version"
}
validate_paths() {
  local path
  for path in "$INSTALL_DIR" "$INSTALL_DIR/modudock" "$DATA_DIR" "$DATA_DIR/core.lock" "$UNIT_FILE" "$SHARE_DIR" "$BACKUP_DIR" "$LOCK_FILE"; do
    [[ ! -L "$path" ]] || die "Refusing a symbolic-link installation target: $path"
  done
  [[ ! -e "$DATA_DIR" || -d "$DATA_DIR" ]] || die 'Data path is not a directory.'
  [[ ! -e "$UNIT_FILE" || -f "$UNIT_FILE" ]] || die 'Service path is not a regular file.'
}
download_release() {
  local version=$1 arch=$2 asset expected actual entry
  asset="modudock-linux-$arch.tar.gz"
  local base="https://github.com/$REPOSITORY/releases/download/$version"
  log "Downloading $version for linux/$arch."
  https "$base/$asset" -o "$WORK_DIR/$asset" || die 'Release download failed; no installation files were changed.'
  https "$base/SHA256SUMS" -o "$WORK_DIR/SHA256SUMS" || die 'Release checksums could not be downloaded.'
  expected=$(awk -v name="$asset" '$2==name || $2=="*"name {print $1}' "$WORK_DIR/SHA256SUMS")
  [[ "$expected" =~ ^[0-9a-fA-F]{64}$ ]] || die 'Release checksum is missing, duplicated or malformed.'
  actual=$(sha256sum "$WORK_DIR/$asset"); actual=${actual%% *}
  [[ "${expected,,}" == "$actual" ]] || die 'SHA-256 verification failed. The running installation was not changed.'
  # 필요한 파일만 표준출력으로 읽어 아카이브 경로·심볼릭 링크를 설치 경로에 풀지 않는다.
  for entry in "modudock-linux-$arch" packaging/modudock.service third-party-notices.txt LICENSE; do
    local name=${entry##*/}
    tar -xzOf "$WORK_DIR/$asset" "./$entry" > "$WORK_DIR/$name" || die "Cannot read release file: $entry"
    [[ -s "$WORK_DIR/$name" ]] || die "Missing release file: $entry"
  done
  CANDIDATE="$WORK_DIR/modudock-linux-$arch"
}
prompt_admin() {
  local LC_ALL=C
  if ! { exec 3<>/dev/tty; } 2>/dev/null; then
    die 'First installation needs a terminal for the administrator account. Run from an interactive SSH session.'
  fi
  IFS= read -r -p 'Administrator username [admin]: ' -u 3 ADMIN_USERNAME || die 'Administrator input cancelled.'
  ADMIN_USERNAME=${ADMIN_USERNAME:-admin}
  [[ "$ADMIN_USERNAME" =~ ^[A-Za-z0-9_.-]{1,64}$ ]] || die 'Username must contain 1-64 ASCII letters, digits, dots, underscores or hyphens.'
  IFS= read -r -s -p 'Administrator password (12-1024 bytes): ' -u 3 ADMIN_PASSWORD || die 'Password input cancelled.'
  printf '\n' >&3
  local confirmation
  IFS= read -r -s -p 'Repeat password: ' -u 3 confirmation || die 'Password input cancelled.'
  printf '\n' >&3
  [[ "$ADMIN_PASSWORD" == "$confirmation" ]] || die 'Passwords do not match.'
  [[ ${#ADMIN_PASSWORD} -ge 12 && ${#ADMIN_PASSWORD} -le 1024 ]] || die 'Password must contain 12-1024 bytes.'
  unset confirmation
  exec 3>&-
}
validate_existing_service() {
  local execution arguments value property data_count=0
  execution=$(systemd_property ExecStart modudock) || die 'Cannot inspect the existing service.'
  [[ "$execution" == *"path=$INSTALL_DIR/modudock ;"* && "$execution" == *'argv[]='* ]] || die 'Existing service uses a different executable; migrate it manually before updating.'
  arguments=${execution#*'argv[]='}; arguments=${arguments%%' ;'*}
  [[ ${execution#*'argv[]='} != *'argv[]='* ]] || die 'Multiple service start commands are not supported.'
  local args=()
  read -r -a args <<< "$arguments"
  [[ ${args[0]:-} == "$INSTALL_DIR/modudock" && ${args[1]:-} == serve ]] || die 'Unsupported service command.'
  set -- "${args[@]:2}"
  while (($#)); do
    case "$1" in
      --data-dir|-data-dir)
        [[ $# -ge 2 && "$2" == "$DATA_DIR" ]] || die 'Existing service uses a different data directory; automatic update refused.'
        ((data_count+=1)); shift 2;;
      --data-dir=*|-data-dir=*)
        [[ ${1#*=} == "$DATA_DIR" ]] || die 'Existing service uses a different data directory; automatic update refused.'
        ((data_count+=1)); shift;;
      --listen|-listen|--public-url|-public-url)
        [[ $# -ge 2 ]] || die 'Invalid service arguments.'; shift 2;;
      --listen=*|-listen=*|--public-url=*|-public-url=*) shift;;
      *) die 'Unsupported or environment-expanded service arguments; migrate the unit to explicit paths before updating.';;
    esac
  done
  [[ $data_count == 1 ]] || die 'Existing service must name exactly one supported absolute data directory.'
  [[ $(systemd_property User modudock) == modudock ]] || die 'Existing service must run as the modudock account.'
  [[ $(systemd_property Group modudock) == modudock ]] || die 'Existing service must use the modudock group.'
  for property in RootDirectory RootImage BindPaths BindReadOnlyPaths TemporaryFileSystem; do
    value=$(systemd_property "$property" modudock) || die 'Cannot inspect service filesystem settings.'
    [[ -z "$value" ]] || die "Unsupported service filesystem mapping: $property. Automatic update refused."
  done
}
acquire_data_lock() {
  # 서비스 계정 권한으로만 잠금 파일을 열어 root의 링크 추적·소유권 변경을 피한다.
  coproc MODUDOCK_DATA_LOCK {
    as_service bash -c 'exec 8<>"$1"; flock -n 8 || exit 1; printf "locked\n"; IFS= read -r release' -- "$DATA_DIR/core.lock"
  }
  DATA_LOCK_PID=$MODUDOCK_DATA_LOCK_PID
  DATA_LOCK_INPUT=${MODUDOCK_DATA_LOCK[1]}
  local output=${MODUDOCK_DATA_LOCK[0]} result
  IFS= read -r -t 10 -u "$output" result && [[ "$result" == locked ]] || die 'Cannot lock data as the service account. Stop manually started processes and check data permissions.'
}
release_data_lock() {
  if [[ -n "$DATA_LOCK_INPUT" ]]; then
    printf 'release\n' >&"$DATA_LOCK_INPUT" || true
    exec {DATA_LOCK_INPUT}>&-
    wait "$DATA_LOCK_PID" || true
    DATA_LOCK_INPUT=''; DATA_LOCK_PID=''
  fi
}
backup_installation() {
  [[ -d "$DATA_DIR" || -e "$INSTALL_DIR/modudock" || -f "$UNIT_FILE" ]] || return 0
  install -d -m 0700 "$BACKUP_DIR"
  BACKUP_PATH=$(mktemp -d "$BACKUP_DIR/$(date -u +%Y%m%dT%H%M%SZ).XXXXXX")
  [[ ! -d "$DATA_DIR" ]] || cp -a -- "$DATA_DIR" "$BACKUP_PATH/data" || die 'Data backup failed. Existing binary was not replaced.'
  [[ ! -f "$INSTALL_DIR/modudock" ]] || cp -a -- "$INSTALL_DIR/modudock" "$BACKUP_PATH/modudock" || die 'Binary backup failed.'
  [[ ! -f "$UNIT_FILE" ]] || cp -a -- "$UNIT_FILE" "$BACKUP_PATH/modudock.service" || die 'Service backup failed.'
  [[ ! -d /etc/systemd/system/modudock.service.d ]] || cp -a /etc/systemd/system/modudock.service.d "$BACKUP_PATH/service-overrides" || die 'Service override backup failed.'
  log "Backup retained at $BACKUP_PATH (includes core.db, auth.key and plugin data)."
}
cleanup() {
  local status=$?
  release_data_lock
  unset ADMIN_PASSWORD
  case "$STAGED_BINARY" in "$INSTALL_DIR"/.modudock.new.*) rm -f -- "$STAGED_BINARY";; esac
  case "$WORK_DIR" in /tmp/modudock-install.*) rm -rf -- "$WORK_DIR";; esac
  if ((status != 0)) && [[ -n "$BACKUP_PATH" ]]; then
    log "Installation failed. Preserve $BACKUP_PATH. Stop modudock and restore the old binary AND full data directory together if rollback is needed."
    log 'Inspect: systemctl status modudock; journalctl -u modudock -n 100'
  fi
  return "$status"
}
main() {
  (($# == 0)) || die 'Usage: install.sh (optional MODUDOCK_VERSION=vX.Y.Z and MODUDOCK_HEALTH_URL=http://127.0.0.1:7438).'
  [[ $(uname -s) == Linux ]] || die 'Linux only.'
  [[ $(id -u) == 0 ]] || die 'Run as root, for example curl ... | sudo bash.'
  local command arch version load_state new_install=0 account_exists=0
  configure_platform
  for command in curl tar sha256sum awk uname mktemp install cp mv chmod chown flock systemctl cut date sleep grep id; do
    command -v "$command" >/dev/null || die "Required command is missing: $command"
  done
  if [[ "$PLATFORM" == dsm ]]; then
    for command in synogroup su stat; do command -v "$command" >/dev/null || die "Required DSM command is missing: $command"; done
    validate_service_account
  else
    for command in getent useradd runuser; do command -v "$command" >/dev/null || die "Required command is missing: $command"; done
  fi
  check_systemd
  arch=$(detect_arch "$(uname -m)")
  validate_paths
  prepare_install_lock
  WORK_DIR=$(mktemp -d /tmp/modudock-install.XXXXXX)
  trap cleanup EXIT
  version=$(release_version)
  download_release "$version" "$arch"
  prepare_service_unit
  [[ -f "$DATA_DIR/core.db" ]] || new_install=1
  if ((new_install)); then prompt_admin; fi
  if service_account_exists; then account_exists=1; validate_service_account; fi
  if [[ "$PLATFORM" == dsm ]]; then ensure_dsm_group; verify_service_execution; prepare_native_root; fi
  load_state=$(systemd_property LoadState modudock) || die 'Cannot inspect the existing service load state.'
  if [[ -f "$UNIT_FILE" || "$load_state" == loaded ]]; then
    validate_existing_service
  fi
  [[ ! -d "$DATA_DIR" || $account_exists == 1 ]] || die 'Existing data has no modudock service account; migrate it manually before updating.'
  if systemctl is-active --quiet modudock; then systemctl stop modudock || die 'Could not stop the existing service.'; fi
  validate_paths
  if [[ -d "$DATA_DIR" ]]; then
    acquire_data_lock
  fi
  backup_installation
  if ((account_exists == 0)); then
    useradd --system --user-group --home-dir "$DATA_DIR" --shell /usr/sbin/nologin modudock
  fi
  if [[ "$PLATFORM" == linux ]]; then verify_service_execution; fi
  install -d -m 0700 -o modudock -g modudock "$DATA_DIR"
  install -d -m 0755 "$INSTALL_DIR" "$SHARE_DIR"
  STAGED_BINARY="$INSTALL_DIR/.modudock.new.$$"
  install -m 0755 "$CANDIDATE" "$STAGED_BINARY" || die 'Could not stage the new binary.'
  mv -f -- "$STAGED_BINARY" "$INSTALL_DIR/modudock" || die 'Could not replace the binary.'
  STAGED_BINARY=''
  install -m 0644 "$WORK_DIR/LICENSE" "$WORK_DIR/third-party-notices.txt" "$SHARE_DIR/"
  if [[ ! -f "$UNIT_FILE" ]]; then
    install -m 0644 "$WORK_DIR/modudock.service" "$UNIT_FILE"
  else
    log 'Existing systemd unit and overrides preserved.'
  fi
  release_data_lock
  if ((new_install)); then
    printf '%s\n' "$ADMIN_PASSWORD" | as_service "$INSTALL_DIR/modudock" admin create --data-dir "$DATA_DIR" --username "$ADMIN_USERNAME" --password-stdin || die 'Administrator creation failed. The service was not started; create an administrator manually before starting it.'
    unset ADMIN_PASSWORD
  fi
  systemctl daemon-reload || die 'systemd daemon-reload failed.'
  systemctl enable modudock || die 'systemd could not enable ModuDock.'
  systemctl start modudock || die 'systemd could not start ModuDock.'
  local attempt healthy=0
  for attempt in {1..15}; do
    if systemctl is-active --quiet modudock && as_service "$INSTALL_DIR/modudock" healthcheck --url "${MODUDOCK_HEALTH_URL:-http://127.0.0.1:7438}"; then healthy=1; break; fi
    sleep 1
  done
  ((healthy)) || die 'Service did not become healthy. For a custom port, set MODUDOCK_HEALTH_URL to its local HTTP origin.'
  log "Installed $version. systemd service: modudock. Default access: http://127.0.0.1:7438"
  log 'LAN/HTTPS access: configure a systemd override and reverse proxy; no firewall rules were changed.'
}

if [[ -z ${BASH_SOURCE[0]:-} || ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
