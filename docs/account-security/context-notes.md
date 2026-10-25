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
