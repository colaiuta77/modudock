# ModuDock Go 구현 컨텍스트

## 승인

- 2026-10-23 사용자가 제안 설계와 `D:/Documents/ChatGPT/ModuDock-Go` 신규 프로젝트 생성을 승인했다.
- 제품명은 ModuDock Go, 모듈명은 `modudock`으로 한다. Git 원격 주소는 등록하지 않는다.
- 기존 `D:/Documents/ChatGPT/Modudock` 소스·DB·미추적 데이터는 수정하지 않는다.
- Linux amd64/arm64와 Docker가 우선이며 Windows/macOS는 개발 실행 대상이다.
- 단일 코어·SQLite 큐·별도 프로세스 플러그인·인증 REST API 설계를 구현한다.

## 구현 결정

- 독립 신규 저장소 `codex/go-platform` 브랜치에서 작업한다. 기존 프로젝트 worktree를 추가로 만들 필요가 없다.
- 개발 환경 Go는 공식 파일을 SHA-256 검증하여 `.tools/go`에 설치한다. 사용자 전역 PATH는 변경하지 않는다.
- 새 소스 파일 첫 줄에 한국어 역할 주석을 넣는다. 생성물·설정 파일은 예외다.
- `docs/api-contract.md`의 계약을 프론트·백엔드의 기준으로 사용한다.
- 설치 서버에는 Git·Go·Node·Python·Redis가 필요하지 않다. 개발/CI에서만 Go와 Node를 사용한다.
- 프로세스 분리는 악성 플러그인에 대한 보안 샌드박스가 아니다. 신뢰하는 플러그인만 실행한다.
- 이번 작업은 로컬 구현·검증·커밋까지다. GitHub push와 NAS 업로드는 하지 않는다.

## 진행 기록

- 프론트는 승인된 v1 API 계약만 사용한다. 브라우저 세션/CSRF는 메모리·HttpOnly cookie에 두고, localStorage에는 사이드바 접기 여부만 저장한다.
- 아이콘은 Lucide named import로 필요한 것만 번들에 포함한다. 기존 세일 기능은 복사하지 않았다.
- UI 테스트 10개와 TypeScript/Vite 빌드 통과, npm audit 0건을 확인했다. 수동 새로고침은 편집 폼을 해제하지 않고 데이터를 다시 조회한다.
- Docker는 Debian 13 기반 distroless 비root 이미지와 단일 서비스·단일 볼륨으로 구성한다. 호스트 경로나 Docker socket을 연결하지 않는다.
- 실제 npm/Go 모듈에서 라이선스 원문을 모아 실행파일에 내장한다. npm 배포본에 원문이 없는 react-remove-scroll-bar는 공식 저장소의 고지를 별도로 보관한다.
- 코어 독립 검토에서 자격 폐기와 토큰 발급 사이의 경합, 비밀번호 변경의 Argon2 메모리 제한 누락을 확인했다. 재현 테스트 후 수정하고 재검토를 통과했다.
- Windows 바이너리의 내장 UI·실제 로그인·CSRF·외부 API token scope smoke를 통과했다. 로컬 Docker는 UID 65532로 healthy이며 약 25MB다. 이 결과는 Task 1 기반의 중간 검증으로 최종 Task 2 통합 후 다시 검사한다.
- 브라우저에서 데스크톱 접기와 390px 모바일 메뉴 이동·닫힘, 계정 목록과 로그 표시를 확인했다. 기존 서비스와 겹치지 않게 임시 17438/17439 포트와 별도 테스트 데이터만 사용한다.

## Go 취약점 검사 해석

- `GO-2026-5932`는 `golang.org/x/crypto/openpgp` 관련 경고다. 현재 `go list -deps ./cmd/modudock`에는 Argon2와 Blake2b만 있고 OpenPGP는 없다.
- govulncheck v1.9.0의 binary 검사 구현은 심볼이 제거된 실행파일에 대해 모듈 단위로 보수적으로 경고한다. 따라서 최종 판정은 같은 소스의 source 검사와 실제 import 그래프를 함께 기록한다. 경고를 숨기거나 Argon2를 다른 구현으로 바꾸지 않는다.
- 공식 안내는 https://pkg.go.dev/vuln/GO-2026-5932 이다. 플러그인 구현을 포함한 source 검사에서 호출 경로 0건·import된 패키지 0건, 사용하지 않는 모듈 경로 경고 1건으로 종료 코드 0을 확인했다.

## 2026-10-24 재개

- 보조 작업의 사용량 제한 종료로 UI 수정이 커밋 직전에 멈췄다. 기존 코드를 보존하고 담당 작업을 재개했다.
- UI 수정 594fc9d에서 필수 schema 표시값과 제출값 일치, 메뉴 없는 플러그인 작업 공간 진입, 세션 401 이후 로그인 복귀를 해결했다. 20개 테스트·빌드와 독립 재검토를 통과했고 npm audit는 0건이다.
- Docker Desktop이 중지되어 있었으므로 숨김으로 시작하고 엔진 응답을 확인했다. 기존 운영 Modudock 컨테이너나 NAS는 변경하지 않는다.
- 비밀값 마스킹 수정 33dc3dd에서 JSON Pointer 배열 참조와 unevaluatedItems의 6개 실제 API 회귀를 해결했다. 재검토와 최종 Linux race(core157.692초)/vet를 통과했다.
- Windows 내장 UI 빌드와 native smoke, Linux amd64·arm64 이미지 빌드·health·native smoke를 통과했다. ARM64는 에뮬레이션이다. macOS amd64·arm64는 크로스 컴파일만 확인했다.
- 실제 공개 GitHub 플러그인 release 설치는 아직 수행하지 않았다. 네트워크 fixture와 실제 파일·프로세스·DB 통합으로 설치/업데이트/롤백을 확인했으며, 외부 배포 전 실제 release 검증이 필요하다.
