# ModuDock

ModuDock은 Git 저장소에서 플러그인을 설치하고 작업과 스케줄을 운영하는 Docker 기반 자동화 플랫폼입니다. API 프로세스는 플러그인 코드를 직접 import하거나 실행하지 않습니다.

## 현재 제공 기능

- JWT 인증과 `admin`, `operator`, `viewer` 역할.
- Git HTTPS·SSH 저장소 clone, manifest 검증, commit 고정, 전용 venv 구성, health check, 원자적 활성화.
- 기존 healthy release 재활성화를 통한 롤백.
- Celery 작업 실행, 진행률, 협력형 취소 요청, 구조화 로그, SSE 상태 스트림.
- SQLite 기반 스케줄 CRUD와 DB lease 단일 활성 스케줄러.
- JSON Schema 기반 작업 입력 폼과 플러그인 설정 검증.
- 설정 하위의 플러그인·스케줄·사용자 계정·전체 로그·플러그인 메뉴 관리.
- 관리자와 일반 계정 관리, 계정 비활성화, 관리자 재설정과 본인 비밀번호 변경.
- manifest 메뉴의 표시 이름, 노출 여부, 사이드바 순서 편집.
- 첨부 테마 토큰과 Tabler UI를 반영한 밝은 관리 콘솔, 공식 Tabler Icons 기반 내비게이션.
- FastAPI 이미지에 통합된 React 관리 콘솔과 샘플 플러그인.

## UI 기반

관리 콘솔은 공식 `@tabler/core` CSS를 기반 계층으로 사용하고, 제품별 디자인 토큰과 레이아웃은 `styles.css`에서 확장합니다. 내비게이션과 폼 상태는 React가 관리하므로 Bootstrap·Tabler JavaScript는 로드하지 않습니다. 아이콘은 `@tabler/icons-react`를 사용합니다.

Tabler 관련 패키지는 MIT 라이선스입니다. 배포 시 포함해야 하는 저작권과 라이선스 문구는 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)에 기록했습니다.

## 빠른 시작

필요 조건은 Docker Engine과 Docker Compose입니다.

```bash
cp .env.example .env
```

`.env`의 `MODUDOCK_SECRET_KEY`와 `MODUDOCK_ADMIN_PASSWORD`를 반드시 변경한 뒤 실행합니다.

```bash
docker compose up --build
```

- 관리 콘솔은 `http://localhost:7438`에서 엽니다.
- API 문서는 `http://localhost:7438/docs`에서 확인합니다.
- 초기 계정 이름은 `.env`의 `MODUDOCK_ADMIN_USERNAME` 값입니다.

## 샘플 플러그인 사용

`samples/hello-plugin`은 독립 Git 저장소로 옮겨 HTTPS 또는 SSH로 접근 가능한 원격 저장소에 push한 뒤 관리 콘솔에서 설치할 수 있습니다. ModuDock은 로컬 경로와 `file://`, Git `ext::` transport를 허용하지 않습니다.

플러그인은 저장소 루트에 `modudock.yaml`을 포함해야 합니다.

```yaml
schema_version: 1
id: hello-world
name: Hello World
version: 0.1.0
tasks:
  - id: greet
    title: 인사 메시지 만들기
    command: [python, task.py]
```

작업은 JSON을 표준 입력으로 받고 JSON 객체를 표준 출력으로 반환해야 합니다. 명령은 shell 문자열이 아닌 argv 배열로 선언합니다.

## 로컬 백엔드 개발

Python 3.14와 Redis가 필요합니다.

```bash
cd backend
python -m venv .venv
.venv/Scripts/pip install -r requirements-dev.txt
python -m app.bootstrap
uvicorn app.main:app --reload
```

별도 터미널에서 worker와 scheduler를 실행합니다.

```bash
celery -A app.worker.celery_app worker --loglevel=INFO
python -m app.scheduler
```

프런트엔드는 Node.js 24 LTS에서 실행합니다.

```bash
cd frontend
npm install
npm run dev
```

## API 개요

- `POST /api/auth/token`.
- `GET /api/auth/me`, `PUT /api/auth/password`.
- `GET`, `POST /api/users`.
- `PUT`, `DELETE /api/users/{id}`, `PUT /api/users/{id}/password`.
- `GET /api/plugins`.
- `POST /api/plugins/install`.
- `GET /api/plugin-menus`, `PUT /api/plugin-menus/{plugin_id}/{menu_id}`.
- `GET /api/plugins/{id}/manifest`.
- `GET`, `PUT /api/plugins/{id}/settings`.
- `GET /api/plugins/{id}/releases`.
- `POST /api/plugins/{id}/releases/{release_id}/activate`.
- `POST /api/plugins/{id}/tasks/{task}/runs`.
- `GET /api/runs`.
- `POST /api/runs/{id}/cancel`.
- `GET /api/runs/{id}/events`.
- `GET`, `POST`, `PUT`, `DELETE /api/schedules`.
- `GET /api/logs`.

## 보안 경계와 알려진 제한

- API 컨테이너에는 Docker socket이나 호스트 루트를 마운트하지 않습니다.
- 플러그인은 별도 worker 컨테이너의 subprocess에서 실행되지만, MVP에서는 작업별 Linux namespace, seccomp, 네트워크 정책까지 적용하지 않았습니다. 신뢰할 수 없는 플러그인의 완전한 샌드박스는 후속 runner agent로 분리해야 합니다.
- 실행 중 취소는 프로세스 강제 종료가 아닌 협력형 요청입니다. 명령이 반환되면 결과를 폐기하고 `cancelled`로 확정합니다.
- 설치 시 `requirements.txt`를 전용 venv에 설치하지만, task command가 해당 venv 실행 파일을 사용하려면 manifest에 명시해야 합니다.
- SQLite 단일 서버를 전제로 합니다. WAL, 5초 busy timeout, 짧은 transaction을 적용했습니다.
- 파일 탐색기, 선언형 시스템 명령, 내장 터미널은 아직 포함하지 않았습니다. 이 기능은 별도 agent, 허용 경로, 재인증, 감사 로그를 전제로 추가합니다.

## 데이터와 로그

SQLite DB는 `modudock-data`, Redis AOF는 `modudock-redis`, 플러그인 release는 `modudock-plugins` Docker volume에 보존됩니다. 모든 저장 로그는 `plugin_id`, `task_id`, `run_id`, `user_id`, `timestamp` 공통 필드를 가집니다.

전체 로그에는 사용자 감사 이벤트와 플러그인·작업 로그뿐 아니라 ModuDock API 초기화, worker 준비·종료, scheduler 시작·오류와 처리되지 않은 API 예외도 기록됩니다. 화면에서는 `MODUDOCK/API`, `MODUDOCK/WORKER`, `MODUDOCK/SCHEDULER` 출처로 구분합니다.

Docker daemon의 원시 stdout·stderr 전체를 화면에서 직접 읽지는 않습니다. 웹/API 컨테이너에 Docker socket을 노출하지 않는 보안 경계를 유지하기 위한 결정이며, 원시 컨테이너 로그는 운영 호스트의 `docker compose logs`로 확인합니다.
