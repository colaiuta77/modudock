# 제거 기능 결정 기록

- 2026-10-26. 사용자는 SFPanel의 `bash -s -- uninstall`처럼 같은 설치기로 binary/service만 제거하고 config/data를 보존하도록 요청했다.
- 작업 시작 HEAD17a10f5, worktree clean. 기존 Go 프로젝트에서 이어가며 Python Modudock 프로젝트는 변경하지 않는다.
- 설정 보존은 YAML뿐 아니라 기존 unit/drop-in의 listen/public-url도 포함한다. 기본 unit은 제거 전 root 전용 백업으로 남기고 drop-in은 원위치 보존한다. 계정·데이터·로그·고지·백업 전체 디렉터리를 삭제하지 않는다.
- 스크립트 수정에는 새 바이너리 릴리스가 필요 없다. GitHub main 게시 여부는 사용자에게 비동기로 확인했다. 원격 NAS/Oracle에서 실제 제거는 하지 않는다.
- 사용자가 GitHub main 게시까지 승인했다. 새 버전 태그나 바이너리 릴리스는 만들지 않는다.
- RED `bash scripts/test-uninstall.sh`에서 기존 main이 Usage로 거부하여 Linux uninstall 테스트가 실패했다. 구현 후 Linux/DSM 경로·데이터/설정 보존·unit/drop-in 비공개 백업·반복 실행·stop/disable/backup/속성조회 실패·외부 binary·symlink·공유 잠금 테스트가 GREEN이다.
- 독립 검토에서 chroot/파일시스템 매핑을 해석하지 않고 호스트 경로를 삭제할 위험을 발견했다. 기존 업데이트 검증과 같은 다섯 속성의 비어 있지 않은 값·조회 실패를 중지 전에 거부한다. RED에서 `failed RootDirectory inspection ignored`를 확인한 뒤 검사 루프를 추가했다.
- e359cef 재검토에서 게시 승인받았다. Bash 구문 검사, test-uninstall.sh·test-install.sh·test-install-dsm.sh, go test ./... -count=1 및 git diff --check가 통과했다. 실제 호스트/NAS 제거는 실행하지 않았다.
