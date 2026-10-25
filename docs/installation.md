# 설치·백업·업데이트

## 공통 주의사항

- 기존 ModuDock과 다른 디렉터리·볼륨을 사용합니다. 기존 DB를 복사하지 않습니다.
- 기본 포트는 7438입니다. 기존 서비스가 사용 중이면 신규 서비스는 7439 같은 다른 포트를 지정합니다.
- 데이터는 로컬 디스크에 저장합니다. NFS/SMB 공유 폴더의 SQLite와 다중 서버 공유는 지원하지 않습니다.
- 최초 관리자 생성은 서버가 중지된 상태에서 실행합니다. 기본 계정이나 기본 비밀번호는 없습니다.
- 인터넷에서는 HTTPS 역방향 프록시를 사용하고 정확한 외부 Origin을 `--public-url`에 지정합니다. URL에 하위 경로를 붙이는 배포는 지원하지 않습니다.

## Docker

### 현재 로컬 개발본

```sh
docker compose build
docker compose run --rm app admin create --data-dir /data --username admin
docker compose up -d --no-build
docker compose ps
docker compose logs --tail=100 app
```

비밀번호는 숨김 입력입니다. GUI 작업 스케줄러처럼 터미널이 없는 경우 `--password-stdin`으로 안전한 표준입력을 사용합니다. 비밀번호를 명령 인자·Compose 환경변수·Git 파일에 저장하지 마세요.

Compose 기본값은 호스트 loopback 공개입니다. LAN에서 접근하려면 `.env`를 만들고 다음 값을 지정합니다. `.env`는 Git에서 제외됩니다.

```dotenv
MODUDOCK_BIND=0.0.0.0
MODUDOCK_PORT=7439
MODUDOCK_PUBLIC_URL=http://192.168.1.100:7439
```

HTTPS 프록시에서는 `MODUDOCK_PUBLIC_URL=https://새로운-서비스-도메인`으로 바꿉니다. 프록시에서 `/api/v1/events`의 응답 버퍼링을 끄고 장시간 SSE 연결을 허용하세요. NAS와 프록시의 방화벽으로 접근 범위를 제한합니다.

컨테이너는 UID/GID `65532:65532`로 실행합니다. 기본 named volume은 Docker가 권한을 준비합니다. 직접 bind mount로 변경하면 해당 폴더를 이 계정이 읽고 쓸 수 있게 준비해야 합니다. 호스트 루트·Docker socket·기존 ModuDock 볼륨을 연결하지 않습니다.

### 이미지 배포 이후

이미지가 발행되면 설치 사용자는 `compose.yaml`만 내려받고 `.env`에 실제 레지스트리 주소와 고정 버전을 설정합니다. 아래 주소는 예시이며 아직 발행된 이미지가 아닙니다.

```dotenv
MODUDOCK_IMAGE=ghcr.io/소유자/새저장소:0.1.0
```

```sh
docker compose pull
docker compose run --rm --no-deps app admin create --data-dir /data --username admin
docker compose up -d --no-build
```

업데이트 전에 서비스를 중지하고 볼륨 전체를 백업합니다. 새 이미지 태그로 변경한 뒤 `pull`, `up -d --no-build`로 시작하고 로그와 health를 확인합니다. `docker compose down -v`는 데이터 볼륨을 지우므로 업데이트에 사용하지 않습니다.

## Linux 네이티브

릴리스 실행파일과 SHA256SUMS를 내려받아 해시를 검증합니다. `linux-amd64`는 일반 Intel/AMD NAS, `linux-arm64`는 64비트 ARM NAS용입니다. 32비트 ARM은 초기 지원 대상이 아닙니다.

```sh
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf modudock-linux-amd64.tar.gz
sudo install -m 0755 modudock-linux-amd64 /usr/local/bin/modudock
sudo useradd --system --home-dir /var/lib/modudock --shell /usr/sbin/nologin modudock
sudo install -d -m 0700 -o modudock -g modudock /var/lib/modudock
sudo -u modudock /usr/local/bin/modudock admin create --data-dir /var/lib/modudock --username admin
```

`packaging/modudock.service`를 `/etc/systemd/system/modudock.service`에 설치한 뒤 필요하면 `ExecStart`에 `--public-url https://도메인`을 추가합니다.

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now modudock
sudo systemctl status modudock
journalctl -u modudock -n 100
```

systemd 예제는 loopback 접속으로 시작합니다. 직접 LAN 접속이 필요하면 `--listen 0.0.0.0:7438`로 변경하되 방화벽으로 허용 범위를 제한하세요. 비root 서비스 계정의 권한이 플러그인 권한이기도 합니다.

### Synology DSM

DSM마다 서비스 등록 방식과 커널이 다르므로 최초 운영은 Docker 방식을 권장합니다. 네이티브는 해당 CPU용 바이너리의 `version`, `healthcheck`부터 실제 장비에서 검증하세요.

전용 일반 계정과 `/volume1/docker/ModuDock-Go` 같은 새 폴더를 준비합니다. 관리자 생성 후 DSM 작업 스케줄러의 사용자 정의 부팅 작업을 전용 계정으로 등록합니다.

```sh
cd /volume1/docker/ModuDock-Go
exec ./modudock serve --data-dir ./data --listen 127.0.0.1:7439 --public-url https://새로운-서비스-도메인
```

이 방식은 systemd처럼 자동 재시작을 보장하지 않습니다. DSM에서 프로세스 종료·자동 시작·권한을 확인해야 하며, 현재 실제 DSM 기동 검증은 하지 않았습니다. 플러그인 실행파일이 요구하는 최소 커널도 확인해야 합니다.

## 백업과 복원

MFA를 사용하면 `core.db`와 함께 `auth.key`가 반드시 필요합니다. 키 누락 시 MFA는 자동 해제되지 않습니다. [계정 보안·비상 복구 안내](account-security.md)를 확인하세요. 이번 DB v3를 구버전으로 되돌릴 때는 업데이트 전 전체 백업도 복원해야 합니다.

1. 서비스를 정상 중지하고 프로세스 종료를 확인합니다.
2. 데이터 디렉터리/볼륨 전체를 백업합니다. DB만 복사하지 말고 플러그인 릴리스·전용 데이터·설정도 포함합니다.
3. 백업을 별도 안전한 경로에 두고 파일 권한을 유지합니다. 설정·세션·토큰 해시와 플러그인 비밀값이 포함될 수 있습니다.
4. 새 실행파일/이미지로 교체하고 시작합니다. 실패하면 먼저 중지한 뒤 이전 실행파일과 짝이 맞는 전체 백업으로 복구합니다.

DB 마이그레이션을 이전 코드가 해석한다고 가정하지 마세요. 플러그인 코드 롤백도 데이터 스키마 버전이 같을 때만 허용되며 데이터 자체를 되돌리지는 않습니다.

## 외부 API

관리 화면의 API 토큰에서 필요한 scope와 만료만 지정합니다. 토큰은 생성 직후 한 번만 표시됩니다.

```sh
curl -H "Authorization: Bearer $MODUDOCK_TOKEN" https://서비스-도메인/api/v1/status
```

토큰을 URL이나 로그에 넣지 않습니다. `/api/v1/openapi.json`은 인증이 필요합니다. 공개 API 지원은 무인증 공개를 의미하지 않습니다.
