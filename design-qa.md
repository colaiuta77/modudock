# ModuDock Tabler 디자인 QA

## 비교 대상

- Source visual truth path: `C:\Users\JSH\Documents\Codex\2026-07-31\plugin-automation-platform\work\design-qa\tabler-reference.png`.
- Implementation screenshot path: `C:\Users\JSH\Documents\Codex\2026-07-31\plugin-automation-platform\work\design-qa\modudock-final-desktop.png`.
- 모바일 구현 화면: `C:\Users\JSH\Documents\Codex\2026-07-31\plugin-automation-platform\work\design-qa\modudock-mobile-settings-final.png`.
- Source pixels: 1265 × 710.
- Implementation pixels: 1280 × 720.
- CSS viewport: 기본 데스크톱 1280 × 720, 모바일 390 × 844.
- Density normalization: 두 데스크톱 화면 모두 브라우저 1x 캡처이며 약 1% 이내의 크기 차이로 직접 비교했다.
- State: 밝은 테마의 로그인된 관리자 대시보드. Tabler 기준은 데이터가 채워진 예시이고 ModuDock은 실제 빈 작업 상태다.

## Full-view comparison evidence

- 두 화면 모두 옅은 회색 앱 배경, 흰색 내비게이션과 카드, 얇은 회색 경계, 파란 활성 상태를 사용한다.
- ModuDock은 기존 설정 계층을 유지하기 위해 Tabler의 상단 내비게이션 대신 좌측 사이드바를 유지했다. 콘텐츠의 카드 밀도, 제목 계층, 6~8px 반경과 그림자 강도는 Tabler 기준에 맞췄다.
- 첨부 JSON의 `#066fd1`, `#f9fafb`, `#374151`, `#e5e7eb` 토큰이 버튼, 배경, 텍스트와 경계에 일관되게 적용됐다.
- Tabler Icons의 24px 그리드 기반 선형 아이콘을 브랜드, 메뉴, 설정 카드, 상태와 주요 동작에 사용했다.

## Focused region comparison evidence

- 내비게이션과 통계 카드가 1x 전체 화면에서도 아이콘 선 두께, 레이블 크기, 카드 패딩과 활성 상태를 판독할 수 있어 별도 확대 비교는 필요하지 않았다.
- 설정 화면과 390px 모바일 화면을 추가 캡처해 설정 카드 아이콘, 긴 한글 설명 줄바꿈, 가로 내비게이션과 화면 폭을 별도로 확인했다.

## Required fidelity surfaces

- Fonts and typography: Inter Var 우선순위와 시스템 한글 폴백을 사용한다. H1은 제공 규격인 32px/700/38px이며 본문은 14px/20px다. 작은 UI 레이블은 Tabler처럼 10~12px의 중간 굵기를 사용한다.
- Spacing and layout rhythm: 4, 8, 16, 20, 32px 계열 간격, 8px 카드 반경, 1px 경계와 작은 그림자를 일관되게 사용한다.
- Colors and visual tokens: 제공된 강조색·배경·텍스트 값을 그대로 사용하고 성공·경고·오류는 저채도 배경과 충분한 대비를 갖춘 의미색으로 분리했다.
- Image quality and asset fidelity: 대시보드에 래스터 이미지가 필요하지 않다. 모든 UI 아이콘은 사용자가 지정한 공식 `@tabler/icons-react` 벡터 자산을 사용하며 대체 문자나 임의 SVG가 없다.
- Copy and content: 기존 ModuDock의 한글 메뉴와 기능 설명을 유지하고 영문은 짧은 보조 레이블에만 사용했다.

## Findings

- P0/P1/P2 잔여 항목이 없다.
- P3로 데이터가 없는 개요의 최근 작업 패널이 넓게 비어 보이지만 실제 빈 상태를 정확히 표현하며 핵심 사용을 방해하지 않는다.

## Comparison history

1. 첫 모바일 비교에서 390px viewport에 문서 폭이 957px까지 확장되는 P2 가로 넘침을 확인했다.
2. 모바일 aside와 nav에 `min-width: 0`, `max-width: 100%`, 내부 가로 스크롤 경계를 적용했다.
3. 수정 후 390px viewport에서 문서 scroll width가 375px로 줄어 영구 UI가 화면 밖으로 밀리지 않음을 확인했다.
4. 모바일 내비게이션 스크롤바를 숨기되 터치·휠 가로 탐색은 유지했다.

## Primary interactions and runtime checks

- 로그인된 개요 화면 로드.
- 설정 메뉴 열기와 설정 카드 표시.
- Tabler Core CSS 적용 후 사용자 추가 폼과 계정 관리 표 표시.
- 데스크톱 새로고침 동작.
- 390px 모바일 개요·설정 반응형 전환.
- 브라우저 콘솔 오류 0건.

## Implementation Checklist

- [x] 첨부 테마 토큰 적용.
- [x] Tabler Icons 공식 React 패키지 적용.
- [x] 데스크톱 대시보드와 설정 화면 확인.
- [x] 모바일 overflow 수정과 재확인.
- [x] 주요 내비게이션과 콘솔 오류 확인.
- [x] Tabler Core CSS 기반 계층 적용 후 데스크톱·모바일 회귀 확인.
- [x] 사이드바 상위 메뉴 왼쪽 정렬과 하위 메뉴 16px 들여쓰기 확인.
- [x] 데스크톱·390px 모바일 전체 로그에서 플랫폼 컴포넌트 출처 확인.

final result: passed
