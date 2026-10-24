# ModuDock Go 구현 체크리스트

- [x] 신규 디렉터리와 독립 Git 저장소 생성.
- [x] 승인된 설계 보관.
- [x] 구현 계획과 API·플러그인 계약 고정.
- [x] Go 코어·SQLite·인증·권한·외부 API 구현과 검증.
- [x] 영속 작업 큐·스케줄·로그·플러그인 구현과 검증.
- [x] Vite 8·Tailwind·shadcn/ui·Lucide 관리 콘솔 구현과 검증.
- [x] Docker·Linux 네이티브 배포와 예제 플러그인·릴리스 자동화.
- [x] 통합 테스트·독립 검토·로컬 커밋.

GitHub push·NAS 업로드·운영 데이터 이관은 수행하지 않는다.

## 최종 리뷰 수정

- [x] 2020-12/local JSON Pointer 경계의 RED→GREEN과 기존 secret 회귀 확인.
- [x] disable 후 실패는 failed, active 실패는 retry하는 실제 실행 회귀 확인.
- [x] tasks 생략·빈 목록·혼합 목록 스케줄 UI의 RED→GREEN 확인.
- [x] 저장된 required secret 비우기와 신규 required secret 검증.
- [x] 전체 Go test/vet, frontend test/build, diff check 및 scoped commit·보고서.
