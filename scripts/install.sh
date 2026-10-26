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
CONFIG_FILE=/etc/modudock/config.yaml
LOG_DIR=/var/log/modudock
LOGROTATE_FILE=/etc/logrotate.d/modudock
LISTEN=127.0.0.1:7438
PUBLIC_URL=''
LOG_FILE=''
EXISTING_CONFIG=''
EXISTING_SERVICE=0
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
    NATIVE_ROOT=${MODUDOCK_DSM_ROOT:-${MODUDOCK_DSM_VOLUME:-/volume1}/modudock}
    if [[ -z ${MODUDOCK_DSM_ROOT+x} && -z ${MODUDOCK_DSM_VOLUME+x} && ! -f "$UNIT_FILE" && ! -e "$NATIVE_ROOT/bin/modudock" ]] && { exec 3<>/dev/tty; } 2>/dev/null; then
      local root_input
      IFS= read -r -p "DSM installation root [$NATIVE_ROOT]: " -u 3 root_input || die 'DSM root input cancelled.'
      NATIVE_ROOT=${root_input:-$NATIVE_ROOT}
      exec 3>&-
    fi
    safe_path "$NATIVE_ROOT" || die 'DSM root must be an absolute path without spaces, traversal or special characters.'
    [[ "$NATIVE_ROOT" =~ ^(/volume[1-9][0-9]*)/.+ ]] || die 'DSM root must be below a mounted /volumeN path.'
    DSM_VOLUME=${BASH_REMATCH[1]}
    [[ "$DSM_VOLUME" =~ ^/volume[1-9][0-9]*$ ]] || die 'MODUDOCK_DSM_VOLUME must be a mounted /volumeN path.'
    awk -v volume="$DSM_VOLUME" '$2==volume {type=$3} END {exit !(type=="btrfs" || type=="ext4")}' "$MOUNTS_FILE" || die 'The selected DSM volume must be a mounted local btrfs/ext4 volume.'
    PATH="/usr/syno/sbin:/usr/syno/bin:$PATH"
    INSTALL_DIR="$NATIVE_ROOT/bin"
    DATA_DIR="$NATIVE_ROOT/data"
    SHARE_DIR="$NATIVE_ROOT/share"
    BACKUP_DIR="$NATIVE_ROOT/backups"
    CONFIG_FILE="$NATIVE_ROOT/config/config.yaml"
    LOG_DIR="$NATIVE_ROOT/logs"
    validate_dsm_storage
  fi
}
validate_dsm_storage() {
  [[ "$PLATFORM" == dsm ]] || return 0
  local mount type path
  while read -r mount type; do
    for path in "$NATIVE_ROOT" "$INSTALL_DIR" "$DATA_DIR" "${CONFIG_FILE%/*}" "$LOG_DIR" "$SHARE_DIR" "$BACKUP_DIR"; do
      if [[ "$path/" == "$mount/"* && "$type" != btrfs && "$type" != ext4 ]]; then die 'DSM storage cannot be on a nested network or virtual mount.'; fi
    done
  done < <(awk '{print $2, $3}' "$MOUNTS_FILE")
}
safe_path() { [[ "$1" =~ ^/[A-Za-z0-9._/-]+$ && "$1" != *'//'* && "$1/" != *'/../'* && "$1/" != *'/./'* ]]; }
reject_symlink_parents() {
  local path=$1
  while [[ "$path" != / && -n "$path" ]]; do
    [[ ! -L "$path" ]] || die "Refusing a symbolic-link installation path: $path"
    path=${path%/*}
  done
}
valid_port() { [[ "$1" =~ ^[0-9]{1,5}$ ]] && ((10#$1 > 0 && 10#$1 <= 65535)); }
validate_network() {
  [[ "$LISTEN" =~ ^([A-Za-z0-9.-]*|\[[0-9a-fA-F:]+\]):([0-9]+)$ ]] && valid_port "${BASH_REMATCH[2]}" || die 'Listen must be a host:port address with port 1-65535.'
  if [[ -n "$PUBLIC_URL" ]]; then
    [[ "$PUBLIC_URL" =~ ^https?://(\[[0-9a-fA-F:]+\]|[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?)(:([0-9]+))?$ ]] || die 'Public URL must be an exact HTTP(S) origin (no path, slash, credentials, query or fragment).'
    [[ -z ${BASH_REMATCH[4]:-} ]] || valid_port "${BASH_REMATCH[4]}" || die 'Invalid public URL port.'
  fi
}
detect_ipv4() {
  command -v ip >/dev/null || return 0
  ip -o -4 addr show scope global 2>/dev/null | awk '$2 !~ /^(docker[0-9]|veth|br-|virbr|cni|flannel|lo$)/ {split($4,a,"/"); if (a[1] !~ /^(127\.|169\.254\.)/) print a[1]}' | awk '!seen[$0]++' || true
}
configure_network() {
  LISTEN=${MODUDOCK_LISTEN:-127.0.0.1:7438}
  PUBLIC_URL=${MODUDOCK_PUBLIC_URL:-}
  if [[ -z ${MODUDOCK_LISTEN+x} ]] && { exec 3<>/dev/tty; } 2>/dev/null; then
    local choice input i=2
    local candidates=()
    mapfile -t candidates < <(detect_ipv4)
    printf 'Listen choices. 1) Local only (127.0.0.1:7438)\n' >&3
    for input in "${candidates[@]}"; do printf '%s) %s:7438\n' "$i" "$input" >&3; ((i+=1)); done
    printf '%s) All IPv4 interfaces (0.0.0.0:7438)\n%s) Custom host:port\n' "$i" "$((i+1))" >&3
    IFS= read -r -p 'Select listener [1]: ' -u 3 choice || die 'Network input cancelled.'
    choice=${choice:-1}
    [[ "$choice" =~ ^[0-9]{1,2}$ ]] || die 'Invalid listener choice.'
    if ((choice == i)); then LISTEN=0.0.0.0:7438
    elif ((choice == i+1)); then IFS= read -r -p 'Listen host:port: ' -u 3 LISTEN || die 'Network input cancelled.'
    elif ((choice >= 2 && choice < i)); then LISTEN="${candidates[choice-2]}:7438"
    elif ((choice != 1)); then die 'Invalid listener choice.'; fi
    exec 3>&-
  fi
  if [[ -z ${MODUDOCK_PUBLIC_URL+x} ]] && { exec 3<>/dev/tty; } 2>/dev/null; then
    IFS= read -r -p 'Browser HTTP(S) origin (empty for local/automatic origin): ' -u 3 PUBLIC_URL || die 'Public URL input cancelled.'
    exec 3>&-
  fi
  validate_network
  [[ "$LISTEN" != 0.0.0.0:* && "$LISTEN" != '[::]:'* && "$LISTEN" != :* ]] || log 'WARNING: listening on ALL interfaces. Limit LAN/Internet exposure with host, NAS and upstream firewall rules.'
  [[ -n "$PUBLIC_URL" ]] || log 'No public URL set. Configure an explicit HTTPS origin before enabling remote passkeys.'
}
listener_health_url() {
  local host=${LISTEN%:*} port=${LISTEN##*:}
  case "$host" in ''|0.0.0.0) host=127.0.0.1;; '[::]') host='[::1]';; esac
  printf 'http://%s:%s' "$host" "$port"
}
write_config() {
  printf 'data_dir: "%s"\nlisten: "%s"\npublic_url: "%s"\nlog_file: "%s"\n' "$DATA_DIR" "$LISTEN" "$PUBLIC_URL" "$LOG_FILE" > "$WORK_DIR/config.yaml"
}
check_candidate_config() {
  chmod 0700 "$CANDIDATE"
  printf 'data_dir: /modudock-config-capability-probe\n' > "$WORK_DIR/probe.yaml"
  [[ $("$CANDIDATE" config check --config "$WORK_DIR/probe.yaml" --field data_dir) == /modudock-config-capability-probe ]] || die 'Downloaded binary lacks YAML config support. Select a config-capable release (v0.2.0 or newer); installation unchanged.'
}
rotation_available() {
  command -v logrotate >/dev/null && systemctl is-active --quiet logrotate.timer
}
show_installation() {
  log '----------------------------------------'
  log "Version: $1"
  log "Access: ${PUBLIC_URL:-$(listener_health_url)}"
  log "Binary: $INSTALL_DIR/modudock"
  log "Config: ${EXISTING_CONFIG:-$CONFIG_FILE}"
  log "Data: $DATA_DIR"
  log "Operational log: ${LOG_FILE:-disabled (journal and bounded UI logs only)}"
  log "Logrotate rule: $LOGROTATE_FILE"
  log 'Logs: journalctl -u modudock -n 100'
  log 'Status: systemctl status modudock'
  log 'Restart: systemctl restart modudock'
  log 'Stop: systemctl stop modudock'
  log '----------------------------------------'
  [[ "$LISTEN" != 0.0.0.0:* && "$LISTEN" != '[::]:'* && "$LISTEN" != :* ]] || log "Use this host's reachable LAN IP for direct browser access; loopback above is only the local health target."
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
    awk -v platform="$PLATFORM" -v volume="$DSM_VOLUME" -v data="$DATA_DIR" -v binary="$INSTALL_DIR/modudock" -v config="$CONFIG_FILE" -v logs="$LOG_DIR" -v log_file="$LOG_FILE" '
      /^Description=/ {print; if (platform=="dsm") print "RequiresMountsFor=" volume; next}
      /^StateDirectory/ {if (platform!="dsm") print; next}
      /^WorkingDirectory=/ {print "WorkingDirectory=" data; next}
      /^ExecStart=/ {print "ExecStart=" binary " serve --config " config; next}
      /^ReadWritePaths=/ {next}
      /^ProtectSystem=/ {print; print "ReadWritePaths=" data (log_file!="" ? " " logs : ""); next}
      {print}
    ' "$WORK_DIR/modudock.service" > "$WORK_DIR/modudock-native.service"
    mv "$WORK_DIR/modudock-native.service" "$WORK_DIR/modudock.service"
}
prepare_native_root() {
  [[ "$PLATFORM" == dsm ]] || return 0
  local parent=${NATIVE_ROOT%/*} mode
  while [[ "$parent" != "$DSM_VOLUME" && "$parent" != / ]]; do
    if [[ -e "$parent" ]]; then
      [[ -d "$parent" && $(stat -c %u "$parent") == 0 ]] || die 'DSM root parents must be root-owned directories.'
      mode=$(stat -c %a "$parent")
      (( (8#$mode & 0022) == 0 )) || die 'DSM root parents must not be writable by group or others.'
    fi
    parent=${parent%/*}
  done
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
  validate_dsm_storage
  for path in "$INSTALL_DIR" "$INSTALL_DIR/modudock" "$DATA_DIR" "$DATA_DIR/core.lock" "$UNIT_FILE" "$SHARE_DIR" "$BACKUP_DIR" "$LOCK_FILE" "$CONFIG_FILE" "$LOG_DIR" "$LOGROTATE_FILE"; do
    safe_path "$path" || die "Unsafe installation path: $path"
    reject_symlink_parents "$path"
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
  local execution arguments value property data_count=0 config_count=0 config_path='' data_argument=''
  local overrides=()
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
        [[ $# -ge 2 ]] || die 'Invalid data-dir argument.'
        data_argument=$2
        overrides+=(--data-dir "$2")
        ((data_count+=1)); shift 2;;
      --data-dir=*|-data-dir=*)
        data_argument=${1#*=}
        overrides+=(--data-dir "${1#*=}")
        ((data_count+=1)); shift;;
      --config|-config)
        [[ $# -ge 2 ]] || die 'Invalid config argument.'; config_path=$2; ((config_count+=1)); shift 2;;
      --config=*|-config=*) config_path=${1#*=}; ((config_count+=1)); shift;;
      --listen|-listen|--public-url|-public-url|--log-file|-log-file)
        [[ $# -ge 2 ]] || die 'Invalid service arguments.'
        case "$1" in *listen) LISTEN=$2; overrides+=(--listen "$2");; *public-url) PUBLIC_URL=$2; overrides+=(--public-url "$2");; *log-file) LOG_FILE=$2; overrides+=(--log-file "$2");; esac
        shift 2;;
      --listen=*|-listen=*|--public-url=*|-public-url=*|--log-file=*|-log-file=*)
        value=${1#*=}
        case "$1" in *listen=*) LISTEN=$value; overrides+=(--listen "$value");; *public-url=*) PUBLIC_URL=$value; overrides+=(--public-url "$value");; *log-file=*) LOG_FILE=$value; overrides+=(--log-file "$value");; esac
        shift;;
      *) die 'Unsupported or environment-expanded service arguments; migrate the unit to explicit paths before updating.';;
    esac
  done
  [[ $data_count -le 1 && $config_count -le 1 ]] || die 'Existing service has duplicate config/data arguments.'
  if ((config_count)); then
    safe_path "$config_path" || die 'Existing config path must be a supported absolute path.'
    reject_symlink_parents "$config_path"
    [[ -f "$config_path" ]] || die 'Existing config file is missing.'
    check_candidate_config
    local effective=(config check --config "$config_path" "${overrides[@]}")
    DATA_DIR=$("$CANDIDATE" "${effective[@]}" --field data_dir) || die 'Existing config validation failed.'
    LISTEN=$("$CANDIDATE" "${effective[@]}" --field listen) || die 'Cannot resolve configured listener.'
    PUBLIC_URL=$("$CANDIDATE" "${effective[@]}" --field public_url) || die 'Cannot resolve configured public URL.'
    LOG_FILE=$("$CANDIDATE" "${effective[@]}" --field log_file) || die 'Cannot resolve configured log file.'
    safe_path "$DATA_DIR" || die 'Existing config data_dir must be an explicit supported absolute path.'
    [[ -f "$DATA_DIR/core.db" ]] || die 'Configured database is missing; refusing to create a replacement database.'
    [[ -z "$LOG_FILE" ]] || { safe_path "$LOG_FILE" && rotation_available && [[ -f "$LOGROTATE_FILE" ]]; } || die 'Existing file logging needs a safe absolute path, active logrotate.timer and a rule; configure rotation before updating.'
    CONFIG_FILE=$config_path
    EXISTING_CONFIG=$config_path
  else
    [[ $data_count == 1 ]] || die 'Existing service must name exactly one supported absolute data directory.'
    [[ "$data_argument" == "$DATA_DIR" ]] || die 'Existing legacy service uses a different data directory; automatic update refused.'
    EXISTING_CONFIG='existing systemd arguments (preserved)'
    PUBLIC_URL=${PUBLIC_URL%/}
    validate_network
  fi
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
  [[ ! -f "$CONFIG_FILE" ]] || cp -a -- "$CONFIG_FILE" "$BACKUP_PATH/config.yaml" || die 'Config backup failed.'
  [[ ! -f "$LOGROTATE_FILE" ]] || cp -a -- "$LOGROTATE_FILE" "$BACKUP_PATH/logrotate" || die 'Log rotation backup failed.'
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
uninstall_native() {
  [[ $(uname -s) == Linux ]] || die 'Linux only.'
  [[ $(id -u) == 0 ]] || die 'Run uninstall as root.'
  local required state execution binary='' arguments property value backup=''
  for required in systemctl uname id awk grep install cp mktemp date flock rm; do
    command -v "$required" >/dev/null || die "Required command is missing: $required"
  done
  check_systemd
  prepare_install_lock
  state=$(systemd_property LoadState modudock) || die 'Cannot inspect the service; nothing was removed.'
  if [[ "$state" == loaded ]]; then
    [[ $(systemd_property FragmentPath modudock) == "$UNIT_FILE" ]] || die 'Service is not the managed local unit; nothing was removed.'
    execution=$(systemd_property ExecStart modudock) || die 'Cannot inspect the service executable.'
    [[ "$execution" == '{ path='*' ; argv[]='* ]] || die 'Unsupported service command; nothing was removed.'
    binary=${execution#*'path='}; binary=${binary%%' ;'*}
    arguments=${execution#*'argv[]='}; arguments=${arguments%%' ;'*}
    [[ ${execution#*'argv[]='} != *'argv[]='* && "$arguments" == "$binary serve "* ]] || die 'Unsupported service command; nothing was removed.'
    for property in User Group; do
      [[ $(systemd_property "$property" modudock) == modudock ]] || die 'Service identity is not modudock; nothing was removed.'
    done
    for property in RootDirectory RootImage BindPaths BindReadOnlyPaths TemporaryFileSystem; do
      value=$(systemd_property "$property" modudock) || die 'Cannot inspect service filesystem settings; nothing was removed.'
      [[ -z "$value" ]] || die "Unsupported service filesystem mapping: $property. Nothing was removed."
    done
  elif [[ "$state" != not-found || -e "$UNIT_FILE" ]]; then
    die 'Service state is not recognized. Inspect systemctl cat modudock before removal.'
  fi
  PLATFORM=$(detect_platform)
  if [[ "$PLATFORM" == dsm ]]; then
    if [[ -n "$binary" ]]; then
      safe_path "$binary" && [[ "$binary" =~ ^(/volume[1-9][0-9]*/.+)/bin/modudock$ ]] || die 'Unsupported DSM executable; nothing was removed.'
      NATIVE_ROOT=${BASH_REMATCH[1]}
      [[ -z ${MODUDOCK_DSM_ROOT+x} || "$MODUDOCK_DSM_ROOT" == "$NATIVE_ROOT" ]] || die 'Explicit DSM root differs from the service; nothing was removed.'
      [[ -z ${MODUDOCK_DSM_VOLUME+x} || "$NATIVE_ROOT/" == "$MODUDOCK_DSM_VOLUME/"* ]] || die 'Explicit DSM volume differs from the service; nothing was removed.'
      MODUDOCK_DSM_ROOT=$NATIVE_ROOT
    else
      MODUDOCK_DSM_ROOT=${MODUDOCK_DSM_ROOT:-${MODUDOCK_DSM_VOLUME:-/volume1}/modudock}
    fi
    configure_platform
  elif [[ -n "$binary" && "$binary" != "$INSTALL_DIR/modudock" ]]; then
    die 'Service uses a different executable; nothing was removed.'
  fi
  for property in "$INSTALL_DIR/modudock" "$UNIT_FILE" "$BACKUP_DIR"; do
    safe_path "$property" || die 'Unsafe removal path.'
    reject_symlink_parents "$property"
  done
  [[ ! -e "$INSTALL_DIR/modudock" || -f "$INSTALL_DIR/modudock" ]] || die 'Executable target is not a regular file.'
  [[ ! -e "$UNIT_FILE" || -f "$UNIT_FILE" ]] || die 'Unit target is not a regular file.'
  if [[ "$state" == not-found && ! -e "$INSTALL_DIR/modudock" ]]; then
    log 'No managed service or executable found. Config, data and all retained files are unchanged.'
    log 'For an orphaned custom DSM installation, set MODUDOCK_DSM_ROOT explicitly.'
    exec 9>&-
    return
  fi
  if [[ -f "$UNIT_FILE" ]]; then
    install -d -m 0700 -o root -g root "$BACKUP_DIR"
    backup=$(mktemp -d "$BACKUP_DIR/uninstall-$(date -u +%Y%m%dT%H%M%SZ).XXXXXX")
    cp -a -- "$UNIT_FILE" "$backup/modudock.service" || die 'Service settings backup failed; nothing was removed.'
    if [[ -d "${UNIT_FILE}.d" ]]; then
      reject_symlink_parents "${UNIT_FILE}.d"
      cp -a -- "${UNIT_FILE}.d" "$backup/service-overrides" || die 'Service overrides backup failed; nothing was removed.'
    fi
  fi
  if [[ "$state" == loaded ]]; then
    systemctl stop modudock || die 'Service stop failed; executable and unit retained.'
    systemctl disable modudock || die 'Service disable failed; executable and unit retained.'
  fi
  reject_symlink_parents "$INSTALL_DIR/modudock"
  reject_symlink_parents "$UNIT_FILE"
  rm -f -- "$INSTALL_DIR/modudock" "$UNIT_FILE" || die 'Could not remove executable/unit. Config and data were not removed.'
  systemctl daemon-reload || die 'Files removed but systemd reload failed. Run systemctl daemon-reload manually.'
  log "Removed executable: $INSTALL_DIR/modudock"
  log "Removed service registration: $UNIT_FILE"
  [[ -z "$backup" ]] || log "Service settings backup retained: $backup"
  log 'Config, data, logs, backups, drop-ins, logrotate rules and service account were preserved.'
  exec 9>&-
}
main() {
  if [[ $# == 1 && "$1" == uninstall ]]; then uninstall_native; return; fi
  (($# == 0)) || die 'Usage: install.sh [uninstall] (optional MODUDOCK_VERSION, MODUDOCK_DSM_ROOT and MODUDOCK_HEALTH_URL).'
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
  load_state=$(systemd_property LoadState modudock) || die 'Cannot inspect the existing service load state.'
  if [[ -f "$UNIT_FILE" || "$load_state" == loaded ]]; then
    validate_existing_service
    EXISTING_SERVICE=1
  else
    check_candidate_config
    [[ ! -e "$CONFIG_FILE" ]] || die 'Config exists without a recognized service. Resolve its paths manually before installation.'
    configure_network
    if rotation_available; then LOG_FILE="$LOG_DIR/service.log"; else LOG_FILE=''; log 'Scheduled logrotate unavailable: journal and bounded UI logs only; no optional file log enabled.'; fi
    write_config
    "$CANDIDATE" config check --config "$WORK_DIR/config.yaml" >/dev/null || die 'New config is invalid.'
  fi
  validate_paths
  [[ -f "$DATA_DIR/core.db" ]] || new_install=1
  log "Preparing $version on $PLATFORM."
  show_installation "$version"
  if ((new_install)); then prompt_admin; fi
  if service_account_exists; then account_exists=1; validate_service_account; fi
  if [[ "$PLATFORM" == dsm ]]; then ensure_dsm_group; verify_service_execution; prepare_native_root; fi
  prepare_service_unit
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
  if ((EXISTING_SERVICE == 0)); then
    install -d -m 0750 -o root -g modudock "${CONFIG_FILE%/*}"
    install -m 0640 -o root -g modudock "$WORK_DIR/config.yaml" "$CONFIG_FILE"
    install -d -m 0700 -o modudock -g modudock "$LOG_DIR"
    install -d -m 0700 -o root -g root "$BACKUP_DIR"
    install -d -m 0755 "${LOGROTATE_FILE%/*}"
    printf '%s {\n  daily\n  maxsize 5M\n  rotate 7\n  missingok\n  notifempty\n  compress\n  copytruncate\n  su modudock modudock\n}\n' "$LOG_DIR/service.log" > "$WORK_DIR/logrotate"
    install -m 0644 -o root -g root "$WORK_DIR/logrotate" "$LOGROTATE_FILE"
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
    if systemctl is-active --quiet modudock && as_service "$INSTALL_DIR/modudock" healthcheck --url "${MODUDOCK_HEALTH_URL:-$(listener_health_url)}"; then healthy=1; break; fi
    sleep 1
  done
  ((healthy)) || die 'Service did not become healthy. For a custom port, set MODUDOCK_HEALTH_URL to its local HTTP origin.'
  log "Installed $version. systemd service: modudock; no firewall rules were changed."
  show_installation "$version"
}

if [[ -z ${BASH_SOURCE[0]:-} || ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
