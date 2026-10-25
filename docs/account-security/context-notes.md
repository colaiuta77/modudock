# 계정 보안 보강 결정 기록

## 2026-10-25 요청과 작업 경계

- 사용자는 계정별 TOTP MFA, 복구 코드, Passkey 비밀번호 없는 로그인을 로컬에 구현하도록 요청했다. TOTP는 첨부 화면처럼 QR 스캔과 수동 키 등록을 지원한다.
- 대상은 `D:/Documents/ChatGPT/ModuDock-Go`이다. 기존 `Modudock`은 수정하지 않는다.
- 원격 푸시, NAS 업로드, 운영 계정·DB 변경은 범위가 아니다.
- 시작 시 `git status --short`는 비어 있고 브랜치는 `codex/go-platform`이었다.
- 처음에는 브레인스토밍 지침의 설계 승인을 기다렸으며, 이후 사용자의 진행 요청으로 승인을 받았다. 제품 구현 전 상세 계획과 실행 방식을 확정한다.

## 코드에서 확인한 연결 지점

- `internal/core/auth.go`의 `login`은 비밀번호 확인 후 즉시 24시간 세션을 만든다. 발급 경계를 MFA와 Passkey 성공 경로에서 공유해야 한다.
- 기존 Argon2id, 난수, 토큰 해시, CSRF/Origin 검사와 계정 폐기 규칙은 유지한다.
- `internal/core/db.go`는 스키마 버전 2까지 지원하므로 추가 마이그레이션이 필요하다.
- `internal/core/server.go`의 인증은 Bearer 토큰도 허용한다. 새 계정 보안 API는 세션과 재인증을 명시적으로 요구해야 한다.
- `frontend/src/api.ts`는 로그인 결과를 즉시 사용자로 해석한다. MFA 대기 응답을 구분해야 한다.
- `frontend/src/Settings.tsx`의 비밀번호 화면과 `frontend/src/App.tsx`의 로그인·메뉴가 UI 연결 지점이다.
- 현재 의존성에는 TOTP/WebAuthn 구현이 없고 TOTP 비밀용 암호화 키 관리도 없다.

## 제안의 이유

- SQLite에 만료되는 인증 대기 상태를 두고 트랜잭션으로 일회성 소비를 보장한다. Redis는 추가하지 않는다.
- QR는 서버에서 생성한다. 외부 QR 서비스 및 첨부 사진의 키는 사용하지 않는다.
- TOTP는 AES-GCM 암호화, 복구 코드는 해시, Passkey는 공개키로 저장한다.
- Passkey는 고정 HTTPS 도메인을 사용한다. HTTP LAN IP에서는 비활성 사유와 기존 로그인을 제공한다.
- 사용자 확인 필수 Passkey 로그인에는 TOTP를 중복 요구하지 않는다. 비밀번호 경로는 TOTP 활성 계정만 두 번째 인증을 요구한다.
- 상세 정책은 설계 문서의 제안이며, 승인 전 구현 완료로 표시하지 않는다.

## 참조

- https://github.com/go-webauthn/webauthn
- https://github.com/pquerna/otp
- https://www.w3.org/TR/webauthn-3/
- https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html

## 설계 검토와 현재 검증

- 독립 검토자는 설계에서 구현 계획을 막을 중요한 미해결 질문이 없다고 보고했다. 제품 보안 검증이나 실기기 인증 테스트를 수행했다는 의미는 아니다.
- 문서 4개의 존재·UTF-8 대체 문자 부재·제목·끝 개행 및 사용자 승인/구현 항목이 미완료임을 Node assert로 확인했다.
- `git diff --exit-code -- internal frontend cmd go.mod go.sum compose.yaml Dockerfile`는 0으로 종료했다. 제품 코드·의존성·배포 파일은 변경하지 않았다.
- 제품 구현과 관련 테스트 실행은 설계 승인 후 진행한다. 이번 단계에서는 Go/프런트 테스트·브라우저 검증을 수행하지 않았다.

## 설계 승인 후 상세 계획과 기준 테스트

