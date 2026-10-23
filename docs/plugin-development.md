# 플러그인 개발

ModuDock Go의 플러그인은 관리자가 신뢰하는 독립 실행파일이다. Go 공유 라이브러리, 소스 빌드, 설치 스크립트, Python/Node 런타임 자동 설치는 지원하지 않는다. 코어와 같은 OS 사용자 권한으로 실행하므로 프로세스 분리는 보안 sandbox가 아니다. 파일·네트워크 접근을 격리하지 않는다.

## 예제 빌드와 공개 릴리스 준비

`examples/hello`는 표준 라이브러리만 사용하는 실행 가능한 예제다.

```sh
go test ./examples/hello
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o out/linux-amd64/hello ./examples/hello
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o out/linux-arm64/hello ./examples/hello
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o out/windows-amd64/hello.exe ./examples/hello
(cd out/linux-amd64 && zip ../hello-linux-amd64.zip hello)
(cd out/linux-arm64 && zip ../hello-linux-arm64.zip hello)
(cd out/windows-amd64 && zip ../hello-windows-amd64.zip hello.exe)
sha256sum out/*.zip
```

`examples/hello/modudock.template.json`을 공개 GitHub 저장소 루트의 `modudock.json`으로 복사한다. 저장소·릴리스 URL과 **ZIP 파일** SHA-256을 실제 값으로 바꾼다. 이 템플릿의 placeholder는 의도적으로 설치 검증을 통과하지 않는다. manifest가 가리키는 사전 빌드 ZIP을 같은 저장소의 GitHub Release에 게시한 뒤 해당 manifest commit을 포함하는 ref로 설치한다. 릴리스 asset을 바꾼다면 새 digest와 새 manifest commit을 게시한다. 저장소·asset을 자동 게시하는 명령은 제공하지 않는다.

설치 API는 `POST /api/v1/plugins/install`의 `{ "repository_url":"https://github.com/OWNER/REPO", "ref":"v1.0.0" }`다. 관리자가 설치하며 ref는 먼저 immutable commit SHA로 해석된다. manifest도 그 SHA에서 읽는다. 공개 HTTPS GitHub만 허용하며 다른 호스트·저장소의 asset·사설 IP·허용하지 않은 redirect를 거부한다. GitHub API의 익명 요청 한도가 적용된다. private GitHub와 인증 토큰·SSH Git은 지원하지 않는다.

## Manifest

외부 계약 전체는 [API 계약](api-contract.md)에 있다. 필수 root 값은 `schema_version=1`, `protocol_version=1`, `core_api=1`, 안전한 소문자 `id`, `name`, semver 형태 `version`, `data_schema_version>=1`, 플랫폼별 `artifacts`다. 플랫폼은 `linux|darwin|windows`와 `amd64|arm64` 조합이며 현재 호스트의 정확한 조합이 필요하다. 선언하지 않은 필드·runtime·permissions는 거부한다. runtime이 필요 없는 독립 바이너리를 배포한다.

`tasks`는 ID·title·object `input_schema`·1–3600초 timeout과 idempotent 여부를 선언한다. `resources`는 ID·GET/POST/PUT/DELETE 중 허용 method·object input schema·1–60초 timeout을 선언한다. GET 입력은 query string의 단일 문자열 필드이며 나머지 method는 JSON 객체 본문이다. 결과는 직접 JSON 객체로 응답한다. 리소스는 동시에 최대 4개 프로세스다.

`menus`는 ID·title·icon과 선택적 resource ID를 선언한다. icon은 UI의 고정 허용 목록으로 표시하며 HTML/SVG/JS를 실행하지 않는다. 관리자 title·position·enabled 변경은 업데이트·롤백 후에도 보존한다. 해당 릴리스에서 빠진 메뉴는 표시하지 않는다.

JSON Schema는 Draft 2020-12 기준으로 검증한다. object root를 선언하고 보통 `additionalProperties:false`를 사용한다. 로컬 `#` 참조만 허용하며 `$id`와 외부 파일·네트워크 참조는 거부한다. `settings_schema`의 `writeOnly:true` 또는 `format:"password"` 필드는 읽을 때 `********`로 마스킹한다. 입력에서 `********` 또는 빈 문자열이면 기존값을 보존한다. 중첩 object/array, 로컬 참조와 조합 schema에서도 비밀 표시를 따른다. 조건·대안 중 하나라도 비밀로 선언하면 보수적으로 마스킹한다. 설정 API는 관리자 전용이다. 설정값을 stdout 결과나 로그에 직접 출력하지 않는다.

## 한 요청·한 프로세스 프로토콜

코어가 stdin에 JSON 한 줄을 보낸다.

```json
{"protocol_version":1,"operation":"task","task_id":"greet","input":{"name":"Ada"},"settings":{"greeting":"Hello"},"data_dir":"/data/plugin-data/hello","run_id":"opaque-id"}
```

