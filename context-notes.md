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
