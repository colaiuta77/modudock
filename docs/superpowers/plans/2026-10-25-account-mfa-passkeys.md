# Account MFA and Passkeys Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 계정별 TOTP QR 등록, 단회용 복구 코드, UV 필수 Passkey 로그인을 기존 ModuDock-Go에 구현한다.

**Architecture:** 기존 로그인에서 세션 발급을 분리하고 SQLite의 단회성 인증 상태를 거친다. TOTP 비밀은 로컬 파일 키로 암호화하고 복구 코드는 해시, Passkey는 공개키로 저장한다. UI는 기존 컴포넌트를 재사용한다.

**Tech Stack:** Go, modernc SQLite, React/TypeScript, Vite 8, Tailwind/shadcn/ui, Lucide, pquerna/otp, go-webauthn/webauthn, 브라우저 WebAuthn API.

**Spec:** `docs/superpowers/specs/2026-10-25-account-mfa-passkeys-design.md`.

## Global Constraints

- 로컬 `D:/Documents/ChatGPT/ModuDock-Go`만 변경한다. 기존 Python 프로젝트, 운영 DB, NAS, 원격 저장소는 변경하지 않는다.
- 기존 Go·SQLite·Vite 8·React·Tailwind·shadcn/ui·Lucide 구성과 Docker/네이티브 설치를 유지한다.
- Redis, 외부 인증 서버, 이메일/SMS, SSO, 관리자 전체 MFA 강제 정책, 세션 관리 목록 UI는 추가하지 않는다.
- TOTP는 SHA-1, 6자리, 30초 주기와 앞뒤 한 주기 허용이다. 같은 시간 단계의 코드는 한 번만 소비한다.
- 복구 코드는 10개이며 각각 최소 128비트 난수다. 해시만 저장하며 재발급은 전부 교체한다.
- MFA/WebAuthn challenge와 재인증은 5분, TOTP 등록 대기는 10분이다. Passkey는 계정당 최대 10개다.
- Passkey는 명시된 HTTPS PublicURL 또는 개발용 http://localhost에서만 제공하고 정확한 Origin, RP ID, discoverable credential, UV를 검증한다.
- 보안 수단 실질 변경은 모든 웹 세션·대기 상태를 폐기하지만 자동화 API 토큰은 유지한다. 비밀번호/권한/활성 상태 변경과 비상 복구는 기존 토큰도 폐기한다.
- 새 소스 파일 첫 줄은 역할을 설명하는 한국어 주석이다. 파일 수정은 apply_patch를 사용한다. 무관한 리팩터링은 하지 않는다.

## Review Focus

1. 만료·재사용·동시 인증 완료는 정상 세션이나 일회용 코드를 중복 생성/소비하지 않아야 한다. Task 1·2·3의 병렬 요청 테스트로 고정한다.
2. TOTP 키 파일 손상·누락이나 다른 사용자 암호문 교환이 인증을 우회하거나 키 자동 덮어쓰기로 이어지면 안 된다. Task 1의 암호화·복구 테스트로 고정한다.
3. QR 등록의 10분 대기 중 재인증 5분이 지났으면 새 증명이 필요하며 기존 TOTP는 새 코드 확인 전까지 유지되어야 한다. Task 2에서 시간을 고정한 테스트를 작성한다.
4. MFA 오류가 전체 로그인 상태를 잘못 초기화하거나 강제 로그아웃이 새 복구 코드 표시를 지우면 안 된다. Task 4에서 React 생명주기와 401 경로를 테스트한다.
5. PublicURL 변경·프록시 Host 조작·기기 인증 취소는 계정 잠금이나 약한 인증으로의 자동 전환을 일으키지 않아야 한다. Task 3·4에서 검증한다.

## 파일과 의존 관계

