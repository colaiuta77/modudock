# ModuDock Go

UI가 포함된 Go 실행파일 하나로 설치하는 자동화·플러그인 플랫폼입니다. Docker 또는 Linux 네이티브로 운영하며 Windows/macOS는 개발 실행을 지원합니다.

기존 Python ModuDock과 별도 프로젝트입니다. 기존 DB, FlaskFarm 플러그인, 오늘의 세일은 가져오지 않습니다. 운영 서버에 Go·Node.js·Python·Redis·Git을 설치할 필요가 없습니다.

## 구성

- Go HTTP API, SQLite WAL, 내장 영속 작업 큐와 스케줄러.
- React·Vite 8·Tailwind CSS·shadcn/ui·Lucide와 Precision 디자인의 관리 화면.
- 관리자/운영자/조회자, 계정 관리, 세션·CSRF, 만료·권한 범위가 있는 API 토큰.
- 계정별 TOTP QR 등록·일회용 복구 코드·Passkey와 오프라인 비상 복구.
- GitHub 플러그인 설치·업데이트·롤백, 선언형 메뉴·설정·작업·리소스.
- 작업 이력·취소·진행률, cron·시간대 스케줄, 시스템·작업 로그.

고급 세션 관리 화면, 파일관리, 터미널은 아직 구현 범위가 아닙니다.

## 시작

상세 절차는 [설치·업데이트 안내](docs/installation.md)에 있습니다. 소스 저장소는 [colaiuta77/modudock](https://github.com/colaiuta77/modudock)이며, 실행파일과 Docker 이미지는 `v*` 릴리스 태그의 GitHub Actions가 성공한 뒤 제공됩니다.

첫 공개 릴리스는 [v0.1.0](https://github.com/colaiuta77/modudock/releases/tag/v0.1.0)입니다. Linux amd64·arm64 아카이브와 SHA256SUMS를 제공합니다.

### Linux 네이티브 한 줄 설치

systemd Linux amd64·arm64용입니다. **GitHub Releases에 실행파일과 SHA256SUMS가 발행된 후** 사용할 수 있습니다. 릴리스가 없으면 설치 파일을 바꾸지 않고 중단합니다.

```sh
curl -fsSL https://raw.githubusercontent.com/colaiuta77/modudock/main/scripts/install.sh | sudo bash
```

첫 설치 때 SSH 터미널에서 관리자 이름과 비밀번호를 입력합니다. 기본 접속은 `http://127.0.0.1:7438`이며 외부 공개는 별도 HTTPS 프록시 설정이 필요합니다. 재실행은 기존 계정·데이터·서비스 설정을 보존하며 업데이트합니다. DSM 7은 제어판에서 비관리자 `modudock` 계정을 한 번 생성한 뒤 같은 명령을 사용합니다. DSM 데이터는 기본 `/volume1/modudock/data`이며 [DSM 설치 주의사항](docs/installation.md#synology-dsm)을 먼저 확인하세요.

개발 소스에서 Docker 이미지를 만들고 처음 실행하는 예입니다.

```sh
docker compose build
docker compose run --rm app admin create --data-dir /data --username admin
docker compose up -d --no-build
```

같은 컴퓨터에서 `http://127.0.0.1:7438`로 접속합니다. 비밀번호는 명령줄 인자로 받지 않고 터미널에서 입력합니다. NAS/LAN 공개와 HTTPS 역방향 프록시 설정은 설치 안내를 먼저 확인하세요.

## 소스 빌드

개발 환경은 Go 1.27.3과 Node.js 24.22.1입니다. 결과는 `dist/`에 생성됩니다.

```sh
sh scripts/build.sh
./dist/modudock-linux-amd64 admin create --data-dir ./data --username admin
./dist/modudock-linux-amd64 serve --data-dir ./data
```

Windows에서는 `./scripts/build.ps1`을 사용하고 결과의 `.exe`를 실행합니다. 크로스 빌드는 `GOOS=linux GOARCH=arm64 sh scripts/build.sh`처럼 설정합니다. 실제 릴리스 버전은 `MODUDOCK_VERSION=v0.1.0`으로 주입합니다.

```sh
go test ./...
go vet ./...
npm --prefix frontend test
node scripts/smoke.mjs dist/modudock-linux-amd64
```

프론트만 개발할 때는 API를 `serve --public-url http://127.0.0.1:5173`로 실행하고 `npm --prefix frontend run dev`를 사용합니다. 개발 프록시에도 Origin/CSRF 검사를 유지합니다.

## 문서

- [API 계약](docs/api-contract.md). 실제 로그인 후 `/api/v1/openapi.json`에서도 조회합니다.
- [MFA·Passkey 설정과 비상 복구](docs/account-security.md).
- [Precision UI와 컴포넌트 추가](docs/precision/README.md).
- [플러그인 개발](docs/plugin-development.md).
- [설계 결정](docs/design.md).
- [검증 기록](docs/verification.md).
- [구현 체크리스트](checklist.md), [컨텍스트](context-notes.md).

플러그인은 코어 계정으로 실행하는 신뢰 코드입니다. 별도 프로세스는 악성 코드용 보안 샌드박스가 아니며, 설치 전 저장소와 릴리스를 검토해야 합니다. SQLite 데이터는 로컬 디스크에 두고 하나의 코어 인스턴스만 사용하세요.

## 라이선스와 배포

이 프로젝트는 저장소의 [GNU AGPL-3.0 LICENSE](LICENSE)를 따릅니다. 제3자 라이선스는 빌드 시 실제 의존성에서 수집하여 `/third-party-notices.txt`와 배포 파일에 포함합니다.

GitHub Actions는 main/PR/개발 브랜치를 검증하고 `v*` 태그에서 실행파일과 GHCR 이미지를 발행합니다. 소스 push만으로는 릴리스 파일이 생성되지 않습니다. NAS 업로드·설치는 별도 작업입니다.
