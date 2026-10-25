# Precision UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 기존 기능을 유지하면서 모든 ModuDock-Go 화면을 Precision 스타일로 통일한다.
**Architecture:** 공식 Precision 토큰과 사용 중인 7개 UI 컴포넌트 스타일을 기존 계약에 통합한다. 화면 CSS는 동일 토큰을 소비하며 추가 런타임 의존성 없이 확장 문서와 MIT 고지를 제공한다.
**Tech Stack:** React 19, Vite 8, Tailwind 4, Radix, Lucide, Go 임베디드 UI.
**Spec:** `docs/precision/design.md`

## Global Constraints
- Go API/DB/인증/플러그인 동작을 변경하지 않는다.
- 기존 컴포넌트 export/props/data-slot과 폼 제출 계약을 보존한다.
- 전체 all.json 설치, 미사용 컴포넌트, 원격 폰트, 전역 Codex MCP 설정 변경을 하지 않는다.
- 한글, 모바일, reduced-motion, 키보드 포커스, QR 흰 여백을 보존한다.

## Review Focus
- 글로벌 CSS가 위험 버튼·disabled·포커스 스타일을 덮어쓰지 않는지 브라우저에서 확인.
- 모바일 서랍과 긴 폼/표/복구 코드가 뷰포트 전체를 넘치게 하지 않는지 확인.
- 모든 메뉴의 기존 색상 잔재 및 실제 사용 컴포넌트가 토큰을 따르는지 확인.
- 로그인·MFA·복구 코드의 제출/재인증 흐름을 기존 테스트와 실제 브라우저로 확인.
- 배포된 고지에 Better Design MIT 원문이 포함되는지 빌드 결과 확인.

### Task 1: 공통 컴포넌트와 전체 화면 Precision 적용
**Files:** `frontend/src/precision.css` (new), `frontend/src/styles.css`, `frontend/src/components/ui/{button,card,badge,input,textarea,tabs,sheet}.tsx`, `frontend/src/App.tsx` (선택 메뉴 접근성).
**Interfaces:** 기존 export/props 유지. precision.css 토큰을 styles.css와 공통 UI가 소비한다.
- [ ] 브라우저 테스트를 먼저 작성해 배경/primary/모서리/포커스/반응형 레이아웃 계약을 확인하고 변경 전 실패를 관찰한다.
- [ ] 공식 소스의 토큰·컴포넌트 스타일을 적용하고 전체 화면의 하드코딩 색상/그림자/간격을 정리한다.
- [ ] `npm.cmd --prefix frontend test` 및 `npm.cmd --prefix frontend run build` 통과, 브라우저 데스크톱/모바일 검증 통과.
- [ ] `feat(ui): apply Precision design system` 커밋.

### Task 2: 확장 방법·라이선스·최종 검증
**Files:** `docs/precision/README.md`, `docs/better-design-license.txt`, `scripts/notices.mjs`, `docs/precision/checklist.md`, `docs/precision/context-notes.md`.
**Interfaces:** Task 1의 공통 토큰/컴포넌트 경로와 기존 notices 빌드 파이프라인 사용.
- [ ] 공식 개별 컴포넌트 추가 명령과 덮어쓰기 주의, 버전 출처와 통합 변경사항 문서화.
- [ ] MIT 원문을 고지 생성기에 연결하고 실제 생성물에서 확인.
- [ ] 프론트 전체 테스트/빌드, `node scripts/build.mjs`, 임베디드 스모크 및 Go 테스트 확인.
- [ ] 최종 독립 리뷰 1회 후 중요한 발견만 회귀 검증하고 결과 문서/커밋.
