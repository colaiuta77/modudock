# ModuDock 구현 계획

## 목표

Docker Compose로 단일 서버에 설치할 수 있는 플러그인 기반 자동화 플랫폼의 실행 가능한 MVP를 만든다. API 프로세스는 플러그인 코드를 import하지 않으며, Git에서 가져온 플러그인은 커밋 단위로 검증·고정·활성화·롤백한다.

## 범위

1. FastAPI, SQLAlchemy, Alembic 기반 API와 SQLite 저장소.
2. Redis broker 및 Celery worker.
3. 단일 활성 스케줄러와 실행 이력.
4. JWT 인증, 사용자·역할 기반 접근 제어.
5. 플러그인 manifest 검증, Git 설치, health check, 원자적 활성화, 롤백.
6. 작업 실행, 진행률, 취소 요청, 구조화 로그와 SSE.
7. React, TypeScript, Vite 기반 관리 UI.
8. 샘플 플러그인과 로컬 개발·배포 문서.

## 제외 범위

- 제한된 파일 탐색기.
- 선언형 시스템 명령 실행기.
- 격리된 내장 터미널.
- 임의 JavaScript 플러그인 UI.
- 다중 서버 고가용성.

## 검증

- 백엔드 단위·API 테스트.
- Python 구문 및 정적 검사.
- 프런트엔드 TypeScript 빌드.
- Docker Compose 구성 렌더링.
- 샘플 플러그인 manifest 검증.

## Tabler 디자인 개편

1. 첨부 JSON의 색상·타이포그래피·간격·버튼·카드 토큰을 CSS 변수로 정의한다.
2. Tabler 미리보기의 밝은 관리 콘솔 구조를 ModuDock 사이드바와 콘텐츠 화면에 적용한다.
3. `@tabler/icons-react`를 사용해 메뉴, 설정 카드, 상태 및 주요 동작에 일관된 선형 아이콘을 표시한다.
4. 인증·플러그인·스케줄·계정·로그 기능은 변경하지 않고 반응형 레이아웃과 접근성을 유지한다.
5. 동일한 실제 화면에서 데스크톱·모바일 렌더링과 주요 상호작용을 비교 검증한다.
