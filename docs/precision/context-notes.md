# Precision 작업 맥락

- 기준 커밋은 e627449. 실제 프로젝트는 `D:/Documents/ChatGPT/ModuDock-Go`; 과거 Python 저장소는 수정하지 않는다.
- 사용자가 선택 설치와 전체 기존 UI 스타일 적용을 승인하고 진행을 요청했다. 추가 디자인 질의 없이 진행한다.
- 기존 clean `codex/go-platform` 체크아웃을 이어서 사용한다. 별도 worktree·push·NAS 배포는 하지 않는다.
- 공식 GitHub README의 예전 Precision 설명은 charcoal이나 현재 공식 registry/precision의 실제 토큰과 소스는 indigo이며 내부 주석은 Attio Flow다. 현재 설치 대상 레지스트리의 일관된 토큰을 기준으로 삼는다.
- 원격 소스는 `.tools/precision-source`에 참조용으로만 다운로드했다. 설치 스크립트 실행/전역 MCP 설정 변경은 불필요하다.
- 새 동작은 추가하지 않고 기존 UI 계약과 native 폼을 유지한다. CSS 선택적 통합이므로 motion/react 및 중복 개별 Radix 패키지는 설치하지 않는다.
- 단계별 진행과 실제 검증 결과를 이 문서에 계속 기록한다.
- 변경 전과 변경 후 프론트 테스트 40개 통과. 브라우저에서 기존 primary #066fd1이 Precision 기대값과 달라 RED였고, 토큰 적용 후 GREEN이다.
- rem 기준을 바꾸지 않기 위해 루트 16px/body 14px를 유지한다. 초기 14px 루트 실수는 실제 계산 반경 5.25px로 드러나 수정했다.
- `scripts/precision-check.mjs`는 Chrome에서 1440/390px 20개 메뉴 화면, 플러그인 탭/입력, 서랍, QR 등록 UI, 복구 코드 확인, MFA 로그인 UI, 입력 포커스와 가로 넘침을 검증했다. API는 테스트 응답이며 서버 인증 검증은 별개다.
- 기존 package.json/package-lock.json을 변경하지 않았다. CSS transition으로 충분하므로 motion 패키지를 추가하지 않았다.
- 검증 범위를 320/390/768/1024/1440px, 계정·스케줄 편집 폼까지 확장하여 모두 통과했다. 제품 코드 변경 없이 브라우저 테스트만 보강했다.
- `node scripts/build.mjs` 최초 실행은 Vite 개발 서버의 lightningcss 네이티브 모듈 잠금으로 EPERM이었다. 제가 실행한 17441 포트 Vite 프로세스만 종료하고 재실행하여 통과했다. 운영 프로세스나 데이터를 건드리지 않았다.
- `go test ./... -count=1` 4개 패키지 통과(core 83.122s), `go vet ./...` 통과. `node scripts/smoke.mjs dist/modudock-windows-amd64.exe` 실제 HTTP 로그인/CSRF/사용자/API 토큰/내장 UI 통과.
- 프론트 번들은 CSS 47.41kB(gzip 9.68kB), JS 382.25kB(gzip 117.86kB)다. npm ci audit 취약점 0, 고지 277개 섹션에 Better Design MIT 원문 포함을 확인했다.
- 이 작업의 최초 구현 커밋은 1871cb0이다. 실행 중인 운영 인스턴스, Docker 이미지/tar, Linux/macOS 바이너리는 갱신하지 않았다. Windows 검증용 실행파일과 고지만 새로 빌드했고 dist 체크섬은 실제 파일에 맞게 갱신한다.

## 최종 독립 리뷰와 범위 판단

독립 읽기 전용 리뷰 `precision_final_review`는 e627449..1871cb0 제품 코드를 검토해 Critical/Important/Minor 모두 0건으로 판정했다. 공식 토큰과 라이선스 고지 4개 생성물을 직접 확인했고, 전체 테스트는 구현자의 실행 기록을 사용했다. 수정할 발견이나 보류한 사소한 결함은 없다.

| 검토에서 제외한 항목 | 최종 판단과 남는 위험 |
| --- | --- |
| 실제 TOTP/Passkey 장치·세션 보안 | 이번에는 UI만 변경했다. 기존 40개 프론트 테스트·Go 테스트·HTTP 스모크는 통과했으나 실기기 인증의 재검증은 아니다. |
| 브라우저 확인창·날짜 팝업·Passkey 프롬프트 | OS/브라우저 UI는 CSS 적용 범위가 아니다. 앱과 시각적으로 다를 수 있다. |
| 미사용 컴포넌트·미사용 탭 변형·다크 테마 | 승인된 선택 통합과 현재 밝은 UI만 적용했다. 나중에 추가할 때 해당 컴포넌트를 별도로 검증해야 한다. |
| Firefox/Safari·실제 모바일 키보드·확대/고대비 | Chrome 320~1440px만 직접 실행했다. 기기/브라우저별 차이가 남을 수 있다. 강제 색상에서는 native checkbox로 복귀한다. |
| Docker 재빌드·NAS 배포·원격 push | 로컬 변경만 요청받았다. 기존 Docker tar/이미지에는 이 UI가 없으며 새로 빌드해야 한다. |
| 리뷰 시작 후 보강한 테스트 스크립트 | 제품 코드는 동일하다. 추가 스크립트의 계정·스케줄 편집 및 작은 화면 검증은 구현자가 직접 실행해 통과했다. |

기존 작업 브랜치를 유지하고 별도 worktree를 만들지 않았으므로 격리된 신규 체크아웃은 없다. 앱 코드와 별개인 테스트 서버는 종료하고, 이번 작업만의 임시 리뷰 자료는 검증 기록을 이 문서에 옮긴 후 제거한다.
