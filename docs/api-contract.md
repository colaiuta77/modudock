# ModuDock Go v1 API 계약

모든 경로는 `/api/v1` 아래에 있으며 JSON key는 snake_case다. 시간은 UTC RFC3339, ID는 opaque string이다. 오류는 `{ "error": { "code": "...", "message": "..." } }`이다. 목록은 `{ "items": [], "total": 0 }`이고 `limit` 기본 50·최대 200, `offset` 기본 0이다. 서버는 허용하지 않은 필드, 초과 크기, 잘못된 타입을 거부한다. 변경 성공은 생성 201 또는 JSON 200, 삭제 204다.

## 인증·계정·토큰

- `POST /auth/login` body `{username,password}` → MFA가 없으면 200 `{user,csrf_token}`와 HttpOnly 세션 쿠키. TOTP 계정은 202 `{mfa_required:true,expires_at}`와 5분 대기 쿠키만 반환한다.
- `GET /auth/me` → `{user,csrf_token}`. API token 요청의 csrf_token은 빈 문자열이다.
- `POST /auth/logout` → 204, 세션 폐기.
- `PUT /auth/password` body `{current_password,new_password}` → 204, 모든 기존 세션·API token 폐기.
- 쿠키 인증 변경 요청에는 `X-CSRF-Token`을 보내고 서버는 Origin도 검사한다. Bearer 인증은 CSRF 대신 token scope를 검사한다.
- User는 `{id,username,role,is_active,created_at}`이며 role은 `admin|operator|viewer`다.
- 관리자 `GET/POST /users`, `PUT/DELETE /users/{id}`. 생성 `{username,password,role}`, 변경 `{username,role,is_active,password?}`. 마지막 활성 관리자 제거·강등 금지. 자기 삭제 금지. 비밀번호 재설정·비활성화·역할 변경은 세션과 토큰 폐기.
- 관리자 `GET/POST /tokens`, `DELETE /tokens/{id}`. 생성 `{name,scopes:[...],expires_at}` → `{token,metadata}`. owner는 생성 관리자 본인. 허용 scopes는 `status:read`, `plugins:read`, `plugins:write`, `runs:read`, `runs:write`, `schedules:read`, `schedules:write`, `logs:read`, `users:write`, `tokens:write`. 토큰은 해시로 저장한다.
- Token metadata는 `{id,name,scopes,expires_at,created_at}`다. `/auth/me`는 유효한 토큰이라면 scope와 무관하게 본인 정보만 반환한다.

## 계정 보안

아래 공개 인증 요청도 POST에는 정확한 `Origin`이 필요하다. 대기 쿠키는 정상 인증 세션이 아니다.

- `GET /auth/options` → `{passkey_available,passkey_reason}`.
- `POST /auth/mfa` body `{method:"totp"|"recovery",code}`와 대기 쿠키 → 200 Auth. 코드 오류·만료는 401, 실패 예산 초과는 429다.
- `POST /auth/login/cancel` → 204, 대기 상태/쿠키 폐기.
- `POST /auth/passkey/begin` body `{}` → `{publicKey:...}`와 대기 쿠키.
- `POST /auth/passkey/finish` body `{credential:...}`와 대기 쿠키 → 200 Auth. credential은 브라우저 `PublicKeyCredential.toJSON()` 결과다.

다음은 본인 브라우저 세션만 허용한다. 모든 변경에 Origin/CSRF를 검사하며 Bearer는 scope와 무관하게 403이다.