- Task 1은 `security_store.go`, `auth_secret.go`와 해당 테스트를 만들고 DB·기존 세션 발급/폐기·janitor를 수정한다.
- Task 2는 `mfa.go`, `mfa_test.go`를 만들고 로그인·라우트·OpenAPI를 연결한다.
- Task 3은 `passkeys.go`, `passkeys_test.go`를 만들고 Task 1의 세션/상태 저장과 Task 2의 재인증을 재사용한다.
- Task 4는 `Security.tsx`, `Security.test.tsx`, `webauthn.ts`, `webauthn.test.ts`를 만들고 기존 API 클라이언트·로그인·설정 메뉴·스타일을 수정한다.
- Task 5는 CLI 비상 복구, 전체 명세/운영 문서, 라이선스 고지와 통합 검증을 마무리한다.
- 순서는 1 → 2 → 3 → 4 → 5다. 모든 단계는 자체 테스트와 로컬 커밋을 가진다.

## 공통 HTTP 계약

모든 경로는 `/api/v1` 기준이다. 기존 성공 로그인 응답 `{user, csrf_token}`은 유지한다. WebAuthn begin 응답은 라이브러리의 `publicKey` 필드를 포함한 JSON 객체이며 브라우저는 해당 필드를 표준 JSON 변환 메서드에 전달한다.

- `GET /auth/options`는 익명에게 `{passkey_available, passkey_reason}`만 제공한다. 사용자 목록은 노출하지 않는다.
- `POST /auth/login`은 기존 200 성공 또는 202 `{mfa_required:true, expires_at}`를 반환한다. 대기 상태는 별도 HttpOnly 쿠키에 연결한다.
- `POST /auth/mfa`는 `{method, code}`와 대기 쿠키를 검증하고 200 기존 로그인 응답을 반환한다. method는 `totp` 또는 `recovery`다.
- `POST /auth/login/cancel`은 현재 브라우저의 비로그인 대기 상태를 폐기한다. 위 비로그인 변경 API는 정확한 Origin을 요구한다.
- `GET /auth/security`는 세션 전용이며 TOTP 활성 여부·남은 복구 코드 수·Passkey 메타데이터·가용 여부를 반환한다. 비밀은 반환하지 않는다.
- `POST /auth/reauth`는 `{purpose,password,method?,code?}`를 받고 `{proof,expires_at}`를 반환한다. purpose는 `totp-setup`, `totp-disable`, `recovery-regenerate`, `passkey-register`, `passkey-delete:<id>`로 제한한다.
- `POST /auth/totp/setup`은 `{proof}`를 받고 `{secret,qr_png,expires_at}`를 반환한다. qr_png는 서버가 생성한 PNG data URL이다. `DELETE /auth/totp/setup`은 취소한다.
- `POST /auth/totp/confirm`은 `{proof,code}`를 받고 `{recovery_codes,signed_out:true}`를 반환한다. `DELETE /auth/totp`는 `{proof}`를 검증하고 204와 세션 쿠키 삭제를 반환한다.
- `POST /auth/recovery-codes`는 `{proof}`를 받고 새 `{recovery_codes,signed_out:true}`를 반환한다.
- `POST /auth/passkey/begin`과 `/finish`는 익명 로그인용이다. begin은 WebAuthn JSON options, finish는 서명된 credential JSON을 받아 기존 로그인 응답을 반환한다.
- `POST /auth/passkeys/register/begin`은 `{proof}`와 세션을 요구하며 options를 반환한다. `/finish`는 `{proof,name,credential}`을 검증하고 `{signed_out:true}`를 반환한다.
- `POST /auth/passkeys/reauth/begin`은 `{purpose}`를 받고 `/finish`는 `{credential}`을 받아 `{proof,expires_at}`를 반환한다. 현재 사용자 소유 credential만 허용한다.
- `PUT /auth/passkeys/{id}`는 `{name}`으로 본인 Passkey 이름만 바꾼다. `DELETE`는 `{proof}`로 해당 키를 삭제하고 세션을 폐기한다.
- 공개 로그인/MFA/Passkey 검증 실패는 401, 세션 보안 작업의 잘못된 증명은 403 `reauth_required`, 제한은 429다. 계정 존재·활성 상태를 익명에게 구별해 알리지 않는다.

### Task 1. 안전한 인증 저장과 세션 경계

**Files:** Create `internal/core/security_store.go`, `security_store_test.go`, `auth_secret.go`, `auth_secret_test.go`; modify `db.go`, `db_test.go`, `auth.go`, `auth_test.go`, `server.go`.

