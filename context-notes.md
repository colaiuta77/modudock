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
