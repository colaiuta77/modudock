# 네이티브 설치기의 접속 주소 설정 설계안

> 설계 승인 전이다. 구현은 승인 후 테스트 우선으로 진행한다.

## 목표와 범위
신규 Linux·DSM 설치에서 리스닝 주소를 선택하고 브라우저용 public-url을 입력하여 수동 systemd 편집을 줄인다. 인증·Origin 검증, Go CLI 기본값, Docker 설정, 방화벽은 변경하지 않는다.

## 접근법
- 전체 인터페이스 0.0.0.0을 무조건 기본 적용하는 방법은 간단하지만 인터넷 노출 범위를 넓힌다.
- 권장안은 감지된 IPv4를 후보로 제시하고 로컬 전용·감지 IP·전체 인터페이스·직접 입력 중 명시적으로 선택하게 하는 것이다. 감지 실패는 직접 입력으로 처리한다.
- public-url은 브라우저에서 사용할 HTTP/HTTPS origin을 별도로 받는다. 경로·계정정보·query·fragment와 systemd 해석 문자를 거부한다. 비우면 기존 Go 동작을 유지하며 passkey에는 명시적 URL이 필요함을 알린다.
- 일반 업데이트는 기존 unit·수동 drop-in과 접속 설정을 그대로 보존한다. 사용자가 이미 두 서버를 수동 설정했으므로 기존 설치 재설정 기능은 이번 최소안에서 제외한다. 이 범위는 사용자 확인 대기이다.
- health 검사는 실제 선택된 로컬 listener를 사용한다. wildcard는 loopback으로 검사하고 public-url로 외부 요청하지 않는다. MODUDOCK_HEALTH_URL 명시값이 우선이다.

## 구현과 검증
1. scripts/test-install.sh·scripts/test-install-dsm.sh에 선택값, IP 감지 실패·Docker 주소 오추천 방지, 잘못된 URL·문자 주입 거부, IP별 health와 기존 설정 보존의 RED 테스트를 작성한다.
2. scripts/install.sh의 /dev/tty 패턴을 재사용하고 prepare_service_unit에 Linux/DSM 공통 네트워크 값을 반영한다. 선택값은 엄격한 입력 검증 후에만 unit에 기록한다.
3. docs/installation.md에 최초 설치 질문과 업데이트 보존 정책, 프록시·방화벽 주의사항을 갱신한다.
4. Bash 전체 회귀·Go tests·독립 리뷰를 통과시키고, 사용자 승인 범위에 맞춰 main 게시와 CI를 확인한다.

## 미검증
실제 DSM의 IP 도구·DHCP 변경·부팅 시 주소 준비와 vendor systemd hardening 지원은 장비 확인이 필요하다. 원격 NAS·Oracle 설정을 자동 변경하지 않는다.
