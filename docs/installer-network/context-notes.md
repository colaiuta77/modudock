# 결정과 확인 기록
- 2026-10-26. 사용자가 NAS 설치 성공 후 loopback을 수동 변경했다고 알렸고, 신규 설치기에 0.0.0.0 또는 감지한 로컬 IP 선택과 리버스 프록시 public-url 입력을 요청했다.
- 사용자는 Invalid origin이 public-url 수정으로 해결됐다고 확인했다. 인증·CSRF·Origin·passkey 코드는 이번 변경 대상이 아니다.
- 현재 c37343b이며 작업 시작 시 worktree는 깨끗했다. 모든 systemctl 속성 조회는 구형 옵션과 호환되는 systemd_property를 사용한다. --value·--now를 다시 도입하지 않는다.
- 현재 prepare_service_unit은 DSM 경로만 변환하며 신규 unit은 고정 loopback이다. 기존 unit/drop-in은 보존한다. 특정 IP를 선택하면 고정 loopback health 검사와 마지막 접속 안내도 함께 바뀌어야 한다.
- 독립 읽기 전용 검토는 기본 업데이트 완전 보존을 권장했다. 기존 설치 재설정까지 추가하면 수동 ExecStart drop-in 충돌 처리와 전용 override 관리가 필요하다. 최소안에서는 신규 설치 입력만 추가하고 이미 수동 설정된 서버의 업데이트는 보존하는 방안을 사용자에게 확인한다.
- IP 자동 감지는 후보 제시일 뿐 자동 적용이 아니다. hostname -I 첫 주소나 Docker/veth 주소를 무조건 선택하지 않는다. systemd unit의 %, $, 공백·개행·따옴표·역슬래시 주입과 URL origin 범위를 엄격하게 검증해야 한다.
- 제품 코드 변경·테스트 실행·서버 설정 변경은 아직 하지 않았다. 설계안 승인 후 구현 계획에 따라 진행한다.
