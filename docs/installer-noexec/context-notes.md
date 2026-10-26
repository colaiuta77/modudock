# noexec 진단 기록

- 2026-10-26. HEADa88f8e4, worktree clean. NAS 설치가 check_candidate_config의 /tmp 실행에서 Permission denied로 실패했다. Oracle 재설치는 정상이다.
- 사용자가 `/tmp tmpfs rw,nosuid,nodev,noexec`, `/volume1 btrfs rw,nodev,...`를 제공하여 noexec 원인을 확인했다. chmod 부족이나 YAML 버전 문제로 안내하지 않는다.
- 실행 검증 실패 시 기존 서비스·바이너리·데이터는 바꾸지 않는 순서를 유지한다. NAS의 noexec를 해제하지 않고 설치할 실행파일과 같은 실행 가능 파일시스템을 이용한다.
- 실제 NAS 작업은 하지 않는다. 기존 main 게시 승인에 따른 설치기 결함 수정이며 새 바이너리 릴리스는 필요 없다.
- RED `bash scripts/test-install-noexec.sh`는 독립 mount namespace의 실제 noexec tmpfs에서 후보 실행 Permission denied와 기존 잘못된 YAML 미지원 메시지로 실패했다. `bash scripts/test-install.sh`는 추가한 root 소유권 테스트에서 unsafe staging owner accepted로 실패했다.
- WORK_DIR를 INSTALL_DIR 아래 root 전용 mktemp 폴더로 옮겼다. DSM native root를 먼저 검증하고 실행 staging 부모의 root 소유권·group/other 쓰기 금지와 symlink를 확인한다. 기존 서비스 중지·백업·원자적 활성화 순서는 보존한다.
- GREEN 실제 noexec 전체 설치 테스트, 실행 차단 오류 구분, Linux/DSM 설치와 uninstall 회귀가 모두 통과했다. noexec 테스트는 별도 namespace에서만 mount하며 host 마운트 옵션을 변경하지 않는다. 최종 검증 후 독립 리뷰와 main 게시를 진행한다.
- 9cadc5b 독립 안전성 검토에서 게시 승인받았다. root 전용 staging·직계 작업 폴더만 정리·검사 후 서비스 중지 순서·실제 noexec 테스트를 확인했다. 실제 DSM 장비 실행은 아직 하지 않았다.
- main 7b516f9 게시 완료. 원격 install.sh blob ef88547936fc28239b228f38c78231493aad6dba가 로컬과 일치했다. CI https://github.com/colaiuta77/modudock/actions/runs/40289119291에서 실제 noexec 회귀, 설치·제거, Go race/vet, UI/실행파일 smoke, 취약점 검사와 Docker 빌드가 모두 통과했다. main 설치 스크립트를 다시 실행하면 되며 v0.2.0 릴리스 자산이나 NAS 마운트 옵션은 변경하지 않았다.