- 사용자의 진행 요청으로 설계 승인을 기록했다. 상세 계획은 `docs/superpowers/plans/2026-10-25-account-mfa-passkeys.md`에 저장했다.
- writing-plans 지침에 따라 상세 계획 검토 및 실행 방식 선택을 요청했다. 직접 구현 후 최종 독립 검토를 권장했다. 인증 저장·세션·TOTP·Passkey가 같은 상태 경계를 공유하므로 단계별 구현자 교체보다 일관성을 유지하기 쉽다는 이유다.
- QR 등록 대기 10분과 재인증 5분은 구별한다. setup에서는 증명을 확인하고 confirm에서 소비한다. 만료 후에는 새 증명을 받아야 한다.
- 강제 로그아웃 후에도 신규 복구 코드는 사용자가 저장할 때까지 App의 인증 UI 밖 메모리에 표시한다. 브라우저 저장소는 사용하지 않는다.
- 기준 검사 명령은 `.tools/go/bin/go.exe test ./internal/core -run 'TestLoginLogoutAndCSRF|TestStrictJSONAndLoginLimit|TestAccountChangesRevokeCredentials|TestPublicOriginAndSecureCookie|TestDataDirLockAndInvalidPublicURL' -count=1`이며 통과했다. GOCACHE/GOMODCACHE는 프로젝트의 `.tools` 아래로 지정했다.
- `npm.cmd --prefix frontend test`는 5개 파일/26개 테스트가 통과했다. 이 결과는 기존 기능의 기준 상태이며 아직 만들지 않은 MFA/Passkey 기능 검증 결과가 아니다.
- 제품 코드·의존성·배포 파일은 아직 변경하지 않았다.

## 구현 1단계

- DB v3는 v2 계정·세션·API 토큰·플러그인을 보존한다. 보안 버전과 단회 challenge 저장을 추가했으며 인증 상태는 사용자·세션·브라우저·목적으로 묶는다.
- TOTP 키는 auth.key에 별도로 생성하고 AES-256-GCM의 사용자/목적 AAD로 보호한다. 기존 암호문이 있으면 누락된 키를 재생성하지 않는다. 복구 CLI 지원을 위해 서버 Open 자체에서는 키를 복호화하지 않는다.
- 새 테스트의 미구현 컴파일 실패를 확인한 뒤 구현했다. Go 코어 전체 테스트가 33.520초에 통과했고 git diff --check도 통과했다.
- 기존 Go 전용 저장소와 codex/go-platform 브랜치를 사용한다. 기본 작업공간은 이전 Python 저장소이므로 별도 worktree를 잘못 생성하지 않고 모든 명령에 Go 경로를 명시했다.

## 구현 2단계

- pquerna/otp 공식 릴리스 v1.5.0을 고정했다. QR PNG는 서버 내부에서 생성하며 외부 QR 서비스로 키를 보내지 않는다.
- MFA 로그인은 별도의 5분 쿠키·브라우저 바인딩을 사용하며 정상 세션은 최종 코드 검증 후에만 발급한다. TOTP 시간 단계·복구 코드·challenge·세션은 같은 트랜잭션으로 소비한다.
- TOTP 설정 대기는 10분, 재인증은 5분이다. 재인증이 만료되어도 새 증명을 받아 기존 QR 등록을 완료할 수 있다. 등록·재발급·비활성화는 모든 웹 세션을 폐기하고 API 토큰은 보존한다.
- HTTP 실패 테스트의 404를 확인한 뒤 구현했다. 코드 재사용/동시 소비·대기 상태 접근 차단·Bearer 거부·세션 간 proof 재사용·실패 예산 유지 테스트와 전체 Go 코어 테스트가 통과했다. 전체 테스트는 41.624초에 통과했다.

## 구현 3단계

- go-webauthn 공식 v0.18.2를 고정했다. 표준 JSON session을 그대로 보관하고 등록·서명 검증은 라이브러리에 맡긴다. 테스트 전용 ES256 키는 매번 임시 생성하며 운영 키를 사용하지 않는다.
- RP ID는 최초 등록 때 고정한다. 호스트가 바뀌면 기존 Passkey를 전부 삭제하기 전까지 Passkey만 비활성화하며 비밀번호/TOTP 경로는 유지한다.
- 비동기화 키의 counter 복제 경고는 거부한다. 백업 가능한 동기화 키는 라이브러리의 서명·backup flag 검증을 통과하면 허용하되 counter 경고를 감사 기록한다. 이 정책과 UV 누락·Origin/RP/challenge/handle/서명 변조·삭제/비활성화·동시 finish를 테스트했다.

## 구현 4단계

- 기존 Card/Button/Input과 비밀번호 변경 화면을 재사용해 계정 보안 메뉴를 추가했다. TOTP 등록과 만료 후 재인증, Passkey 관리, 미지원 브라우저 안내를 연결했다.
- 브라우저의 기본 WebAuthn JSON 변환만 사용한다. 미지원 환경에 자체 암호화/바이너리 변환 대체 구현은 추가하지 않았다.
- 복구 코드 표시 상태는 App 인증 분기 밖 메모리에 둔다. 로그아웃 후에도 남고 저장 확인 후 사라지는 것을 UI 테스트로 확인했다. URL/localStorage/sessionStorage에는 저장하지 않는다.
- 전체 프런트 테스트 7개 파일/35개 테스트와 TypeScript 검사 포함 프로덕션 빌드가 통과했다. 실제 브라우저·390px 화면 검증은 최종 통합 단계에서 수행한다.