operation은 `health`, `task`, `resource` 중 하나다. resource 요청은 `resource`와 `method`를 포함한다. stdout은 다음처럼 줄별 JSON이며 반드시 newline으로 끝난다.

```json
{"type":"progress","progress":50}
{"type":"result","result":{"message":"Hello, Ada!"}}
```

progress는 0–100 정수이며 result는 객체다. 마지막 result는 정확히 하나만 보내고 그 뒤 출력하지 않는다. unknown type·필드·잘못된 JSON·null progress·누락 result·여러 result는 실패한다. stdout에 디버그 텍스트를 섞지 않는다. health result는 `{ "healthy":true, "protocol_version":1 }`이며 10초 내 종료한다. health는 데이터 마이그레이션이나 외부 부수효과를 발생시키지 않아야 한다.

stdin 요청은 최대 128KiB, 누적 stdout은 1MiB, stderr는 64KiB다. 한도를 넘으면 프로세스를 종료한다. 저장하는 stderr 로그는 비밀 제거 후 8KiB로 잘라 JSON escaping 이후에도 로그 읽기 한도를 지킨다. 최소 PATH/LANG/TZ만 전달하고 코어의 환경 인증 비밀을 상속하지 않는다. Windows 개발에서는 SystemRoot와 시스템 PATH만 전달한다. 셸 문자열·임의 argv 없이 지정 실행파일을 직접 실행한다. Linux/Unix 취소는 프로세스 그룹을 종료하며 Windows 개발 취소는 직접 자식만 종료한다. 플러그인에서 daemon을 만들지 않는다.

## 릴리스·데이터·작업·스케줄

ZIP은 최대 32MiB, expanded 파일 합계는 64MiB·256개다. 절대 경로·`..`·Windows 장치명·역슬래시·중복 경로·symlink/special file·비정상 압축비를 거부한다. digest와 health 검증 후 안정된 `plugins/<id>/releases/<release-id>/` 경로를 DB에 원자적으로 활성화한다. 실패하면 기존 활성 release는 유지된다. 작업은 생성 시 release ID를 고정하므로 업데이트 중 대기/실행 작업이 바뀌지 않는다. 기존 릴리스 삭제/자동 정리는 제공하지 않는다.

영속 데이터는 release 밖 `plugin-data/<id>/`에 보관한다. 활성화·업데이트·롤백은 기존과 같은 `data_schema_version`만 허용한다. 코드 롤백이 데이터 복원을 뜻하지 않는다. 데이터 형식 변경은 서비스 중지·DB 및 plugin-data 백업·별도 검증·복구 절차가 필요하다.

SQLite 큐는 최대 대기 1000개·동시 실행 2개다. 커밋 후 wake하고 원자적으로 claim한다. `Open`만으로 실행하지 않고 serve의 `StartBackground`가 작업기/스케줄러를 시작한다. 재시작 시 queued를 복구하고 running은 interrupted로 남긴다. 정상 종료 시 실행 context를 취소하고 interrupted를 기록한다. 취소/timeout은 이미 발생한 외부 부수효과를 되돌리지 않는다. idempotent 작업의 일반 실패만 총 3회까지 2초 간격으로 재시도한다. timeout·취소·shutdown은 자동 재시도하지 않는다. 비멱등 작업은 한 번만 실행한다. 정확히 한 번의 외부 부수효과는 보장하지 않는다.

외부 제출은 요청자별 `Idempotency-Key`를 최대 200자로 보낼 수 있다. 같은 plugin/task/input은 기존 Run을 반환하며 다른 payload 재사용은 HTTP409다. 큐 포화는 HTTP429다. 비활성화는 queued 작업을 취소하되 running 작업은 명시 취소하거나 완료하도록 둔다.

스케줄은 분 단위 5필드 cron과 IANA 시간대(예. Asia/Seoul, UTC)를 사용한다. 다음 실행 시각은 코어가 계산한다. 실행 슬롯 저장·enqueue·next 갱신은 한 DB transaction이며 중복 슬롯을 거부한다. 이전 queued/running 작업이 남아 있으면 겹치는 슬롯을 건너뛰고 이력을 남긴다. 서버 중단 중 놓친 슬롯은 재생하지 않는다. 시간대 DST gap은 존재하는 다음 local time으로 이동한다.

## 검증 범위

자동 테스트는 GitHub 응답을 내부 transport fixture로 대체하고 실제 ZIP·실행파일·프로세스·SQLite·HTTP 핸들러를 검증한다. production 설치 우회 API나 허용 host 예외는 없다. 이 저장소의 hello Release를 실제 GitHub에 게시하지 않았으므로 공개 원격 설치 성공은 별도 게시 후 확인해야 한다. 실제 NAS와 Windows 자손 프로세스 전체 종료는 검증 범위 밖이다.
