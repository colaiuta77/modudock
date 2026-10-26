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

### 자동 설치와 업데이트

systemd가 실행 중인 Linux amd64·arm64에서 다음 명령을 사용합니다. 공개 GitHub Release에 해당 버전의 아카이브·SHA256SUMS가 있어야 합니다. 최초 릴리스 발행 전에는 다운로드가 실패하며 현재 설치는 변경하지 않습니다.

```sh
curl -fsSL https://raw.githubusercontent.com/colaiuta77/modudock/main/scripts/install.sh | sudo bash
```

스크립트를 검토하고 고정 버전을 설치하려면 먼저 파일로 내려받습니다. 아래 버전은 실제 존재하는 릴리스 태그로 바꿉니다.

```sh
curl -fsSLo install.sh https://raw.githubusercontent.com/colaiuta77/modudock/main/scripts/install.sh
less install.sh
sudo env MODUDOCK_VERSION=v0.2.0 bash install.sh
```

- 첫 설치는 `/dev/tty`에서 로컬 전용·감지 IPv4·전체 IPv4 인터페이스·직접 `host:port` 입력 중 선택하고, 브라우저의 정확한 HTTP(S) origin을 입력합니다. Docker/veth 주소는 후보에서 제외하고 DSM LAN bridge는 유지합니다. 감지는 추천 후보일 뿐 자동 적용하지 않습니다. 관리자 이름과 숨김 비밀번호도 터미널에서 입력하며 기본 비밀번호는 없습니다. 시스템 패키지 설치나 방화벽 변경은 하지 않습니다.
- 실행파일은 `/usr/local/bin/modudock`, 설정은 `/etc/modudock/config.yaml`, 데이터는 `/var/lib/modudock`, 서비스는 `modudock`입니다. 비root `modudock` 계정을 사용하며 미입력 기본값은 loopback `127.0.0.1:7438`입니다. 신규 YAML 설치에는 v0.2.0 이상 설정 지원 실행파일이 필요합니다. 구버전 다운로드는 서비스 중지·설치 파일 변경 전에 거부합니다.
- 네트워크 질문은 `MODUDOCK_LISTEN=192.168.1.100:7438 MODUDOCK_PUBLIC_URL=https://modudock.example.org`로 명시할 수 있습니다. 비대화형 네트워크 기본값은 loopback과 빈 public-url이지만 최초 관리자 생성에는 SSH 터미널이 여전히 필요합니다. `public_url`에는 경로, 끝 슬래시, 사용자정보, query, fragment를 넣지 않습니다. 비우면 기존 자동 Origin 동작을 유지하며 원격 passkey를 위해 명시적 HTTPS origin을 설정해야 합니다.
- 같은 명령을 다시 실행하면 업데이트합니다. 다운로드·SHA-256·필수 파일·설정을 검증한 후 서비스를 중지하고 데이터 전체·기존 실행파일·config·logrotate 설정·unit/drop-in을 `/var/backups/modudock/날짜.고유값`에 보관합니다. 백업은 비밀값을 포함하므로 root만 접근하며 자동 삭제하지 않습니다.
- 기존 서비스 파일과 drop-in, config, 데이터 위치는 덮어쓰거나 자동 이전하지 않습니다. 업데이트에서는 네트워크 질문을 다시 하지 않습니다. config 기반 unit은 실제 config와 CLI override를 해석하고, DB가 없거나 경로가 불명확하면 서비스 중지 전에 거부합니다. 기존 기본 경로 `--data-dir` 서비스도 유지합니다.
- health는 public-url이 아닌 실제 listener에 HTTP로 요청합니다. `0.0.0.0`은 `127.0.0.1`, `[::]`은 `[::1]`로 검사합니다. `MODUDOCK_HEALTH_URL=http://로컬주소:포트`가 명시되면 우선합니다. 최신 버전 대신 고정 버전을 사용하려면 `MODUDOCK_VERSION`도 지정합니다.
- 자동 업데이트는 실행파일 `/usr/local/bin/modudock`, 데이터 `/var/lib/modudock`, 서비스 계정 `modudock`을 명시한 unit만 지원합니다. 다른 데이터 경로·실행파일·환경변수 치환·chroot/경로 매핑이 있는 unit은 서비스 중지 전에 거부합니다. 지원 경로로 수동 이전하거나 수동 업데이트해야 합니다.
- 시작 실패 시 백업 경로와 진단 명령을 출력합니다. DB 마이그레이션 때문에 실행파일만 자동 롤백하지 않습니다. 복구는 서비스를 중지한 뒤 **이전 실행파일과 전체 데이터 백업**을 함께 복원해야 합니다.
- SHA-256은 내려받은 릴리스 파일의 무결성을 검사합니다. 별도 서명 검증은 없으므로 저장소와 HTTPS 배포 경로를 신뢰해야 합니다. 운영에서는 스크립트 검토와 버전 고정을 권장합니다.
- 미리 존재하는 `core.db`가 있으면 관리자 계정 생성은 건너뜁니다. 첫 계정 생성 중 실패한 설치는 서버를 멈춘 상태에서 아래 `admin create` 명령으로 계정을 만든 뒤 다시 실행하세요.