**Interfaces:** `authAccount`는 기존 User와 `PasswordHash string`, `AuthVersion int64`를 내부적으로 보유한다. `loadAuthAccount(ctx context.Context, userID string) (authAccount,error)`와 `issueSession(ctx context.Context, tx *sql.Tx, userID string, version int64) (authSession,error)`을 제공한다. `authSession`은 `User User`, `CSRF string`, `RawCookie string`을 가지며 RawCookie는 JSON에 직렬화하지 않는다. `revokeWebCredentials(tx *sql.Tx,userID string) error`는 보안 버전을 올리고 웹 세션/인증 상태를 폐기한다. 기존 `revokeCredentials`는 여기에 토큰 폐기를 더한다.

`authChallenge`는 목적·사용자·보안 버전·세션/브라우저 바인딩 해시·만료·시도 수·JSON payload를 저장한다. `putAuthChallenge(ctx context.Context,c authChallenge) (string,error)`, `readAuthChallenge(ctx context.Context,raw string) (authChallenge,error)`, `claimAuthChallenge(tx *sql.Tx,raw,purpose,userID,sessionHash,bindingHash string,version int64,now time.Time) (json.RawMessage,error)`를 제공한다. raw 식별자는 256비트 난수이며 DB에는 해시만 저장한다. claim은 트랜잭션 안에서 검증·삭제하며 바인딩 불일치/만료/소비 완료를 거부한다.

- [ ] `TestSecurityMigrationPreservesV2Data`, `TestAuthChallengeSingleUseAndBindings`, `TestAuthSessionRejectsChangedAccount`, `TestAuthSecretMissingOrCorruptKey`, `TestAuthSecretAADIsolation`을 먼저 작성한다. v2 사용자/세션/토큰/플러그인 보존, 동시 claim 성공 수 1, 잘못된 사용자/용도/브라우저/세션/버전 거부, 암호문이 있는데 새 키 미생성, 다른 사용자 복호화 실패를 assert한다.

```go
if successes != 1 { t.Fatalf("challenge consumed %d times", successes) }
if schemaVersion != 3 { t.Fatalf("schema version = %d", schemaVersion) }
if _, err := os.Stat(keyPath); !errors.Is(err, os.ErrNotExist) { t.Fatal("missing key was recreated") }
```

- [ ] `go test ./internal/core -run 'TestSecurity|TestAuthChallenge|TestAuthSession|TestAuthSecret' -count=1`로 새 테스트의 실패 원인을 확인한다.
- [ ] v3에 `users.auth_version` 및 `auth_challenges`, `user_totp`, `recovery_codes`, `passkeys`, `auth_settings`를 추가한다. FK 삭제와 인덱스, 만료 정리를 포함한다. Passkey user handle은 사용자 ID에 연결된 안정적인 난수로 저장한다.
- [ ] `sealAuthSecret(userID,purpose string,plain []byte) ([]byte,error)`와 `openAuthSecret(userID,purpose string,sealed []byte) ([]byte,error)`를 Server 메서드로 구현한다. AES-256-GCM, 32바이트 키, 독점 생성, nonce, AAD, Linux 0600, 손상/누락 fail-closed를 적용한다. 기존 암호문 여부 확인과 키 초기화는 동기화하고 DB 연결을 점유한 트랜잭션에서 중첩 DB 조회로 교착하지 않는다.
- [ ] 기존 비밀번호 로그인이 공통 세션 발급을 사용하게 한다. 계정 변경은 버전 증가와 자격증명 폐기를 같은 트랜잭션에서 수행한다. 비밀번호 검증 후 계정이 바뀌면 세션을 만들지 않는다.
- [ ] 같은 명령 및 기존 인증 테스트를 통과시키고 `git diff --check`를 확인한다. 변경 파일만 커밋한다. 권장 메시지는 `feat(auth): add secure challenge storage and session boundary`다.

### Task 2. TOTP QR·복구 코드·재인증

**Files:** Create `internal/core/mfa.go`, `mfa_test.go`; modify `auth.go`, `server.go`, `openapi.go`, `openapi_test.go`, `go.mod`, `go.sum`.

