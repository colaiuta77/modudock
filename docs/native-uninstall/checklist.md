# 제거 기능 체크리스트

- [x] 현재 설치기의 Linux·DSM 경로, 잠금, unit 처리 확인.
- [x] 삭제/보존 범위와 레거시 unit 설정 백업 결정 기록.
- [x] 제거 동작·실패 시 보존 테스트 RED. 기존 main이 uninstall을 거부하는 실패 확인.
- [x] 최소 uninstall 구현과 설치 문서·CI 연결.
- [x] 전체 Bash 회귀 테스트와 독립 검토 통과. e359cef 기준 남은 Critical/Important 없음.
- [x] 커밋 및 사용자 승인 범위의 게시. main 2ab2f8d, CI 40286372148 전체 성공.
