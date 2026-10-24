# 검증 기록

2026-10-24 로컬 Windows 개발 환경과 Docker Desktop Linux 엔진에서 확인했다. 아래 최종 검사는 UI 수정 `594fc9d`와 비밀값 마스킹 수정 `33dc3dd`를 포함한다.

## 완료한 기반 검사

- Go 1.27.3, Node.js 24.22.1, npm11.19.1.
- Task1 `go test ./...`, `go vet ./...` 통과. 실제 SQLite3.54.1의 WAL/FK/busy_timeout을 연결 교체 후에도 확인했다.
- 인증 경합 재현 후 수정. 토큰 생성 도중 원래 자격이 폐기된7개 순서, Argon2 슬롯 제한4개 경로를 검증했다.
- 프론트 `npm test -- --reporter=dot`10개 통과, `npm run build` 성공, `npm audit`0건.
- `scripts/build.ps1`로 Windows 실행파일 생성. `node scripts/smoke.mjs`로 내장 UI/관리자 생성/로그인/CSRF/계정 생성/token scope/API404/OpenAPI를 확인했다.
- 브라우저에서 계정 목록·로그·사이드바 접기,390x844모바일 메뉴 열기와 이동 후 닫힘을 확인했다.
- `docker build -t modudock-go:local .` 성공. `docker compose -p modudock-go-smoke up -d --no-build` 기동,UID65532:65532,health 정상. 소스나 Docker socket/호스트 루트 mount 없음.
- `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/release.yml` 통과.
- Windows에서 `go test -race ./...`는 C 컴파일러/CGO가 없어 미실행. 최종 Linux Docker 검증으로 보완한다.

## 최종 통합 검사

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
- `CGO_ENABLED=0 GOOS=darwin GOARCH=amd64/arm64 go build ... ./cmd/modudock` 두 크로스 컴파일이 종료 0이었다. macOS에서 실제로 실행한 결과는 아니다.

## 마무리 대기

- 독립 전체 변경 검토와 새로 만든 테스트 컨테이너 정리.

## 로컬에서 확인할 수 없는 항목

- 실제 Synology DSM/CPU/커널에서 네이티브 서비스·부팅 등록·파일 권한.
- 실제 ARM64 장비 실행. 에뮬레이션과 실제 하드웨어 검증은 구분한다.
- macOS 실제 실행·서명/공증.
- GitHub Actions 원격 실행, GHCR 발행, 새 공개 저장소의 실제 플러그인 Release 설치.

GitHub push·NAS 업로드·운영 데이터 이관은 하지 않았다. 플러그인은 신뢰 코드이며 별도 프로세스만으로 악성 코드를 격리하지 않는다.