**Interfaces:** Task 1의 저장/세션 함수와 기존 `principal`, `endpoint`, `writeJSON`, `decodeJSON`, `audit`를 사용한다. `registerSecurityRoutes()`를 제공하고 공통 HTTP 계약의 options/security/reauth/TOTP/recovery/MFA 경로를 등록한다. `requireSecuritySession(w http.ResponseWriter,r *http.Request,p principal) bool`은 Bearer를 거부한다. `validateReauthProof(ctx context.Context,p principal,proof,purpose string) error`는 시작 단계에서 확인만 하고, 최종 변경은 `claimAuthChallenge`로 소비한다. 보안 변경에는 Task 1의 `revokeWebCredentials`를 그대로 사용해 일반 API 토큰을 보존한다.

- [ ] `TestTOTPEnrollmentRequiresConfirmation`, `TestTOTPResetPreservesOldUntilConfirmed`, `TestTOTPReplayAndConcurrentConsumption`, `TestRecoveryCodesSingleUseAndRegeneration`, `TestMFAIncompleteCannotAuthenticate`, `TestSecurityRoutesRejectBearerAndCrossUser`, `TestReauthExpiryDuringTOTPSetup`를 작성한다. 복구 코드 10개, 128비트 이상, DB 원문 부재, 정확한 QR URI/수동 키 일치, 5분/10분 경계와 전체 세션 폐기를 assert한다.

```go
if len(codes) != 10 { t.Fatalf("recovery code count = %d", len(codes)) }
if protected.StatusCode != http.StatusUnauthorized { t.Fatal("partial login authorized") }
if replay.StatusCode != http.StatusUnauthorized { t.Fatal("used code accepted") }
```

- [ ] `go test ./internal/core -run 'TestTOTP|TestRecovery|TestMFA|TestSecurityRoutes|TestReauth' -count=1`로 실패를 확인한다.
- [ ] 공식 소스와 Go 호환성을 확인한 pquerna/otp 버전을 고정한다. 20바이트 이상 TOTP 시크릿, 6자리·30초·SHA-1, ModuDock issuer, 로컬 QR PNG를 구현한다. Base32 복구 코드 26자를 읽기 쉽게 묶고 ASCII 대소문자·하이픈만 명시적으로 정규화한다.
- [ ] setup에서는 증명을 확인하되 소비하지 않고 pending 키를 세션/보안 버전에 묶는다. confirm에서 5분 내 증명을 소비한다. setup이 유효한 10분 안이라도 증명이 만료되면 같은 purpose의 새 재인증을 받으며 기존 pending 키는 유지한다. 재설정 취소/실패는 활성 키를 건드리지 않는다.
- [ ] 비밀번호 확인 뒤 MFA 계정은 짧은 HttpOnly 대기 쿠키만 발급한다. TOTP 마지막 시간 단계 또는 복구 코드 삭제와 challenge 소비·세션 발급은 한 트랜잭션이다. 실패는 인증 상태의 실패 횟수를 올리고 정상 세션을 만들지 않는다.
- [ ] challenge당 실패 5회, 사용자/IP 단위 15분간 실패 8회, 사용자당 pending 8개/전체 4096개 상한을 적용한다. 비밀번호 단계 성공이 MFA 단계 실패 예산을 초기화하지 않도록 목적별 예산을 분리한다. 신규 요청으로 계정 실패 예산을 초기화하지 않고 성공/만료/취소 정리를 테스트한다.
- [ ] MFA 상태/변경/취소/복구 API에 세션·Origin·CSRF·본인 확인을 적용한다. TOTP 설정 계정의 재인증은 비밀번호+TOTP/복구 코드다. 일회용 증명을 최종 변경과 같은 트랜잭션에서 소비한다. 새 원문 코드 반환 전에 전체 웹 세션 폐기를 완료하고 `signed_out`을 보낸다.
- [ ] 명시된 테스트와 기존 auth/token/OpenAPI 회귀 테스트를 통과시킨다. 고정 의존성과 관련 파일만 `feat(auth): add TOTP QR enrollment and recovery codes`로 커밋한다.

