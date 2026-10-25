# 검증 기록

2026-10-24 로컬 Windows 개발 환경과 Docker Desktop Linux 엔진에서 확인했다. 최종 제품 코드는 `ebd47c4`이며 아래 마지막 검증 결과는 전체 리뷰의 네 가지 수정을 포함한다. 원격 배포는 하지 않았다.

## 초기 기반 검사 이력

- Go 1.27.3, Node.js 24.22.1, npm11.19.1.
- Task1 `go test ./...`, `go vet ./...` 통과. 실제 SQLite3.54.1의 WAL/FK/busy_timeout을 연결 교체 후에도 확인했다.
- 인증 경합 재현 후 수정. 토큰 생성 도중 원래 자격이 폐기된7개 순서, Argon2 슬롯 제한4개 경로를 검증했다.
- 프론트 `npm test -- --reporter=dot`10개 통과, `npm run build` 성공, `npm audit`0건.
- `scripts/build.ps1`로 Windows 실행파일 생성. `node scripts/smoke.mjs`로 내장 UI/관리자 생성/로그인/CSRF/계정 생성/token scope/API404/OpenAPI를 확인했다.
- 브라우저에서 계정 목록·로그·사이드바 접기,390x844모바일 메뉴 열기와 이동 후 닫힘을 확인했다.
- `docker build -t modudock-go:local .` 성공. `docker compose -p modudock-go-smoke up -d --no-build` 기동,UID65532:65532,health 정상. 소스나 Docker socket/호스트 루트 mount 없음.
- `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/release.yml` 통과.
- Windows에서 `go test -race ./...`는 C 컴파일러/CGO가 없어 미실행. 최종 Linux Docker 검증으로 보완한다.

## 통합 1차 검사 이력