YAML 설정의 최소 예제입니다. `serve --config PATH`에서만 읽으며 기존 Docker·CLI 기본값은 유지합니다. 명시한 `--data-dir`, `--listen`, `--public-url`, `--log-file`은 YAML보다 우선합니다. 알 수 없는 필드, 중복 필드, 여러 YAML 문서, 문자열이 아닌 값과 잘못된 주소는 거부합니다. 관리자 암호·인증 비밀은 넣지 않습니다.

```yaml
data_dir: /var/lib/modudock
listen: 192.168.1.100:7438
public_url: https://modudock.example.org
log_file: ""
```

```sh
sudo /usr/local/bin/modudock config check --config /etc/modudock/config.yaml
sudo systemctl restart modudock
sudo -u modudock /usr/local/bin/modudock healthcheck --config /etc/modudock/config.yaml
```

`config check`는 DB·로그 파일을 만들거나 열지 않습니다. 설정은 root 소유·서비스 그룹 읽기 전용이며 서비스는 설정 디렉터리에 쓸 수 없습니다. 신규 설치는 `/etc/logrotate.d/modudock` 규칙을 준비하지만 logrotate 실행파일과 활성 `logrotate.timer`가 모두 확인될 때만 `/var/log/modudock/service.log` 출력을 활성화합니다. cron 스크립트 파일만으로 실제 예약 실행을 보장할 수 없어 자동 감지하지 않습니다. 사용자 지정 cron을 쓰려면 실제 실행·회전과 서비스의 로그 쓰기 권한을 수동 구성·검증한 후 `log_file`을 설정하세요. timer가 없으면 규칙은 비활성 준비 상태이며 선택적 파일 로그를 끄고 journal과 자체 5 MiB×4 제한 UI 로그만 유지합니다. 기존 config에서 파일 로그가 켜져 있으면 자동 업데이트도 활성 timer와 규칙을 요구하므로 cron 전용 설치는 `log_file`을 비우거나 수동 업데이트합니다. UI의 `data/logs/core.jsonl`은 logrotate 대상으로 삼지 않습니다. 운영 로그는 daily/maxsize 5M, 7개 보존, copytruncate이므로 회전 순간 일부 로그 손실 가능성이 있고 실제 예약 주기에 따라 5 MiB를 넘을 수 있습니다. 예약 작업의 실행을 운영 환경에서 확인하세요.

### 수동 설치

릴리스 실행파일과 SHA256SUMS를 내려받아 해시를 검증합니다. `linux-amd64`는 일반 Intel/AMD NAS, `linux-arm64`는 64비트 ARM NAS용입니다. 32비트 ARM은 초기 지원 대상이 아닙니다.

```sh
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf modudock-linux-amd64.tar.gz
sudo install -m 0755 modudock-linux-amd64 /usr/local/bin/modudock
sudo useradd --system --user-group --home-dir /var/lib/modudock --shell /usr/sbin/nologin modudock
sudo install -d -m 0700 -o modudock -g modudock /var/lib/modudock
sudo -u modudock /usr/local/bin/modudock admin create --data-dir /var/lib/modudock --username admin
```

`packaging/config.yaml`을 준비하고 listen/public_url을 실제 환경에 맞춰 수정한 뒤 다음처럼 설치합니다. 운영 파일 로그는 기본 비활성입니다.

```sh
sudo install -d -m 0750 -o root -g modudock /etc/modudock
sudo install -m 0640 -o root -g modudock packaging/config.yaml /etc/modudock/config.yaml
sudo /usr/local/bin/modudock config check --config /etc/modudock/config.yaml
sudo install -m 0644 packaging/modudock.service /etc/systemd/system/modudock.service
```

```sh
sudo systemctl daemon-reload
sudo systemctl enable modudock
sudo systemctl start modudock
sudo systemctl status modudock
journalctl -u modudock -n 100
```

systemd 예제는 config를 읽고 loopback 접속으로 시작합니다. 직접 LAN 접속이 필요하면 YAML의 listen을 변경하되 방화벽으로 허용 범위를 제한하세요. 전체 인터페이스 바인딩은 인터넷 노출까지 넓힐 수 있습니다. 비root 서비스 계정의 권한이 플러그인 권한이기도 합니다.

### Docker 리버스 프록시에서 네이티브 서버에 연결할 때

프록시가 호스트 프로세스라면 loopback upstream을 사용할 수 있습니다. 프록시가 Docker bridge 모드이면 컨테이너의 `127.0.0.1`은 호스트가 아닙니다. 호스트 사설 IP로 전달한다면 ModuDock도 그 IP에서 수신해야 합니다. 신규 설치는 config의 listen/public_url을 수정합니다. 아래 예시는 기존 CLI 방식 unit을 유지하는 설치에만 해당하며 사설 IP와 도메인을 실제 값으로 바꾸어 `sudo systemctl edit modudock.service`에 저장합니다.