### Task 3. Passkey 등록·로그인·관리

**Files:** Create `internal/core/passkeys.go`, `passkeys_test.go`; modify `server.go`, `mfa.go`, `openapi.go`, `openapi_test.go`, `go.mod`, `go.sum`.

**Interfaces:** `registerPasskeyRoutes()`와 `passkeyAvailability() (bool,string)`를 제공한다. 공통 계약의 begin/finish는 WebAuthn 라이브러리 JSON 형식을 그대로 사용한다. 세션 발급과 보안 변경은 Task 1·2 함수만 사용한다. 재인증 완료는 동일한 proof 형식을 반환한다.

- [ ] `TestPasskeyPublicOriginPolicy`, `TestPasskeyRegistrationAndDiscoverableLogin`, `TestPasskeyRejectsInvalidAssertion`, `TestPasskeyReauthOwnerAndPurpose`, `TestPasskeyDeletedOrDisabledDuringFinish`, `TestPasskeyLimitAndConcurrentFinish`를 작성한다. 서버가 검증 가능한 서명 fixture를 만들고 UV 없음/다른 challenge·RP·Origin·user handle, 미등록/삭제된 키, 동일 finish 동시 요청을 assert한다.

```go
if withoutUV.StatusCode != http.StatusUnauthorized { t.Fatal("UV was not required") }
if wrongOrigin.StatusCode != http.StatusUnauthorized { t.Fatal("wrong origin accepted") }
if successes != 1 { t.Fatalf("finish succeeded %d times", successes) }
```

- [ ] `go test ./internal/core -run TestPasskey -count=1`로 실패를 확인한다.
- [ ] go-webauthn 공식 API·릴리스·라이선스·보안 정보를 확인하고 호환 버전을 고정한다. key/user mapping과 라이브러리 credential/session JSON을 구현한다. 개인키 fixture는 테스트에서 생성하며 운영 credential은 사용하지 않는다.
- [ ] 설정한 PublicURL에서 RP를 구성한다. 빈 값·HTTP LAN IP·잘못된 RP 설정은 기능만 비활성화한다. `auth_settings`에 RP ID를 고정하고 변경 시 기존 로그인은 유지한다. 기존 자격증명이 모두 삭제된 경우에만 새 RP ID로 재등록을 허용한다.
- [ ] 등록은 resident key와 UV required, attestation none이다. begin에서 증명을 확인하고 finish에서 증명과 challenge를 원자 소비한다. 기기 이름은 공백 제거 후 1~100자이며 계정당 최대 10개를 트랜잭션 안에서 확인한다.
- [ ] 로그인은 발견 가능한 credential로 사용자를 찾되 user handle 소유권·활성 상태·최신 보안 버전을 재확인한다. UV는 서버에서도 필수다. counter가 지원되는 비동기화 credential의 복제 경고는 거부하며, 동기화/항상 0 counter는 라이브러리의 backup 규칙과 공식 정책대로 처리한다. 결정과 테스트를 기록한다.
- [ ] 기존 Passkey로 재인증하여 TOTP 등록/해제/재발급과 Passkey 등록/삭제를 할 수 있게 한다. 타인 키, 다른 purpose 또는 다른 세션 증명을 거부한다. 이름 변경 외 실질 변경은 전체 웹 세션을 폐기한다.
- [ ] `go test ./internal/core -run 'TestPasskey|TestTOTP|TestRecovery|TestReauth' -count=1`과 OpenAPI 테스트를 통과시키고 `feat(auth): add verified passkey authentication`으로 커밋한다.

### Task 4. 로그인·계정 보안 UI

**Files:** Create `frontend/src/Security.tsx`, `Security.test.tsx`, `webauthn.ts`, `webauthn.test.ts`; modify `api.ts`, `api.test.ts`, `App.tsx`, `App.test.tsx`, `Settings.tsx`, `styles.css`.

