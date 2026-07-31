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

## Web·API 컨테이너 통합

- [x] React를 백엔드 Docker 이미지의 다단계 빌드로 통합.
- [x] FastAPI 정적 파일 제공과 동일 origin API 호출 적용.
- [x] Compose의 별도 web 서비스 제거.
- [x] 백엔드·프런트엔드 및 실제 Compose 기동 검증.
- [x] 문서 갱신과 통합 커밋 생성.

## 설정 관리 콘솔

- [x] 설정 상위 메뉴와 권한별 하위 메뉴 구성.
- [x] 플러그인과 스케줄을 설정 하위로 이동.
- [x] 관리자·일반 계정의 추가, 변경, 비활성화, 비밀번호 재설정 구현.
- [x] 현재 계정 비밀번호 변경 구현.
- [x] 플랫폼·플러그인·작업 통합 로그 조회와 필터 UI 구현.
- [x] 스케줄 생성, 변경, 삭제 UI 구현.
- [x] 플러그인 메뉴 이름, 순서, 노출 여부 편집 구현.
- [x] 백엔드 테스트, 프런트 빌드, Compose 및 브라우저 검증.
- [x] 의미 단위 커밋 생성.

## Tabler 디자인 시스템 개편

- [x] 첨부 테마 JSON과 Tabler 미리보기 기준 화면 분석.
- [x] Tabler Icons React 의존성과 공통 아이콘 컴포넌트 추가.
- [x] 내비게이션과 설정 카드에 의미별 아이콘 적용.
- [x] 밝은 배경, 파란 강조색, Inter 타이포그래피와 8px 카드 체계 적용.
- [x] 표, 폼, 버튼, 알림, 로그 뷰와 로그인 화면 일관성 정리.
- [x] 데스크톱과 모바일 반응형 화면 검증.
- [x] 프런트 빌드와 Compose 통합 검증.
- [x] 디자인 QA 기록.
- [x] 의미 단위 커밋 생성.

## Tabler Core 확장 기반

- [x] Tabler 본체 MIT 라이선스와 고지 의무 확인.
- [x] 제3자 라이선스 고지 문서 추가.
- [x] 공식 `@tabler/core` 의존성과 CSS 기반 계층 추가.
- [x] ModuDock 토큰을 Tabler 변수와 호환되게 연결.
- [x] Tabler JavaScript를 제외하고 React 상태 소유권 유지.
- [x] 프런트 빌드와 Compose 통합 검증.
- [x] 브라우저 스타일 충돌과 모바일 회귀 확인.
- [x] 의미 단위 커밋 생성.