- `GET /auth/security` → `{totp_enabled,recovery_codes_remaining,passkeys:[{id,name,created_at,last_used_at}],passkey_available,passkey_reason}`. 기존 비밀 키와 복구 코드 원문은 반환하지 않는다.
- `POST /auth/reauth` body `{purpose,password,method?,code?}` → `{proof,expires_at}`. TOTP가 켜져 있으면 method/code도 필요하다. 목적은 `totp-setup`, `totp-disable`, `recovery-regenerate`, `passkey-register`, `passkey-delete:<id>`다.
- `POST /auth/totp/setup` body `{proof}` → `{secret,qr_png,expires_at}`. QR은 PNG data URL이고 등록 대기는 10분이다. setup은 proof를 소비하지 않는다.
- `DELETE /auth/totp/setup` → 204, 현재 세션의 등록 대기만 취소.
- `POST /auth/totp/confirm` body `{proof,code}` → 200 `{recovery_codes:[...],user,csrf_token}`와 새 세션 쿠키. 최종 변경 시 5분 proof를 소비하고 기존 웹 세션을 교체한다.
- `DELETE /auth/totp` body `{proof}` → 200 Auth와 새 세션 쿠키, TOTP/복구 코드 삭제.
- `POST /auth/recovery-codes` body `{proof}` → 200 `{recovery_codes:[...],user,csrf_token}`와 새 세션 쿠키. 기존 코드 전체 무효화.
- `POST /auth/passkeys/register/begin` body `{proof}` → `{publicKey:...}`와 세션에 연결된 ceremony 쿠키.
- `POST /auth/passkeys/register/finish` body `{proof,name,credential}` → 200 Auth와 새 세션 쿠키.
- `POST /auth/passkeys/reauth/begin` body `{purpose}` → `{publicKey:...}`와 ceremony 쿠키.
- `POST /auth/passkeys/reauth/finish` body `{credential}` → `{proof,expires_at}`.
- `PUT /auth/passkeys/{id}` body `{name}` → `{name}`, 로그인 유지.
- `DELETE /auth/passkeys/{id}` body `{proof}` → 200 Auth와 새 세션 쿠키.

재인증 오류·만료는 403 `reauth_required`다. 보안 수단 변경은 기존 웹 세션·인증 대기를 모두 폐기하고 현재 브라우저용 세션만 원자적으로 새로 발급한다. 클라이언트는 응답의 csrf_token을 다음 변경 요청 전에 반영하고 설정 화면을 유지한다. 원문 복구 코드는 최종 응답에서 한 번만 보내므로 설정 화면 메모리에만 저장하고 닫기·이탈·로그아웃 시 제거한다. 보안 수단 변경은 자동화 토큰을 보존하고, 비밀번호/역할/활성 상태 변경 및 비상 복구는 토큰까지 폐기한다. TOTP 해제와 Passkey 삭제는 다른 삭제 API와 달리 새 인증 정보를 반환하므로 204가 아닌 200이다.

## 상태·로그

- `GET /status` → `{version,started_at,plugins_active,runs_queued,runs_running,runs_succeeded,runs_failed}`. viewer 이상, `status:read`.
- `GET /logs?level=&q=&run_id=&limit=&offset=` → Log 목록. `{timestamp,level,message,component,plugin_id,task_id,run_id,user_id,request_id}`. admin은 전체, operator는 run_id를 명시한 작업 로그만 허용하며 viewer는 거부. `logs:read`.
- `GET /events` → 인증 SSE. UI는 메시지 수신 시 필요한 데이터를 다시 조회한다. 연결 때 snapshot 이벤트, 이후 change/heartbeat. 데이터가 많은 임의 payload는 이벤트에 넣지 않는다.
- `GET /openapi.json` → 인증 사용자에게 OpenAPI 명세. `/healthz`는 인증 없이 `{status:"ok"}`만 반환한다.

## 플러그인

