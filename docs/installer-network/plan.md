# 네이티브 설치기의 접속 주소 설정 설계안

> 2026-10-26. 권장 방식 및 config.yaml·설치 경로 변경, v0.2.0 릴리스 발행을 승인받았다. 아래 확장 계획을 적용한다.

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

## Task 1. YAML 설정과 네이티브 설치 통합

기존 계획의 Go CLI 기본값 유지 원칙은 그대로 두고, 명시적 `serve --config PATH` 지원을 추가한다. 이전 설계의 승인 대기 문구보다 이 승인된 확장 요구사항이 우선한다.

- 작업 위치는 D:/Documents/ChatGPT/ModuDock-Go, 기존 codex/go-platform 브랜치이다. 기존 설치/데이터를 유지하며 새 worktree는 만들지 않는다.
- 실제 YAML 설정은 최소 필드 data_dir, listen, public_url 및 필요한 운영 로그 경로만 지원한다. 유지보수되는 go.yaml.in/yaml/v3를 사용하고 정확한 버전을 고정한다. 알 수 없는 필드·중복·여러 문서·잘못된 타입/주소는 거부한다. 명시적인 CLI 플래그가 설정값보다 우선하며 기존 Docker/CLI 호출은 그대로 동작한다.
- 설치 전 설정 검증은 DB를 열거나 변경하지 않는다. 필요할 경우 최소 `config check` 명령과 healthcheck의 config 지원을 추가한다. 관리자 암호나 인증 비밀을 YAML에 저장하지 않는다.
- 신규 Linux 경로는 binary /usr/local/bin/modudock, config /etc/modudock/config.yaml, data /var/lib/modudock, logrotate /etc/logrotate.d/modudock이다. systemd는 설정 파일을 읽어 시작한다.
- DSM은 기본 /volume1/modudock 아래 bin, config, data, logs, share, backups를 두며 설치 시 사용자 지정 루트를 입력받는다. 실제 로컬 마운트와 경로 안전성을 검증한다. systemd/logrotate 등록 파일은 여전히 /etc에 root로 설치한다. 미리 만든 비관리자 modudock 계정을 사용한다.
- 신규 설치는 /dev/tty로 loopback·감지 IPv4·전체 인터페이스·직접 입력을 선택하고 public-url을 입력받는다. 비대화형 설치는 명시적 환경값 또는 안전한 기본값을 사용한다. IP 후보는 Docker/veth를 제외하되 DSM LAN bridge를 무조건 제외하지 않는다. public-url은 정확한 HTTP(S) origin만 허용한다.
- 일반 업데이트는 기존 unit/drop-in/config 및 데이터 위치를 보존한다. 기존 --data-dir 기반 설치도 정상 업데이트한다. config 기반 설치는 실제 config와 데이터 경로를 확인한다. 경로가 불명확하면 중단하며 새 DB로 바꾸지 않는다. 자동 마이그레이션/방화벽 변경은 하지 않는다.
- healthcheck는 실제 listener를 사용하며 wildcard는 loopback으로 검사한다. MODUDOCK_HEALTH_URL 명시값 우선이다. public-url로 health 요청하지 않는다.
- 기존 UI용 core.jsonl 자체 회전은 유지한다. logrotate는 별도 운영 로그 파일에만 적용하여 이중 회전을 피한다. 파일 로그는 선택적이고 journal도 유지한다. logrotate가 없는 환경에는 회전이 활성화됐다고 주장하지 말고 무제한 파일 증가를 피한다. 로그 디렉터리만 서비스 쓰기 권한을 주고 config는 쓰지 못하게 한다.
- 설치 시작/완료 시 실제 binary/config/data/log 경로, 버전, 정확한 HTTP 또는 입력한 HTTPS 접속 주소, journalctl 및 systemctl 명령을 보여 준다.
- 관련 Go CLI/core 호출부, packaging, installer 양쪽 테스트/fixture, installation 문서까지 함께 수정한다. 신규 코드 파일은 첫 줄 한국어 역할 주석을 쓴다. 인증 로직·프론트·플러그인·원격 NAS/Oracle 서비스는 변경하지 않는다.
- TDD로 config 파싱/우선순위, 로그 전달, 신규 입력/검증, 기존 설치 보존과 DSM 경로를 먼저 실패하게 테스트하고 구현한다. 가장 작은 테스트부터 전체 Go/Bash 검증까지 실행한다.
- 논리적 단위로 커밋한다. 구현자는 GitHub push/tag/release를 하지 않는다. 보고서에는 RED/GREEN 명령·결과, 수정 파일과 미검증 위험을 기록한다.

## Task 2. 검토와 v0.2.0 발행

- Task 1 결과를 독립 검토하고 회귀 테스트·빌드·설정 기반 실행을 검증한다.
- main에 일반 push 후 CI 성공을 확인하고 v0.2.0 태그로 기존 릴리스 워크플로를 실행한다. 태그 재사용/강제 push는 금지한다.
- 릴리스 자산과 체크섬·설정 명령의 실행 여부를 확인하고 실제 NAS/Oracle에서 미실행한 점을 구분한다.