**Interfaces:** `LoginResult = {user:User;csrf_token:string} | {mfa_required:true;expires_at:string}`를 API 계층에서 구분하고 `signIn(username:string,password:string):Promise<LoginResult>`로 변경한다. `completeMFA(method: 'totp' | 'recovery',code:string):Promise<User>`, `signInWithPasskey():Promise<User>`를 제공한다. 성공한 인증 경로만 CSRF 상태를 설정한다. `SecurityPage({notify,onSignedOut,onRecoveryCodes})`는 기존 PageProps 패턴을 따른다. `onRecoveryCodes(codes:string[])`는 App의 인증 UI 바깥 메모리 상태에 일회용 표시를 올린다. 브라우저 credential 작업은 `createPasskey(options:unknown):Promise<unknown>`, `getPasskey(options:unknown):Promise<unknown>`로 격리한다.

- [ ] `Security.test.tsx`와 `App.test.tsx`에서 QR+수동 키 표시/복사, 잘못된 코드 재입력, 설정 취소, 복구 코드 재발급 확인, logout 후 코드 유지, 미지원 Passkey와 사용자 취소를 먼저 테스트한다. `api.test.ts`에 MFA 401이 세션 만료 이벤트를 만들지 않고 보호 API 401은 기존 동작을 유지함을 추가한다.

```tsx
expect(await screen.findByText(recoveryCodes[0])).toBeTruthy();
expect(screen.queryByRole('navigation')).toBeNull();
expect(localStorage.getItem('recovery_codes')).toBeNull();
expect(unauthorizedListener).not.toHaveBeenCalled(); // MFA 입력 오류 경로
```

- [ ] `npm --prefix frontend test -- src/Security.test.tsx src/webauthn.test.ts src/api.test.ts src/App.test.tsx`로 실패를 확인한다.
- [ ] 브라우저의 WebAuthn JSON 변환 메서드와 `navigator.credentials.create/get`을 feature detect하여 사용한다. 기능이 부족한 브라우저는 명확히 미지원으로 표시한다. 취소는 오류 알림 폭주 없이 해당 작업만 종료한다. 코드/비밀/credential을 localStorage·sessionStorage·URL에 저장하지 않는다.
- [ ] 기존 로그인에 202 대기 상태, TOTP/복구 코드 전환, 취소와 Passkey 버튼을 연결한다. 비밀번호·코드는 완료/취소 시 메모리에서 지운다. await 이후 오래된 요청이 새 로그인 상태를 덮어쓰지 않게 한다.
- [ ] 설정의 비밀번호 변경 항목을 계정 보안으로 확장한다. 기존 비밀번호 변경 컴포넌트와 Card/Button/Input을 재사용한다. QR alt 텍스트에는 키를 넣지 않는다. 6자리 입력은 leading zero를 보존하는 문자열과 `autocomplete=one-time-code`를 사용한다.
- [ ] 복구 코드는 서버에서 `signed_out`을 받으면 인증 UI 밖에 표시하고 저장 확인 후 폐기한다. 이미 발급한 원문 재조회 버튼은 만들지 않는다. Blob 다운로드 후 object URL을 해제한다. 만료된 재인증은 새 증명을 받고 유효한 setup QR는 유지한다.
- [ ] 390px 모바일과 데스크톱에서 가로 스크롤, 키보드 포커스, 레이블, 오류 알림, 비밀 넘침을 확인한다. 테스트 전체와 `npm --prefix frontend run build`를 통과시키고 `feat(ui): add account security and MFA login screens`로 커밋한다.

### Task 5. 비상 복구·운영 문서·종합 검증

**Files:** Modify `cmd/modudock/main.go`, `main_test.go`, `internal/core/mfa.go`, `mfa_test.go`, `README.md`, `docs/api-contract.md`, `docs/installation.md`, `docs/verification.md`, `scripts/notices.mjs` 필요 시 및 `docs/account-security/*`.

**Interfaces:** `ResetMFA(username string) error`는 서버 정지/독점 잠금 상태에서 대상의 모든 MFA 데이터·세션·토큰·challenge를 트랜잭션으로 폐기하고 안전한 감사 이벤트를 남긴다. CLI는 `admin reset-mfa --username NAME --data-dir PATH --confirm-username NAME`을 받는다. 두 이름이 같지 않으면 변경 없이 종료하며 비밀번호·권한을 변경하지 않는다.

