# 검증 기록

2026-10-23 로컬 Windows 개발 환경과 Docker Desktop Linux 엔진에서 확인했다. 최종 플러그인 통합 전의 중간 기록이며, 아래 미완료 항목은 통합 후 갱신한다.

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

## 최종 확인 대기

- 플러그인 설치/업데이트/롤백·진행률·취소·재시작 복구·스케줄의 실제 통합.
- 최종 소스 기준 Go test/vet/race/govulncheck, frontend test/build/audit.
- Linux amd64/arm64 바이너리·Docker 최종 빌드와 기동.
- 독립 전체 변경 검토와 새로 만든 테스트 컨테이너 정리.

## 로컬에서 확인할 수 없는 항목

- 실제 Synology DSM/CPU/커널에서 네이티브 서비스·부팅 등록·파일 권한.
- 실제 ARM64 장비 실행. 에뮬레이션과 실제 하드웨어 검증은 구분한다.
- macOS 실제 실행·서명/공증.
- GitHub Actions 원격 실행, GHCR 발행, 새 공개 저장소의 실제 플러그인 Release 설치.

GitHub push·NAS 업로드·운영 데이터 이관은 하지 않았다. 플러그인은 신뢰 코드이며 별도 프로세스만으로 악성 코드를 격리하지 않는다.
