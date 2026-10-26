# 네이티브 서비스 제거 계획

목표는 `install.sh uninstall`로 실행파일과 systemd 서비스 등록만 제거하고 설정·데이터를 보존하는 것이다. Bash/systemd 기존 설치기를 확장하며 별도 패키지나 실행파일 변경은 하지 않는다.

접근법은 같은 스크립트의 명시적 uninstall 분기이다. 별도 제거 스크립트나 전체 디렉터리 삭제는 사용자 요구와 맞지 않아 제외한다. 기존 CLI 설치의 설정은 unit에 들어 있으므로 unit을 root 전용 백업 디렉터리에 복사한 뒤 삭제한다. drop-in, config, 데이터, 로그, 백업, 공유 고지 파일과 계정은 보존한다.

## Task 1. 안전한 제거와 테스트

- 기존 scripts/install.sh에 인자 없는 설치 동작을 보존하고 정확히 uninstall 한 개만 새로 허용한다. 알 수 없는 인자는 변경 전에 거부한다.
- Linux 표준 binary와 DSM의 선택된 설치 루트를 지원한다. 로드된 서비스의 실제 ExecStart를 확인해 임의 파일을 삭제하지 않는다. DSM custom root는 지원된 절대 경로만 자동 인지하거나 명시적 환경값을 요구한다. 경로가 불명확하면 중단한다.
- 설치와 같은 잠금을 사용한다. 제거에는 릴리스 다운로드, 관리자 입력, 계정 생성이나 필수 계정 검증이 필요하지 않다.
- 삭제 전에 정확한 binary/unit 타깃과 심볼릭 링크·지원 경로를 검증한다. 서비스 파일을 BACKUP_DIR의 고유한 uninstall 디렉터리에 복사한다. 백업·stop·disable 실패 시 실행파일/unit을 삭제하지 않는다.
- systemctl stop/disable 후 binary와 기본 unit 두 파일만 제거하고 daemon-reload한다. --value·--now를 쓰지 않는다. 삭제용 recursive 명령은 사용하지 않는다.
- 이미 제거된 경우 안전하게 성공한다. 서비스 중지 실패·다른 실행파일 unit·symlink·잘못된 인자와 Linux/DSM 경로를 임시 파일 및 명령 경계 대역으로 테스트한다. 보존 대상은 전후 내용이 동일해야 한다.
- 새 scripts/test-uninstall.sh에 한국어 역할 주석을 추가하고 release.yml 검증에 연결한다. docs/installation.md에 명령·보존 범위·재설치 시 자동 이전하지 않는 제한을 기록한다.
- RED 테스트, 최소 구현, 전체 Bash 설치/제거 테스트, 독립 검토, 논리적 커밋 순으로 진행한다. 실제 NAS의 제거는 실행하지 않는다. GitHub 게시는 별도 사용자 응답에 따른다.

## 검증 명령

`bash -n scripts/install.sh scripts/test-uninstall.sh scripts/test-install.sh scripts/test-install-dsm.sh` 및 세 Bash 회귀 스크립트를 실행한다. Git diff 검증과 독립 안전성 리뷰를 통과해야 한다.
