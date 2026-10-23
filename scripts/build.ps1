# 프론트엔드를 내장한 Go 실행파일 빌드를 시작한다.
$ErrorActionPreference = 'Stop'
node (Join-Path $PSScriptRoot 'build.mjs')
exit $LASTEXITCODE
