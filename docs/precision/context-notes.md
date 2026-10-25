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
