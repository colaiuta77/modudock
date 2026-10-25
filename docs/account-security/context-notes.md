# 계정 보안 보강 결정 기록

## 2026-10-25 요청과 작업 경계

- 사용자는 계정별 TOTP MFA, 복구 코드, Passkey 비밀번호 없는 로그인을 로컬에 구현하도록 요청했다. TOTP는 첨부 화면처럼 QR 스캔과 수동 키 등록을 지원한다.
- 대상은 `D:/Documents/ChatGPT/ModuDock-Go`이다. 기존 `Modudock`은 수정하지 않는다.
- 원격 푸시, NAS 업로드, 운영 계정·DB 변경은 범위가 아니다.
- 시작 시 `git status --short`는 비어 있고 브랜치는 `codex/go-platform`이었다.
- 브레인스토밍 지침의 설계 문서 승인 단계가 남아 있다. 제품 코드와 의존성 변경 전 설계를 검토받는다.

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
