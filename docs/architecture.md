# ModuDock 아키텍처

```text
Browser
  └─ FastAPI + React static UI ── SQLite volume
       ├─ declared resource subprocess ── plugin data volume
       └─ Redis broker/AOF
            ├─ Celery worker ── plugin release volume
            └─ Scheduler ── SQLite lease
```

## 플러그인 설치 상태 전이

1. 관리자가 HTTPS 또는 SSH Git URL과 ref를 제출한다.
2. worker가 임시 디렉터리에 clone하고 detached commit SHA를 구한다.
3. `modudock.yaml`을 검증하고 commit별 release 디렉터리에 복사한다.
4. release 전용 Python venv를 만들고 선택적으로 의존성을 설치한다.
5. 선언된 health check를 argv 방식으로 실행한다.
6. 짧은 DB transaction에서 release를 healthy로 기록하고 active pointer를 교체한다.

활성 release pointer 교체 전 실패는 기존 활성 release에 영향을 주지 않습니다. 롤백은 healthy release의 pointer를 다시 선택하는 작업입니다.

## 컨테이너 시작 순서

1. Redis가 health check를 통과한다.
2. API가 Alembic 마이그레이션과 최초 관리자 생성을 단독으로 수행한다.
3. API health check가 성공한 뒤 worker와 scheduler가 시작된다.

SQLite에 대한 동시 DDL 경쟁을 막기 위해 worker와 scheduler는 데이터베이스 bootstrap을 실행하지 않습니다.
React 정적 파일은 API 이미지의 `/app/static`에 포함되며 FastAPI가 같은 포트에서 제공합니다.

## 플러그인 리소스

관리자 또는 Android 요청은 임의 명령을 지정할 수 없습니다. API는 활성 manifest에 선언된 리소스 ID와 command만 찾아 별도 subprocess로 실행합니다. 관리자 리소스는 operator 이상 역할을 요구하고, 공개 리소스는 manifest의 `public_methods`에 선언된 GET 요청만 허용합니다.

리소스 subprocess에는 플랫폼 비밀 환경 변수를 상속하지 않고 플러그인 ID, 전용 데이터 디렉터리와 해당 플러그인 설정만 전달합니다. 응답이 선언된 후속 Celery 작업을 요청하면 API가 작업 이력과 공통 로그를 생성한 뒤 worker에 발행합니다.

이 구조는 플러그인을 API 프로세스에 import하지 않지만 프로세스별 Linux namespace까지 제공하는 완전한 sandbox는 아닙니다.

## 후속 격리 모델

신뢰할 수 없는 플러그인과 터미널을 지원할 때 worker가 직접 subprocess를 실행하는 구조를 runner agent 호출 구조로 교체합니다. runner는 작업별 uid, 읽기 전용 root filesystem, 허용 volume, seccomp, CPU·메모리·시간 제한, 기본 네트워크 차단을 적용해야 합니다.
