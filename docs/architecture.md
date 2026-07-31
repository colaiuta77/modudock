# ModuDock 아키텍처

```text
Browser
  └─ Web UI
      └─ FastAPI ── SQLite volume
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

## 후속 격리 모델

신뢰할 수 없는 플러그인과 터미널을 지원할 때 worker가 직접 subprocess를 실행하는 구조를 runner agent 호출 구조로 교체합니다. runner는 작업별 uid, 읽기 전용 root filesystem, 허용 volume, seccomp, CPU·메모리·시간 제한, 기본 네트워크 차단을 적용해야 합니다.