- [ ] `TestResetMFARequiresExplicitConfirmation`, `TestResetMFAPreservesPasswordAndRevokesAll`, `TestResetMFAWorksWithoutEncryptionKey`, `TestResetMFARefusesRunningDataDirectory`를 작성하고 실패를 확인한다. 존재하지 않는 사용자는 데이터 변경 없이 실패해야 한다.

```go
if beforePasswordHash != afterPasswordHash { t.Fatal("reset changed password") }
if sessions+tokens+factors+challenges != 0 { t.Fatal("credentials survived reset") }
if err == nil { t.Fatal("reset accepted missing confirmation") } // 확인 누락 사례
```

- [ ] CLI와 reset을 최소 구현한다. 온라인 HTTP 복구 엔드포인트나 자동 MFA 우회는 만들지 않는다. 모든 계정의 암호문이 사라졌더라도 남은 키를 자동 삭제하지 않는다.
- [ ] OpenAPI 및 문서에 새로운 성공/대기/실패 계약, 세션 전용 경계, HTTPS/RP ID, NTP, 키+DB 백업, 구버전 롤백 제한, API 토큰과 MFA의 독립성을 기록한다. HTTP에서는 TOTP 비밀도 전송 중 노출될 수 있으므로 운영 HTTPS를 권고한다.
- [ ] 의존성 고지 생성과 `go mod verify`, 취약점 검사, npm audit를 수행한다. 바뀐 런타임 의존성만 설명하고 무관한 업데이트는 하지 않는다.
- [ ] `go test ./... -count=1`, `go vet ./...`, Linux 환경의 `go test -race ./...`, `npm --prefix frontend test`, `npm --prefix frontend run build`, `node scripts/build.mjs`를 실행한다. 각 종료 코드와 실제 결과를 기록한다.
- [ ] Docker Linux amd64/arm64 빌드와 Windows 네이티브 스모크를 수행한다. 임시 데이터로 기존 비밀번호 로그인→QR 등록→TOTP/복구 코드→Passkey→비상 복구를 검증한다. Docker 포트는 운영과 충돌하지 않는 로컬 임시 포트를 사용한다.
- [ ] 브라우저 가상 인증기를 이용한 등록/로그인 테스트와 사람이 확인하는 실제 Windows Hello/휴대폰 스캔을 구별한다. 실기기를 사용할 수 없으면 미검증으로 기록하며 통과로 표시하지 않는다.
- [ ] 최종 diff와 독립 보안 리뷰를 수행하고 지적 사항을 재현·수정·재검증한다. 실제 완료 항목만 체크한다. 논리적 변경에 맞춰 CLI 코드와 문서/검증 기록을 분리 커밋한다.

## 기준 상태 및 실행 환경

2026-10-25에 기존 인증 테스트 다섯 그룹은 통과했고 프런트는 5개 파일/26개 테스트가 통과했다. Go 실행 파일은 `.tools/go/bin/go.exe`, GOCACHE는 `.tools/gocache`, GOMODCACHE는 `.tools/gomodcache`다. Go가 PATH에 없으므로 Windows 명령에서 명시 경로 또는 세션 한정 PATH를 사용한다.

기능 구현 시작 시 using-git-worktrees 지침으로 작업 격리 여부를 결정한다. 로컬 제품 파일 변경과 캐시 쓰기는 현재 도구 권한 범위 밖일 수 있으므로 승인된 실행 경로를 사용하고 권한 우회 도구를 사용하지 않는다.

## 자체 검토

- 설계의 TOTP/복구/Passkey/재인증/암호화/비상 복구/UI/배포 검증을 Task 1~5에 모두 배치했다.
- HTTP 계약과 단계 간 공유 함수·응답 형식을 한 곳에 정의했다.
- 등록 대기 10분과 재인증 5분의 차이는 setup에서 확인, confirm에서 소비하는 방식으로 명시했다.
- 복구 코드가 강제 로그아웃으로 사라지는 UI 경계와 기존 API 토큰 유지 정책을 테스트에 포함했다.
- 제품 구현 및 계획 실행 방식 선택은 아직 완료되지 않았다.