## 구현 5단계와 통합 검증

- 비상 복구 CLI는 사용자 이름을 두 번 확인하고 데이터 디렉터리 독점 잠금을 요구한다. 암호화 키가 누락/손상되어도 대상 계정의 MFA·Passkey·세션·토큰을 원자 폐기하고 비밀번호와 역할은 보존한다. 실행 중 거부와 오프라인 복구를 실제 Windows 실행파일에서도 확인했다.
- Windows 전체 Go 테스트·vet·go mod verify, 프런트 35개 테스트·빌드, Windows 실행파일 스모크가 통과했다. npm audit은 0건이고 govulncheck는 실제 호출/import 경로 0건, 기존 미사용 OpenPGP 모듈 경고 1건이다.
- Chrome 156의 가상 UV 인증기로 QR·TOTP 활성화, 복구 코드 로그인, Passkey 등록/로그인/이름 변경/재인증/삭제를 검증했다. 390px 및 데스크톱 화면을 확인했다. 첫 반응형 검사는 0.15초 CSS 전환 도중 실패했으며 전환 완료 조건을 기다리도록 테스트를 수정해 제품 CSS 변경 없이 통과했다.
- Docker amd64·arm64 이미지 빌드와 양쪽 실행파일 스모크가 통과했다. 전용 임시 볼륨에서 nonroot UID 65532, read-only rootfs, auth.key 0600/32바이트, QR·TOTP·복구 코드·오프라인 초기화·healthcheck를 양쪽 이미지에서 확인했다. ARM64는 에뮬레이션이며 실제 장비 검증이 아니다.
- Linux race 실패는 새 테스트가 httptest.ResponseRecorder.Result를 동시에 호출한 테스트 픽스처의 경쟁이었다. 쿠키를 goroutine 시작 전에 읽도록 수정한 뒤 해당 테스트를 race 모드로 10회 통과했다. 전체 race/vet 재검사와 독립 최종 보안 리뷰는 진행 중이다.
- 실제 Windows Hello, 휴대전화 QR 스캔, Synology 배포는 아직 확인하지 않았다. 원격 push·NAS 업로드는 수행하지 않는다.

## 최종 독립 리뷰와 수정

- 검토 범위는 a342351..5c798c8 및 테스트 픽스처 수정 fd4040f다. 초기 검토 실행은 세션 중단으로 결과 없이 종료되어 복구 후 최종 검토를 한 번 완료했다. Critical은 없고 Important 4건, Minor 1건이 보고되었다.
- 계정 변경 후 익명 Passkey assertion 수락, 익명 시작 요청 제한 초기화, auth.key 누락 시 DB/mutex 잠금 역전은 새 Go 테스트 세 개에서 실제 실패를 확인하고 수정했다. 동일 테스트는 수정 후 통과했다.
- 화면 이탈 후 재인증 후속 요청, 계정 전환 뒤 이전 복구 코드 표시, 취소된 요청의 늦은 401 이벤트를 프런트 테스트에서 재현하고 수정했다. 인증기 취소와 의도된 강제 로그아웃 후 코드 유지도 함께 검사한다.
- Ruling: 보안 변경 시 모든 익명 Passkey 대기를 폐기한다 — discoverable 단계에는 대상 사용자가 없어 기존 공통 폐기 트랜잭션을 확장하는 최소 수정이다 — 동시에 로그인 중인 다른 사용자는 다시 인증을 시작해야 할 수 있다.
- Ruling: API 토큰 유지 안내 누락을 Important로 상향한다 — 모든 기기 로그아웃이라는 표현은 자동화 자격까지 폐기된다는 오해를 만들며 승인된 보안 설계의 필수 안내다 — 안내를 잘못 판단하면 운영자가 남아 있는 토큰을 놓칠 수 있다. 누락 테스트 실패 후 화면 문구를 수정했다.
- 전체 테스트와 최종 빌드 재검증이 끝나기 전에는 완료로 표시하지 않는다. 리뷰 수정은 한 번의 수정 단계와 회귀 테스트로 검증하며 별도 재리뷰는 요청하지 않는다.

### 검토자가 직접 판단하지 않은 범위에 대한 결정

