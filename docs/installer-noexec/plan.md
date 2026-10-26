# noexec 임시 경로 호환 수정 계획

## 근거와 목표
사용자 NAS의 /proc/mounts에서 `/tmp`는 tmpfs rw,nosuid,nodev,noexec, `/volume1`은 실행 가능한 로컬 btrfs임을 확인했다. 현재 설치기는 /tmp 후보를 chmod한 뒤 실행하고 실패를 YAML 미지원으로 오진한다. 직접적인 마운트 증거가 있어 추가 가설 나열 대신 격리된 noexec 마운트로 같은 경로를 재현한다.

## 구현과 검증
1. 별도 마운트 네임스페이스의 noexec 임시 폴더로 기존 설치기의 /tmp 작업 경로를 유도하여 Permission denied와 잘못된 YAML 안내를 재현한다. 기존 설치 테스트의 mktemp 명령 경계만 대역한다.
2. WORK_DIR를 검증된 INSTALL_DIR 아래 root 전용 `.modudock-install.XXXXXX`로 만든다. DSM native root 검증을 먼저 하고 설치 디렉터리의 symlink·소유권·쓰기 권한을 검사한다. 기존 디렉터리 권한을 완화하지 않는다.
3. SHA-256 검증·후보 설정 검사 이후에만 기존 서비스를 중지하고 실행파일을 교체한다. 실행 상태 126/127 등은 경로·마운트/권한 문제로 안내하고, 정상 실행 후 출력이 다를 때만 기능 미지원으로 분류한다.
4. cleanup은 생성된 정확한 전용 WORK_DIR만 지우며 bin/root/data/backup 부모는 삭제하지 않는다. 성공·실패 정리와 검사 실패 시 기존 설치 보존을 테스트한다.
5. Linux·DSM·uninstall 회귀 및 실제 noexec 회귀 테스트, 독립 리뷰 후 main에 게시한다. 마운트 보안 설정 변경·NAS 원격 작업·새 binary release는 하지 않는다.

예상 변경 파일은 scripts/install.sh, scripts/test-install.sh, 새 scripts/test-install-noexec.sh, release.yml, docs/installation.md와 본 작업 기록이다.
