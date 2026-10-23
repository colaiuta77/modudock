#!/usr/bin/env sh
# 프론트엔드를 내장한 Go 실행파일 빌드를 시작한다.
set -eu
node "$(dirname "$0")/build.mjs"
