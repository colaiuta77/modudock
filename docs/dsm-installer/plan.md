# DSM 설치 분기와 프록시 안내 계획

## 승인 범위
DSM 7.4.1 x86_64·kernel 5.10.55+에 맞는 네이티브 설치 분기를 추가하고 GitHub main에 게시한다. 기존 일반 Linux 설치는 유지한다. Oracle-A1은 읽기 전용 진단과 수정 명령 안내만 하고 서비스·방화벽을 자동 변경하지 않는다.

## 설계
- DSM VERSION 파일을 읽기만 하여 감지하고 Synology 도구 PATH를 사용한다. getent/useradd/runuser는 일반 Linux에서만 필수이다.
- DSM은 /volume1/modudock 아래 bin/data/backups/share를 사용한다. MODUDOCK_DSM_VOLUME=/volume2 같은 지정도 지원하며 실제 마운트된 볼륨만 허용한다. 기존 Docker 데이터와 자동 병합하지 않는다.
- UID/GID는 id와 계정·그룹 조회로 확인하고 root·administrators 계정은 거부한다. DSM 계정 생성 방식은 사용자 추가 선택에 따른다. 기존 계정의 암호나 그룹 구성원을 덮어쓰지 않는다.
- DSM 권한 전환은 확인된 util-linux su의 -s/-g/-c를 사용하고 Bash %q로 인자를 이스케이프한다. 앱 관리자 비밀번호는 기존처럼 stdin으로만 전달한다.
- DSM unit은 전용 계정, volume 경로, RequiresMountsFor, ReadWritePaths를 사용하고 재시작·기존 설정 보존·전체 백업·데이터 잠금 정책은 유지한다.
- Oracle-A1은 NPM bridge → 10.0.0.129 upstream과 loopback-only listener 불일치로 502임을 읽기 전용으로 확인했다. 사설 IP --listen과 --public-url을 지정하는 systemd drop-in을 안내한다. 포트를 인터넷에 열지 않는다.

## 작업과 검증
1. DSM 도구만 있는 환경·볼륨·계정·권한 전환·생성 unit 테스트를 먼저 RED로 확인한다.
2. Linux/DSM 플랫폼 분기와 공통 서비스 계정 실행 함수를 적용한다.
3. Linux 기존 테스트·DSM 테스트·Bash 문법·독립 보안 리뷰를 통과시킨다.
4. 설치·프록시 안내를 갱신하고 커밋·정상 push·CI 결과를 확인한다. 기존 v0.1.0 태그는 변경하지 않는다.

## 한계
실제 DSM 설치·재부팅·공유 폴더 ACL·ARM 장비 실행은 아직 검증하지 않았다. SPK 패키지·방화벽·기존 Docker 데이터 이전·NAS 원격 자동 설치는 이번 범위가 아니다.

## DSM systemctl 호환 오류 후속 수정
- `scripts/test-install.sh`에서 실제 `check_systemd`를 실행하고, `--value`와 `--now`를 거부하는 구형 systemctl 대역으로 신규 설치·업데이트 실패를 먼저 재현한다.
- `scripts/install.sh`의 모든 속성 조회는 `이름=값` 출력에서 정확한 이름 접두부만 제거한다. 조회 실패는 전파하고, 값 안의 등호와 구형 systemd에 없는 선택 속성의 빈 출력은 보존한다.
- 서비스 활성화와 시작은 각각 `systemctl enable`과 `systemctl start`로 실행한다. 데이터 백업·전용 계정·기존 unit 보존은 변경하지 않는다.
- Bash 문법, Linux/DSM 전체 설치 회귀 테스트와 독립 리뷰를 확인한 뒤 커밋·main 게시·CI를 검증한다. NAS 원격 설치나 새 릴리스는 실행하지 않는다.
