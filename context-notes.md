# ModuDock 컨텍스트 노트

## 2026-07-31

- 기존 프로젝트와 분리하기 위해 현재 작업 폴더 아래 `ModuDock` 독립 Git 저장소를 생성했다.
- API는 플러그인 코드를 import하거나 실행하지 않는다. 설치 파이프라인은 별도 Celery worker에서 수행한다.
- SQLite는 WAL, busy timeout, 짧은 트랜잭션을 적용하고 데이터 볼륨에 저장한다.
- 스케줄러는 DB lease를 획득한 단일 인스턴스만 due schedule을 발행하는 구조로 설계한다.
- 초기 플러그인 UI는 manifest의 JSON Schema와 작업 선언을 렌더링한다.
- 파일 탐색기와 터미널은 MVP에서 제외한다. 추후 별도 agent, 허용 경로, 재인증, 감사 로그를 필수 전제로 한다.
- 로컬에서 바로 평가할 수 있도록 개발 모드도 제공하되 공식 실행 경로는 Docker Compose로 둔다.
- Git transport는 command helper 악용을 막기 위해 HTTPS와 SSH만 허용한다.
- MVP 취소는 협력형으로 구현한다. 실행 중 프로세스가 반환된 뒤 취소 요청을 확인하고 결과를 폐기한다.
- plugin worker는 API와 분리했지만 작업별 커널 수준 sandbox는 아직 적용하지 않았다. 후속 runner agent의 명시적 보안 요구사항으로 문서화했다.
- `pydantic-settings`는 복합 타입 환경 변수를 validator 실행 전에 JSON으로 디코딩한다. 문서화된 쉼표 구분 CORS 값을 유지하기 위해 해당 필드에 `NoDecode`를 적용하고 validator가 직접 분리하도록 했다.
- Docker로 공개하는 관리 콘솔 포트는 사용자 요청에 따라 8080에서 7438로 변경했다. Vite 로컬 개발 포트 5173은 유지한다.
- 로컬 검증을 위해 Docker Desktop 4.84.0, Docker CLI·Engine 29.6.2, Compose 5.3.1을 설치했다. 구형 inbox WSL로 backend가 시작되지 않아 Store 기반 WSL 2.7.11.0도 설치했다.
- API·worker·scheduler가 동시에 Alembic을 실행하면 SQLite DDL 경쟁이 발생한다. 마이그레이션과 관리자 bootstrap은 단일 API 컨테이너만 담당하고 worker·scheduler는 API health 이후 시작하도록 책임을 분리했다.
- 실제 빈 Docker 볼륨 기동에서 API·Redis healthy, worker Celery ready, scheduler running, API·웹 HTTP 200을 확인했다. Celery root 실행 경고는 후속 컨테이너 권한 강화 항목으로 남긴다.
- 사용자 요청에 따라 별도 Nginx web 컨테이너를 제거했다. React는 Node 빌드 stage에서 생성해 API 이미지에 포함하고 FastAPI가 7438 포트에서 UI와 API를 함께 제공한다.
- 통합 이미지의 실제 빈 볼륨 기동에서 네 컨테이너만 생성됨을 확인했다. 7438 포트의 UI·health·API 문서가 모두 HTTP 200이었고 worker ready와 scheduler running도 확인했다.
- 설정 내비게이션은 FlaskFarm처럼 사이드바의 설정 아래 플러그인, 스케줄, 사용자 계정, 메뉴 편집, 전체 로그, 비밀번호 변경을 배치했다. 일반 계정에는 전체 로그와 본인 비밀번호 변경만 노출한다.
- 계정 삭제는 작업·로그의 사용자 참조를 보존하기 위해 실제 행 삭제 대신 즉시 로그인 차단되는 비활성화로 처리한다. 마지막 활성 관리자와 현재 로그인한 계정은 삭제할 수 없다.
- 플러그인 메뉴 설정은 manifest 원본을 수정하지 않고 `plugin_menu_preferences`에 표시 이름, 노출 여부, 순서를 덮어쓴다. 플러그인 업데이트와 롤백 이후에도 사용자 편집값을 유지하기 위한 결정이다.
- 전체 로그 화면은 DB에 저장된 플랫폼·플러그인·작업 로그를 한 스트림으로 보여주며 레벨, 메시지 검색, 5초 자동 갱신을 제공한다.
- 새 설정 기능은 빈 Docker 볼륨에서 Alembic 0002 적용, API·Redis healthy, worker ready, scheduler running을 확인했다. 관리자 로그인 후 설정, 사용자 관리, 스케줄, 전체 로그 화면을 실제 브라우저에서 확인했고 콘솔 오류는 없었다.
- 사용자가 제공한 테마 JSON을 새 UI의 기준 토큰으로 확정했다. 강조색은 `#066fd1`, 앱 배경은 `#f9fafb`, 기본 텍스트는 `#374151`, 기본 카드 테두리는 `#e5e7eb`, 기본 반경은 8px로 사용한다.
- Tabler 미리보기에서 흰색 내비게이션, 옅은 회색 앱 배경, 1px 경계 카드, 조밀한 정보 계층, 파란 활성 상태와 24px 선형 아이콘을 기준 패턴으로 채택했다.
- 아이콘은 사용자가 지정한 Tabler Icons의 공식 React 패키지를 사용한다. 임의 SVG나 텍스트 기호를 만들지 않고 메뉴와 설정 기능의 의미에 맞는 아이콘을 직접 import한다.
- 기존 사이드바 구조는 설정 하위 메뉴와 동적 플러그인 메뉴를 한눈에 보여주는 ModuDock의 핵심 정보 구조이므로 유지했다. Tabler의 상단 내비게이션을 그대로 복제하지 않고 밝은 표면, 파란 활성 상태, 아이콘과 카드 밀도만 제품 구조에 맞게 이식했다.
- 첫 모바일 렌더링에서 펼쳐진 설정 하위 메뉴가 aside의 intrinsic width를 키워 957px 가로 넘침을 만들었다. 모바일 aside와 nav의 최소 폭을 0으로 제한하고 nav 내부에서만 가로 스크롤하도록 수정해 390px viewport에서 문서 폭 375px을 확인했다.
- 디자인 QA는 Tabler 1265×710 기준 캡처와 ModuDock 1280×720 실제 캡처를 같은 비교 입력으로 확인했다. 콘솔 오류 없이 데스크톱·모바일 주요 상태를 확인했으며 최종 결과는 passed다.
- Tabler 본체는 MIT 라이선스이며 저작권과 라이선스 문구를 배포물에 포함하는 조건으로 사용·수정·재배포할 수 있다.
- Tabler Core는 Bootstrap 기반 CSS와 JavaScript를 함께 제공하지만 ModuDock은 React가 상태를 관리한다. 향후 확장을 위해 공식 CSS만 기반 계층으로 사용하고 Bootstrap의 imperative JavaScript는 로드하지 않는다.
- 전역 충돌을 줄이기 위해 import 순서는 Tabler Core CSS 다음에 ModuDock `styles.css`로 고정한다. 현재 화면은 ModuDock 오버라이드가 유지되고 새 화면은 Tabler 컴포넌트 클래스를 점진적으로 사용할 수 있다.
- `@tabler/core` 1.4.0 적용 후 프런트 빌드와 Compose API 이미지 빌드가 통과했다. 로그인된 설정·사용자 계정 화면과 390×844 설정 화면을 확인했으며 모바일 문서 가로 넘침은 없었다.
