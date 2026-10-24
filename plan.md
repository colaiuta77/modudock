# ModuDock Go Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox syntax for tracking.

**Goal:** UI를 내장한 Go 단일 실행파일로 설치하는 신규 자동화·플러그인 플랫폼을 구현한다.

**Architecture:** SQLite가 코어 상태와 영속 큐의 기준이다. HTTP·스케줄러·실행기는 같은 코어에 있고 플러그인은 제한된 별도 프로세스다. 브라우저와 외부 클라이언트는 버전 있는 인증 API를 사용한다.

**Tech Stack:** Go, database/sql·modernc SQLite, net/http·slog, React·TypeScript·Vite 8·Tailwind 4·shadcn/ui·Lucide.

**Spec:** `docs/design.md`, 구현 공통 계약은 `docs/api-contract.md`다.

## Global Constraints

- 기존 FF·오늘의 세일·운영 데이터는 이식하지 않는다.
- Docker와 Linux amd64/arm64 네이티브를 지원하고 Windows/macOS는 개발용으로 한다.
- Celery·Redis·Python·런타임 Git 의존성을 도입하지 않는다.
- 브라우저는 세션 쿠키·CSRF, 외부 연동은 해시 저장·scope·만료·폐기 API 토큰을 사용한다.
- 파일·작업·네트워크의 경계 입력은 검증한다. 플러그인 프로세스를 보안 샌드박스라 부르지 않는다.
- 새 소스 파일 첫 줄은 한국어 역할 주석이다. 변경은 apply_patch, 사용자 데이터 수정·원격 push 금지다.
- 중요한 로직은 실패하는 동작 테스트부터 작성하고 전체 테스트·빌드 후 커밋한다.

## Review Focus

- 역할 변경·계정 비활성화 이후 기존 세션·토큰이 계속 사용되지 않아야 한다. Task 1에서 회귀 테스트한다.
- 정상 API 오류·미인증 경로가 SPA HTML로 응답하지 않아야 한다. Task 1과 4에서 경로 테스트한다.
- 플러그인 archive 경로 이탈·zip bomb·다른 저장소 다운로드·secret 상속을 거부해야 한다. Task 2에서 경계 테스트한다.
- 재시작·취소·업데이트 경합에서 중복 작업·영구 queued·잘못된 release 실행이 발생하지 않아야 한다. Task 2에서 검증한다.
- 모바일 메뉴·API 실패·마스킹된 설정값 저장이 실제 상태와 맞고 데이터 손실을 만들지 않아야 한다. Task 3에서 검증한다.

## Task 1: Go 코어와 인증 API

**Files:** `go.mod`, `go.sum`, `cmd/modudock/main.go`, `internal/core/{server,db,auth,users,tokens,logs}.go`, 대응 `_test.go`, `internal/web/{web.go,web_test.go,dist/.gitkeep}`.

**Interfaces:** 모듈은 `modudock`. `core.Open(dataDir string, options Options) (*Server,error)`, `(*Server).Handler() http.Handler`, `(*Server).Close() error`, `(*Server).CreateAdmin(username,password string) error`, `Options{Version,PublicURL string,WebFS fs.FS}`를 제공한다. 후속 Task 2는 같은 internal/core 패키지에 파일을 추가하고 DB·route·principal 도우미를 재사용한다. 인터페이스를 변경해야 하면 문서와 호출부를 함께 맞춘다.

- [x] 실제 SQLite와 httptest로 로그인·세션 폐기·CSRF 거부·토큰 scope/만료·마지막 관리자 보호·계정 변경 폐기 테스트를 작성하고 실패를 확인한다.
- [x] DB 초기화·마이그레이션·디렉터리 lock, 인증·권한·사용자·API token·감사 로그 API를 계약대로 구현한다. SQL injection과 malformed JSON을 거부하고 세션 쿠키는 HttpOnly/SameSite이며 PublicURL이 https면 Secure다. Origin은 명시 PublicURL 또는 요청 Host와 비교하고 forwarded header는 신뢰하지 않는다.
- [x] 로그는 용량 제한 회전 JSONL과 stdout, 감사 DB다. 요청별 ID·민감정보 제외를 적용한다. `/status`는 초기엔 실제 빈 작업 집계, `/events`는 재조회 신호, `/openapi.json`은 최종 모든 계약 경로를 포함하도록 단계별 확장한다.
- [x] CLI `serve --data-dir --listen --public-url`, `admin create --data-dir --username --password-stdin`, `healthcheck --url`, `version`을 구현한다. 대화형 비밀번호는 숨김 입력, 비대화형은 stdin이며 argv/환경에 비밀번호를 넣지 않는다. admin create는 서버 중지 상태에서 데이터 lock을 소유한다.
- [x] SPA는 빌드 산출물이 없을 때 명시적인 개발 안내만 표시하고, 빌드된 FS가 있으면 포함해 제공한다. `/api/`나 `/healthz`를 SPA fallback으로 처리하지 않는다.
- [x] focused test 후 `go test ./...`, `go vet ./...`로 확인하고 자체 리뷰 후 의미 단위 커밋한다.

## Task 2: 플러그인·작업·스케줄

**Files:** `internal/core/{plugins,manifest,runner,runner_unix,runner_windows,jobs,schedules}.go`, 대응 테스트, `examples/hello/main.go`, `examples/hello/modudock.template.json`, `docs/plugin-development.md`.

