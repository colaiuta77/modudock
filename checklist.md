# ModuDock 작업 체크리스트

- [x] 새 Git 저장소 생성.
- [x] 구현 계획과 컨텍스트 기록.
- [x] Docker Compose와 환경 변수 예제 작성.
- [x] FastAPI 애플리케이션과 인증·RBAC 구현.
- [x] SQLAlchemy 모델과 Alembic 마이그레이션 구현.
- [x] 플러그인 manifest와 Git 설치·활성화·롤백 구현.
- [x] Celery 작업·진행률·취소·로그 구현.
- [x] 단일 활성 스케줄러와 실행 이력 구현.
- [x] React 관리 UI 구현.
- [x] 샘플 플러그인 작성.
- [x] 테스트와 빌드 검증.
- [x] README와 보안·후속 범위 문서화.
- [x] 의미 단위 커밋 생성.

## Docker CORS 설정 회귀 수정

- [x] `.env`의 쉼표 구분 CORS 값 파싱 원인 확인.
- [x] pydantic-settings 자동 JSON 디코딩 비활성화.
- [x] 회귀 테스트와 전체 백엔드 검증.
- [x] 버그 수정 커밋 생성.

## 프런트엔드 포트와 로컬 Docker 도구

- [x] Docker 공개 포트를 7438로 변경.
- [x] CORS 기본값과 문서·테스트 동기화.
- [x] Windows에 Docker Desktop 설치.
- [x] Compose 구성과 전체 회귀 검증.
- [x] 변경 커밋 생성.

## 백엔드 동시 시작 마이그레이션 경쟁 수정

- [x] worker·scheduler 종료 로그의 실제 예외 확인.
- [x] Alembic bootstrap을 API 단일 책임으로 변경.
- [x] worker·scheduler가 API health 이후 시작되도록 변경.
- [x] 빈 볼륨에서 Compose 전체 기동 검증.
- [x] 백엔드 회귀 테스트와 수정 커밋 생성.
