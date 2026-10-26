#!/usr/bin/env bash
# 설치기 테스트에서 릴리스 실행파일의 설정 조회 계약만 대역한다.
set -euo pipefail
if [[ ${1:-} == config && ${2:-} == check ]]; then
 shift 2
 path=''; field=''; data_dir=data; listen=127.0.0.1:7438; public_url=''; log_file=''
 overrides=()
 while (($#)); do
  case "$1" in
   --config) path=$2; shift 2;;
   --field) field=$2; shift 2;;
   --data-dir|--listen|--public-url|--log-file) overrides+=("$1" "$2"); shift 2;;
   *) exit 2;;
  esac
 done
 [[ -z "$path" || -f "$path" ]] || exit 1
 if [[ -n "$path" ]]; then
  while IFS= read -r line; do
   [[ "$line" != invalid* ]] || exit 1
   key=${line%%:*}; value=${line#*:}; value=${value# }; value=${value#\"}; value=${value%\"}
   case "$key" in data_dir) data_dir=$value;; listen) listen=$value;; public_url) public_url=$value;; log_file) log_file=$value;; esac
  done < "$path"
 fi
 set -- "${overrides[@]}"
 while (($#)); do
  case "$1" in --data-dir) data_dir=$2;; --listen) listen=$2;; --public-url) public_url=$2;; --log-file) log_file=$2;; esac
  shift 2
 done
 case "$field" in '') printf 'Configuration valid.\n';; data_dir) printf '%s\n' "$data_dir";; listen) printf '%s\n' "$listen";; public_url) printf '%s\n' "$public_url";; log_file) printf '%s\n' "$log_file";; *) exit 2;; esac
fi