```ini
[Service]
ExecStart=
ExecStart=/usr/local/bin/modudock serve --data-dir /var/lib/modudock --listen 10.0.0.10:7438 --public-url https://modudock.example.org
```

```sh
sudo systemctl daemon-reload
sudo systemctl restart modudock
curl -fsS http://10.0.0.10:7438/healthz
```

`0.0.0.0:7438`도 가능하지만 모든 IPv4 인터페이스에서 받습니다. 사설 IP 바인딩도 공인 NAT나 기존 보안 규칙에 따라 외부에서 접근될 수 있으므로 방화벽과 클라우드 ingress는 프록시 경로만 허용하세요. 7438을 인터넷 전체에 개방하지 않습니다. 프록시 upstream은 HTTP, 외부 주소는 HTTPS이며 `--public-url`은 수신 주소와 별개입니다. SSE 응답 버퍼링도 끕니다.

### Synology DSM

DSM 7의 systemd 설치 분기를 제공합니다. 기준 환경은 사용자 제공 DSM 7.4.1 x86_64·kernel 5.10.55+입니다. 실제 NAS에서의 최초 설치·재부팅은 별도 확인이 필요합니다.

1. DSM 제어판 → 사용자 및 그룹에서 `modudock` 사용자를 한 번 생성합니다. 이 계정은 서비스 실행용이며 웹 관리자 계정과 다릅니다.
2. `administrators` 그룹에 넣지 않습니다. 다른 공유 폴더는 접근 거부, DSM 애플리케이션 권한도 허용하지 않는 것을 권장합니다. 기본 `users` 그룹 권한이 상속될 수 있으므로 이 계정의 실제 권한을 확인하세요.
3. 계정은 활성 상태여야 하며 암호 만료·다음 로그인 시 암호 변경 요구를 해제합니다. 설치기는 이 계정 암호를 받거나 바꾸지 않고 root의 `su`로 실행 권한만 전환합니다.
4. SSH에서 위와 같은 한 줄 설치 명령을 실행합니다. 기본 전용 그룹 `modudock`이 없으면 설치기가 그룹만 생성합니다. 다른 그룹의 구성원은 덮어쓰지 않습니다.

DSM 설치 경로입니다.

```text
/volume1/modudock/bin/modudock
/volume1/modudock/config/config.yaml
/volume1/modudock/data/
/volume1/modudock/logs/
/volume1/modudock/backups/
/volume1/modudock/share/
/etc/systemd/system/modudock.service
```

`/volume1`은 실제로 마운트된 로컬 btrfs/ext4 볼륨이어야 합니다. 신규 설치에서 전용 루트 경로를 질문하며 기본값은 `/volume1/modudock`입니다. 공백·특수문자·상위 경로 이동·심볼릭 링크·네트워크 마운트는 거부합니다. 기존 부모 디렉터리도 root 소유이며 그룹·다른 사용자 쓰기 금지여야 합니다. 다른 루트는 다음처럼 지정하고 업데이트에도 같은 경로를 사용합니다. 기존 `MODUDOCK_DSM_VOLUME=/volume2`도 `/volume2/modudock`으로 호환합니다.

```sh
curl -fsSLo install.sh https://raw.githubusercontent.com/colaiuta77/modudock/main/scripts/install.sh
sudo env MODUDOCK_DSM_ROOT=/volume2/modudock bash install.sh
```

DSM에서는 getent/useradd/runuser를 요구하지 않고 id·synogroup·su·systemctl을 사용합니다. 루트 설치 폴더는 root 소유, 데이터는 modudock 소유로 분리합니다. 기존 서비스 파일과 drop-in은 보존하고 데이터 전체와 MFA auth.key를 같은 볼륨의 backups에 저장합니다. 기존 Docker 볼륨은 수정하거나 자동으로 이전하지 않습니다.

기본 수신은 `127.0.0.1:7438`입니다. 기존 Docker가 7438을 사용 중이면 신규 질문에서 다른 포트를 선택합니다. 서비스는 `/volume1/modudock/bin/modudock serve --config /volume1/modudock/config/config.yaml`로 시작합니다. config와 binary는 root 소유, data/logs는 서비스 쓰기 권한을 가지며 systemd/logrotate 등록은 `/etc`에 둡니다. 기존 수동 unit/drop-in은 유지합니다.

신규 YAML 설치에는 설정 명령을 지원하는 v0.2.0 이상 릴리스가 필요합니다. 실제 서비스 기동·접근 권한·재부팅 자동 시작 및 DSM 대규모 업데이트 후 서비스 유지를 확인해야 합니다. 플러그인은 서비스 계정과 같은 권한으로 실행하는 신뢰 코드이며 악성 플러그인 격리는 제공하지 않습니다.

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