- Plugin은 `{id,name,version,state,repository_url,active_release_id,manifest}`. state는 `active|disabled`. 목록·manifest에서 비밀 설정값을 노출하지 않는다.
- `GET /plugins`, `GET /plugins/{id}`. viewer 이상, `plugins:read`.
- 관리자 `POST /plugins/install` body `{repository_url,ref}` → Plugin. 설치는 timeout과 전역 설치 lock으로 제한한다. `plugins:write`.
- 관리자 `POST /plugins/{id}/update` body `{ref}` → Plugin. 동일 저장소로 설치 흐름을 재사용한다.
- 관리자 `GET /plugins/{id}/releases` → `{items:[{id,version,commit_sha,data_schema_version,created_at}],total}`.
- 관리자 `POST /plugins/{id}/activate` body `{release_id}` → Plugin. 동일 data_schema_version만 허용한다.
- 관리자 `PUT /plugins/{id}/state` body `{enabled}` → Plugin. 비활성화 시 대기 작업 취소.
- 관리자 `GET/PUT /plugins/{id}/settings`. 응답/입력은 설정 JSON 객체다. secret schema 필드는 읽기 시 마스킹하고 빈 마스킹값을 보내면 기존값 유지한다.
- `GET /menus` → `{items:[{plugin_id,plugin_name,id,title,icon,position,enabled}],total}`. 설정 편집을 위해 관리자에게 disabled도 반환한다.
- 관리자 `PUT /menus/{plugin_id}/{menu_id}` body `{title,position,enabled}` → 해당 메뉴.
- `GET/POST/PUT/DELETE /plugins/{id}/resources/{resource}`는 manifest에 선언한 method만 허용한다. GET은 viewer 이상, 쓰기는 operator 이상이고 token은 plugins:read/write scope를 요구한다.

## 작업·스케줄

- Run은 `{id,plugin_id,release_id,task_id,state,progress,created_at,started_at,finished_at,error,input?,result?}`. viewer에는 input/result/error 상세를 노출하지 않는다.
- `GET /runs`, `GET /runs/{id}`. viewer 이상, `runs:read`.
- operator 이상 `POST /plugins/{id}/tasks/{task_id}/runs` body `{input:{...}}` → Run. optional `Idempotency-Key`를 요청자별로 저장하고 다른 payload 재사용은 409. `runs:write`.
- operator 이상 `POST /runs/{id}/cancel` → Run. `runs:write`.
- Schedule은 `{id,name,plugin_id,task_id,cron,timezone,input,enabled,next_run_at,last_run_at}`.
- operator 이상 `GET/POST /schedules`, `PUT/DELETE /schedules/{id}`. 생성·수정 body는 ID·실행 시각을 제외한 Schedule 필드. token은 schedules:read/write scope.
- cron은 분 단위 5필드, timezone은 IANA 식별자다. 중단 중 누락은 재생하지 않으며 중복 실행도 누적하지 않는다.

## 플러그인 JSON 규약

manifest 파일은 `modudock.json`이다. 필수 값은 schema_version=1, protocol_version=1, core_api=1, id, name, version, data_schema_version>=1, artifacts다.

artifact key는 `linux-amd64`, `linux-arm64`, 개발용 `windows-amd64` 등이고 값은 `{url,sha256,executable}`다. 파일은 zip 아카이브이며 URL은 해당 GitHub 저장소의 Release download URL이다.

tasks는 `[{id,title,input_schema,timeout_seconds,idempotent}]`, resources는 `[{id,methods,input_schema,timeout_seconds}]`, menus는 `[{id,title,icon,resource?}]`, settings_schema는 JSON Schema 객체다. 미선언 권한·런타임은 거부한다. 기본 실행에는 별도 런타임이 없다.

프로세스 stdin에 JSON 한 줄을 전송한다. `{protocol_version:1,operation:"health|task|resource",task_id?,resource?,method?,input:{},settings:{},data_dir:"...",run_id?}`. stdout은 줄별 JSON 이벤트 `{type:"progress",progress:0..100}` 또는 최종 `{type:"result",result:{...}}`다. health는 result에 `{healthy:true,protocol_version:1}`를 반환한다. stderr는 제한된 로그다. 마지막 result 뒤 추가 출력·여러 result·유효하지 않은 메시지는 오류다.