- Ruling: Linux 전체 race 결과는 실행자가 확인한다 — 리뷰 수정 후 전체 race와 vet를 실제 실행해 통과했다 — 독립 재실행이 아닌 실행자의 검증 증거다.
- Ruling: 전체 Go·프런트·Docker 검사의 중복 독립 실행은 하지 않는다 — 실행자의 실제 명령 결과와 검토자의 별도 인증 테스트 결과를 구분한다 — 같은 환경의 숨은 결함을 모두 제거하는 보장은 아니다.
- Ruling: Windows Hello·휴대전화 스캔·물리 보안 키·제공자 간 동기화는 실기기 확인 항목으로 남긴다 — 가상 UV 인증기로 표준 흐름을 검증했다 — 기기별 호환성은 배포 전 확인이 필요하다.
- Ruling: Chrome 외 브라우저/OS 및 전체 보조공학 접근성은 미검증으로 명시한다 — 기본 레이블과 취소·모바일 화면은 테스트했다 — 다른 환경에서 추가 UI 조정이 필요할 수 있다.
- Ruling: NAS 배포와 운영 HTTPS·프록시·NTP·ACL 설정은 변경하지 않는다 — 요청 범위는 로컬 구현이다 — 운영 설정 오류는 별도 배포 점검이 필요하다.
- Ruling: 운영 DB 업그레이드/백업 복원은 합성 마이그레이션 검증과 구분한다 — 실제 사용자 DB를 건드리지 않았다 — 배포 전 전체 데이터 백업이 필요하다.
- Ruling: 의존성 전체의 암호학/공급망 감사를 수행했다고 주장하지 않는다 — 표준 라이브러리와 고정 의존성 및 취약점 도달 검사를 사용한다 — 알려지지 않은 외부 결함은 남을 수 있다.
- Ruling: 대규모 부하·퍼징·광범위 장애 주입은 이번 검증에 포함하지 않는다 — 인증 경합과 키 누락 등 재현된 경계는 별도 회귀 테스트로 검증한다 — 대규모 운영 전 추가 성능/장애 검증이 필요하다.
- Ruling: 복사·다운로드한 비밀과 호스트 전체 침해는 앱 보장 밖이다 — 브라우저 저장소에는 비밀을 남기지 않는다 — 사용자가 클립보드·다운로드·백업 접근을 보호해야 한다.
- Ruling: 무관한 플러그인·작업 큐·관리자 권한 모델 전체 감사는 하지 않는다 — 연결된 인증 경계와 기존 회귀 테스트만 확인했다 — 이번 인증 검토가 전체 플랫폼 보안 감사를 의미하지 않는다.

보류한 Minor는 없다. API 토큰 안내는 보안 기대와 관련된 필수 요구로 상향하여 실패 테스트 후 수정했다.

## 최종 완료 기록

- Ruling: 기존 전용 Go 저장소와 codex/go-platform 브랜치를 재사용한다 — 현재 로컬 프로젝트 적용 요청이며 기본 도구 작업공간은 다른 Python 저장소다 — 별도 worktree 수준의 격리는 없지만 논리적 커밋으로 복원 가능하다.
- Final: fixed 이전 assertion 수락 — TestPasskeyLoginRejectsSecurityChange의 비밀번호/TOTP 변경 사례 RED→GREEN.
- Final: fixed 익명 발급 제한 해제 — TestPasskeyBeginKeepsIssuanceBudget RED→GREEN.
- Final: fixed 키 누락 잠금 역전 — TestAuthSecretMissingKeyDoesNotInvertDBLock RED→GREEN.
- Final: fixed 오래된 보안 응답 — 화면 이탈/계정 전환/늦은 401 테스트 RED→GREEN, 인증기 취소 추가 검사 통과.
- Final: fixed API 토큰 유지 안내 — 화면 안내 테스트 RED→GREEN. 이후 프런트 40/40 및 Go 전체/race suite가 통과했다.

- 제품 최종 수정은 74acfda와 a3d950a이며, 앞선 단계 커밋은 4a3d3fb·3397bce·caa8dc3·dce2554·2ece617이다. 작업 브랜치는 codex/go-platform에 그대로 유지한다.
- 최종 Windows `go test ./... -count=1`은 core 70.328초를 포함해 4패키지가 통과했다. Linux `go test -race ./... -count=1`은 core 161.590초를 포함해 통과했고 양쪽 vet도 종료 0이다.
- 프런트 40개 테스트·TypeScript/Vite 빌드, 최종 Windows 스모크·Chrome 가상 인증기, 최종 Docker amd64/arm64 스모크와 MFA·키 권한·오프라인 복구가 통과했다. 실제 결과는 docs/verification.md에 기록했다.
- 최종 Docker 이미지는 modudock-go:mfa-local이며 dist/modudock-go-mfa.tar에 저장했다. 기존 dist/modudock-go-amd64.tar는 이전 산출물이므로 이번 업데이트 파일이 아니다. 5개 OS/아키텍처 실행파일과 SHA256SUMS도 갱신했다. macOS는 컴파일만 확인했다.
- 테스트용 Windows 서버, Docker 컨테이너·볼륨은 종료/제거했다. 기존 NAS·Python 프로젝트·운영 데이터·원격 저장소는 변경하지 않았다.
