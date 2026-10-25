# Precision UI와 컴포넌트 추가

ModuDock-Go는 [Better Design Precision](https://better-design.com/design-systems/precision)의 밝은 테마를 사용한다. React/Vite/Tailwind/shadcn 구조와 Lucide 아이콘은 그대로다. 별도 디자인 서버나 MCP가 실행 중일 필요가 없으며, UI 사용 시 외부 스타일·폰트를 요청하지 않는다.

## 현재 적용 범위

- `frontend/src/precision.css` — 공식 색상, 반경, 그림자, 전환 토큰과 앱 상태 색상.
- `frontend/src/components/ui/` — Button, Card, Badge, Input, Textarea, Tabs, Sheet. `_shared.ts`는 입력 컴포넌트 공통 스타일.
- `frontend/src/styles.css` — 사이드바, 대시보드, 폼, 표, 상태/로그, 로그인/MFA/복구 코드의 앱 레이아웃과 토큰 사용.
- 네이티브 select/checkbox/table는 기존 폼 계약을 보존하면서 같은 디자인을 사용한다.
- 전체 레지스트리의 미사용 AI/차트/애니메이션 컴포넌트는 포함하지 않았다. 런타임 의존성 추가 없음.

## 나중에 컴포넌트 추가

프로젝트의 `frontend` 폴더에서 공식 shadcn 레지스트리 명령을 실행한다. 예를 들어 tooltip을 추가한다.

```sh
cd frontend
npx shadcn@latest add https://better-design.com/registry/precision/tooltip.json
```

`tooltip` 대신 필요한 컴포넌트 이름을 지정한다. 공식 목록은 [저장소](https://github.com/marvkr/better-design)와 디자인 시스템 페이지를 참고한다. 이 명령은 개발 환경에서 소스를 추가하는 명령이며, 운영 서버에서 실행할 필요는 없다.

주의할 점은 다음과 같다.

1. 먼저 변경사항을 커밋한다. `all.json`, `-o`/`--overwrite`, 자동 확인 `-y`는 사용하지 않는다.
2. 기존 `button.tsx`, `_shared.ts`, 전역 CSS 등을 덮어쓸지 물으면 거절하고 diff를 확인한다. 이 프로젝트에는 React 19/Radix 통합, 한국어 접근성 이름, 모바일 크기와 기존 props 계약에 맞춘 조정이 있다.
3. 설치기가 추가한 색상 변수가 `styles.css`에서 `precision.css`를 덮어쓰지 않는지 확인한다. 공통 토큰의 기준은 `precision.css` 하나다.
4. 의존성을 검토한다. 현재 `radix-ui` 통합 패키지를 사용하므로 개별 `@radix-ui/*` import는 기존 패턴으로 맞출 수 있다. 일부 컴포넌트는 `motion` 등 추가 패키지가 필요하므로 실제 필요한 것만 추가한다.
5. 컴포넌트를 실제 화면에 연결하고 키보드·모바일·disabled 상태를 확인한다. `npm test`와 `npm run build`를 실행한다.
6. 저장소 루트에서 기존 네이티브/Docker 빌드를 다시 수행한다. 컴포넌트 추가는 DB 마이그레이션이나 운영 데이터 초기화를 요구하지 않는다.

현재 작업은 CLI로 전체 파일을 덮어쓰지 않고 공식 토큰/스타일을 선택적으로 병합했다. 기존 컴포넌트 API와 인증 동작을 보존하기 위한 통합이다. 애니메이션 라이브러리 대신 CSS 전환과 reduced-motion을 유지한다. 한국어 메뉴를 위해 사이드바는 224px이며 공식 쇼케이스 폭을 강제로 사용하지 않는다.

## 검증 실행

```sh
npm --prefix frontend test
npm --prefix frontend run build
npm --prefix frontend run dev -- --port 17441 --strictPort
```

별도 터미널에서 Chrome과 Playwright가 설치된 개발 환경으로 브라우저 검증을 실행한다.

```sh
node scripts/precision-check.mjs
```

Playwright가 별도의 도구 디렉터리에 설치되어 있으면 그 패키지의 `index.mjs` 경로를 첫 번째 인자로 전달할 수 있다. 런타임 의존성에 Playwright를 넣지 않는다.

```powershell
node scripts/precision-check.mjs .tools/browser-test/node_modules/playwright/index.mjs
```

검증은 API를 가짜 응답으로 격리하고 실제 렌더링·입력·탭·모바일 메뉴·MFA 화면을 검사한다. 운영 서버나 실제 사용자 데이터에 연결하지 않는다. 스크린샷은 `.tools/precision-screens/`에 생성되며, 이 테스트만으로 서버 인증이나 실제 인증 장치 동작을 검증했다고 볼 수 없다. 실제 API 스모크는 기존 `scripts/smoke.mjs`를 사용한다.

## 출처와 라이선스

2026-10-25 공식 `registry/precision`의 자료를 기준으로 적용했다. 현재 레지스트리는 인디고 강조색을 사용하며, GitHub README의 예전 charcoal 설명과 차이가 있다. MIT 원문은 `docs/better-design-license.txt`에 보관하고, `scripts/notices.mjs`가 `/third-party-notices.txt`와 배포 파일에 포함한다.

프론트 소스만 변경한 작업이므로 기존 Docker 이미지 tar에는 이 디자인이 들어 있지 않다. 새 디자인을 배포하려면 새 이미지를 빌드해야 한다.