**Interfaces:** Task 1의 실제 Server/DB/auth 도우미를 먼저 읽는다. 외부 계약은 `docs/api-contract.md` 그대로다. 내부 Runner는 context 취소·progress·stderr callback을 지원한다. DB 변경은 기존 순차 마이그레이션에 추가한다. Open/Close에 작업/스케줄 생명주기를 통합한다.

- [x] manifest invalid ID·schema·unknown runtime, archive traversal·symlink·size/digest, child timeout/cancel·malformed messages를 실제 파일·프로세스로 검증하는 실패 테스트를 먼저 작성한다.
- [x] HTTPS GitHub ref→commit→manifest→release zip→검증→health→원자적 DB 활성화를 구현한다. URL host/repo/redirect allowlist를 검증하고 private IP·임의 host를 허용하지 않는다. 불변 릴리스·전역 설치 mutex·동일 data_schema_version·실행중 참조 보호를 적용한다.
- [x] protocol stdin/stdout·stderr 제한, 최소 env, argv 실행, Linux process group 취소와 Windows 개발 취소를 구현한다. JSON Schema는 검증된 라이브러리로 검사하고 외부 ref fetch를 막는다.
- [x] SQLite 작업 큐는 commit 후 wake, 원자적 claim, 제한 동시성·대기 크기, 재시작 queued 복구·running interrupted, idempotency key payload 비교, cancel·timeout, release pin을 구현한다. 실패 재시도는 선언된 idempotent 작업만 최대 3회로 제한한다.
- [x] 5필드 cron·IANA 시간대 CRUD, unique due slot, 트랜잭션 enqueue+next update, 중단 missed skip·overlap skip을 구현한다. 새 schedule의 next time은 서버가 계산한다.
- [x] plugin 설정·menu·resources API를 연결한다. secret 입력은 마스킹 값을 보존하고 viewer에게 노출하지 않는다. UI schema form/table이 사용할 manifest를 검증한다.
- [x] 모든 주요 상태 전이·재시작·취소·schema mismatch·업데이트 후 고정 release·schedule overlap 테스트 후 `go test ./...`, `go vet ./...`와 가능한 환경 race test를 실행하고 커밋한다.

## Task 3: Lucide 관리 콘솔

**Files:** `frontend/` 전체. 기존 Modudock frontend에서 package 설정·CSS·shadcn 컴포넌트는 검토 후 재사용한다. 앱·API 호출은 신규 계약에 맞춘다. 세일 코드는 복사하지 않는다.

**Interfaces:** `docs/api-contract.md`를 따른다. 동일 origin `/api/v1`; 로그인/재개 응답의 csrf_token을 메모리에 보관해 변경 요청에 전달한다. 세션을 local/sessionStorage에 저장하지 않는다.

- [x] 로그인 실패·권한에 따른 메뉴·사이드바 저장·모바일 닫힘·CSRF 요청·API 실패 메시지·설정 secret 보존 동작을 검증하는 테스트를 먼저 작성한다.
- [x] Vite 8·React·Tailwind·shadcn을 유지하고 Lucide named import로 일관된 아이콘을 적용한다. App, api, 설정·플러그인·작업·로그 페이지를 명확히 나눈다.
- [x] 계정·비밀번호·API token CRUD, plugin GitHub install/update/activate/disable/settings, menu 편집, schema 기반 작업 실행과 resource 테이블, runs cancel/progress/detail, schedules CRUD, logs 필터·자동 새로고침을 구현한다.
- [x] 접근성·모바일 내부 표 스크롤·빈 상태·연결 오류를 다룬다. 고정된 외부 서비스·세일 기능과 숫자 가짜 지표는 넣지 않는다.
- [x] 라이선스 고지에 Lucide를 포함하고 `npm test`, `npm run build`, `npm audit`를 검증한다. 실제 backend에 연결해 데스크톱·모바일 화면을 확인하고 커밋한다.

## Task 4: 배포·통합·문서

**Files:** `Dockerfile`, `compose.yaml`, `.dockerignore`, `.github/workflows/release.yml`, `scripts/build.ps1`, `scripts/build.sh`, `packaging/modudock.service`, `README.md`, `docs/installation.md`, `docs/verification.md`.

**Interfaces:** frontend build는 `internal/web/dist`로 복사한 후 Go embed 빌드한다. image entrypoint는 `/app/modudock`, 명령 serve·주소 `0.0.0.0:7438`·data-dir `/data`; compose는 API 하나·volume 하나·init·healthcheck만 둔다.

- [x] Linux/Windows용 build script가 명시 출력 디렉터리에 UI 포함 바이너리를 만드는지 실제 실행해 확인한다. 지정 출력만 교체하고 다른 데이터는 건드리지 않는다.
- [x] 비root multi-stage image·AMD64/ARM64·영속 volume·systemd 예제·DSM 부팅 작업 및 중지/백업/업데이트 절차를 작성한다. 관리자 생성은 서비스 중지 후 one-shot CLI로 실행한다.
- [x] GitHub Actions는 PR/branch에서 검사·빌드, v* 태그에서 바이너리 checksum·GHCR image를 배포하도록 설정한다. 실제 원격 게시하지 않는다.
- [x] OpenAPI 계약과 실제 API·UI를 통합 검증한다. 새 데이터에서 로그인→예제 plugin→작업→스케줄→재시작 복구, Docker 동일 경로, native health를 확인한다.
- [x] 독립 리뷰 후 중요한 결함을 고치고 테스트·빌드·취약점 검사를 재실행한다. 확인한 환경과 미확인 NAS 제한을 문서에 기록하고 로컬 커밋한다.