- Windows `go test ./...` 통과. core 73.128초, cmd 6.183초. `go vet ./...`, `git diff --check`도 통과했다.
- Linux Docker `go test -race ./...` 통과. core 157.692초, cmd 1.811초, web 1.097초. 실제 Unix 손자 프로세스 취소 테스트를 포함한다.
- `TestGitHubHTTPInstallRunSettingsScheduleUpdateRollbackRecovery`가 설치·설정·메뉴·작업·스케줄·업데이트·롤백·재시작 복구·고정 릴리스를 검증했다. 네트워크만 fixture이고 SQLite·ZIP·실행파일·하위 프로세스·HTTP handler는 실제다.
- `TestSettingsMasksAndPreservesArraySchemaSecrets`의 6개 subtest는 수정 전 원문 노출/비밀값 덮어쓰기를 재현했고 수정 후 통과했다. JSON Pointer 배열 참조와 `unevaluatedItems`의 PUT/GET 마스킹·마스크/빈값 보존을 검사한다.
- 최종 프론트 `npm test -- --reporter=dot` 20개 통과, `npm run build` 성공, `npm audit` 0건.
- `node scripts/build.mjs`로 최종 UI·라이선스를 내장한 Windows 실행파일 생성. `node scripts/smoke.mjs dist/modudock-windows-amd64.exe` 통과.
- `docker buildx build --platform linux/amd64,linux/arm64 --load --build-arg VERSION=0.1.0-dev -t modudock-go:local .` 통과. 이미지는 로컬에만 저장했다. amd64 이미지 약 27.2MB, UID 65532:65532다.
- 두 이미지에서 실행파일을 추출하고 Linux amd64 및 ARM64 에뮬레이션의 Node 컨테이너에서 같은 `scripts/smoke.mjs`를 실행해 모두 통과했다. 관리자 생성·내장 UI·로그인·CSRF·사용자 생성·API token scope·API 404·OpenAPI를 확인한다.
- 실제 최종 Windows 서버에서 개요·플러그인·스케줄·로그 API 연결, 데스크톱 접기, 390×844 모바일 메뉴 열기→로그 이동→닫힘을 확인했다. 로그의 가로 스크롤은 로그 영역 내부에만 있고 페이지 너비는 viewport와 같았다.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -show=verbose ./...` 종료 0. 실제 호출 경로와 import된 패키지 취약점은 0건이다. 사용하지 않는 `x/crypto/openpgp`의 모듈 수준 경고 `GO-2026-5932` 1건은 남으며, 심볼 제거 바이너리의 모듈 단위 경고와 구분한다.
- 최종 Linux Docker `go vet ./...` 종료 0. amd64·arm64 컨테이너 모두 healthy, HTTP `/healthz`는 `ok`, 비root UID 65532:65532 및 읽기 전용 rootfs를 확인했다.
- 문서와 동일한 Compose의 중지 → one-shot `admin create --password-stdin` → 재시작 → 실제 HTTP 로그인·세션 조회도 전용 테스트 볼륨에서 통과했다.
- `CGO_ENABLED=0 GOOS=darwin GOARCH=amd64/arm64 go build ... ./cmd/modudock` 두 크로스 컴파일이 종료 0이었다. macOS에서 실제로 실행한 결과는 아니다.
- 최종 `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/release.yml` 종료 0, 진단 없음.

## 최종 리뷰 수정과 검증

전체 변경 독립 리뷰의 Important 3건과 Minor 1건을 한 번의 수정 작업으로 처리하고 `c828f78..ebd47c4` 범위 재검토에서 네 건 모두 승인받았다. 추가 차단 결함은 없었다.

- 다른 schema draft·named/dynamic/recursive 참조를 거부하고 2020-12 및 로컬 JSON Pointer만 지원한다. 중첩 dialect·인코딩된 fragment·annotation 안의 참조 대상도 검사한다. 배열 Pointer·unevaluatedItems·이스케이프 Pointer의 마스킹과 기존값 보존은 유지한다.
- 비활성화 후 실패한 멱등 작업이 다시 queued로 남지 않도록 retry SQL에 원자적 active 조건을 넣었다. failed 종료·종료 시각·재활성화 후 재선점 방지와 정상 active retry를 실제 프로세스/SQLite로 검사했다.
- 작업 없는 플러그인의 스케줄 화면 충돌, 저장된 필수 비밀값을 편집 후 비웠을 때 제출이 막히는 문제를 수정했다. 서버가 플러그인 0개를 반환하는 최종 실제 브라우저에서도 스케줄 추가 시 오류 대신 안내가 표시됐다.

최종 명령 결과는 다음과 같다.

| 검사 | 결과 |
| --- | --- |
| Windows `go test ./... -count=1` | 4패키지 통과. core 60.792초, cmd 2.942초, hello 0.707초, web 1.138초. |
| Windows 및 Linux Docker `go vet ./...` | 종료 0, 진단 없음. |
| Linux Docker `go test -race ./...` | 종료 0. core 272.716초, cmd 2.697초, hello/web cache 통과. |
| frontend `npm test` | 5개 파일, 26개 테스트 통과. |
| frontend `npm run build` 및 `node scripts/build.mjs` | TypeScript/Vite/Windows 내장 UI 실행파일 빌드 성공. |
| 최종 build의 `npm ci` audit | 취약점 0건. |
| `govulncheck@v1.9.0 -show=verbose ./...` | 종료 0, 호출 경로·import 패키지 취약점 0건. 미사용 OpenPGP 모듈 경고 1건은 위 설명대로 남음. |
| Docker amd64·arm64 multiarch build | 성공, 로컬 `modudock-go:local`에만 저장. |
| `scripts/smoke.mjs` | 최종 Windows·Linux amd64·ARM64 에뮬레이션 실행파일 모두 PASS. |
| 최종 amd64·arm64 컨테이너 | 모두 healthy, HTTP health ok, UID 65532:65532, read-only rootfs. |
| macOS amd64·arm64 `go build` | 두 크로스 컴파일 종료 0. 실행은 미확인. |

`dist/`에 5개 실행파일, 제3자 라이선스, `SHA256SUMS`를 보관했다. 검증용 Windows 서버·Docker 컨테이너·테스트 볼륨은 정리했으며, 기존 Modudock과 NAS 데이터는 건드리지 않았다.

기존 개발 DB에 저장된 미지원 dialect/anchor manifest를 자동 이관하는 기능은 없다. 이 신규 규격을 운영에 배포한 적은 없으며, 개발 중 그러한 manifest를 저장했다면 지원 범위로 갱신해야 한다.

## 로컬에서 확인할 수 없는 항목

- 실제 Synology DSM/CPU/커널에서 네이티브 서비스·부팅 등록·파일 권한.
- 실제 ARM64 장비 실행. 에뮬레이션과 실제 하드웨어 검증은 구분한다.
- macOS 실제 실행·서명/공증.
- GitHub Actions 원격 실행, GHCR 발행, 새 공개 저장소의 실제 플러그인 Release 설치.

GitHub push·NAS 업로드·운영 데이터 이관은 하지 않았다. 플러그인은 신뢰 코드이며 별도 프로세스만으로 악성 코드를 격리하지 않는다.

## 2026-10-25 계정 MFA·Passkey 최종 검증

제품 변경 최종 커밋은 `a3d950a`다. 독립 검토가 찾은 Important 4건과 보안 안내 누락 1건을 실패 테스트로 재현한 후 수정했으며, 재리뷰 대신 전체 회귀 검사로 확인했다. UI에서 계정 전환/화면 이탈 뒤 응답과 인증기 작업을 중단하며, 계정 보안 변경은 진행 중인 익명 Passkey 인증도 폐기한다.

| 검사 | 실제 결과 |
| --- | --- |
| Windows `.tools/go/bin/go.exe test ./... -count=1` | 4패키지 통과. cmd 4.117초, hello 0.884초, core 70.328초, web 1.254초. |
| Linux Docker `go test -race ./... -count=1` | 4패키지 통과. core 161.590초. 초기 테스트 픽스처의 recorder 경합을 수정하고 대상 테스트 10회 및 전체 검사를 다시 통과했다. |
| Windows/Linux `go vet ./...`, `go mod verify` | 종료 0. 의존성 파일 검증 통과. |
| `npm.cmd --prefix frontend test` | 7개 파일, 40개 테스트 통과. |
| `npm.cmd --prefix frontend run build` | TypeScript/Vite 8.3.5 빌드 성공. |
| `node scripts/build.mjs` 및 Windows `scripts/smoke.mjs` | 실행파일 생성과 실제 HTTP·내장 UI·계정·CSRF·토큰 범위·OpenAPI 검증 통과. |
| Docker buildx `--platform linux/amd64,linux/arm64 --load` | 두 아키텍처 로컬 이미지 빌드 성공. |
| 두 Docker 이미지의 실행파일 스모크 | amd64 및 arm64 에뮬레이션 모두 PASS. |
| 두 이미지의 MFA HTTP 검사 | QR/TOTP 등록, 10개 복구 코드, 복구 로그인, 남은 9개 조회, 오프라인 초기화, healthcheck 통과. nonroot 65532:65532, read-only rootfs, auth.key 0600/32바이트 확인. |
| Chrome 156 가상 인증기 | 비밀번호 로그인→QR·수동 키→TOTP 활성화→로그아웃 후 코드 유지→복구 로그인→UV Passkey 등록/로그인/이름 변경/재인증/삭제 통과. 390px 가로 넘침 없음, 페이지 오류 없음. |
| `govulncheck@v1.9.0 -show=verbose ./...` | 종료 0. 호출·import 경로 취약점 0건. 기존 미사용 OpenPGP 모듈 경고 GO-2026-5932 1건은 남음. |
| 최종 `npm ci` audit | 취약점 0건. 276개 라이선스 고지 섹션 생성. |
| macOS amd64/arm64 `go build` | 크로스 컴파일 성공. 실제 macOS 실행은 미검증. |

검증된 이미지는 `modudock-go:mfa-local`, 로컬 배포 파일은 `dist/modudock-go-mfa.tar`다. `docker load -i modudock-go-mfa.tar` 후 Compose의 `MODUDOCK_IMAGE=modudock-go:mfa-local`을 설정해 사용할 수 있다. 이전 `dist/modudock-go-amd64.tar`는 이번 빌드가 아니다.

실제 휴대전화 QR 스캔·Windows Hello·물리 보안 키·다른 브라우저·전체 보조공학 접근성·NAS 설정·운영 DB 복원·대규모 부하/퍼징·외부 의존성 전체 보안 감사는 이번 검증이 아니다. 검토 범위별 판단과 위험은 `account-security/context-notes.md`에 기록했다. 운영 적용 전 서버를 중지하고 core.db·auth.key를 포함한 데이터 디렉터리 전체를 백업해야 한다.
