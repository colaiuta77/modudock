# local-mart-sale

동네마트의 당일 할인 행사를 ModuDock에서 작성·게시하고 Android 앱에 제공하는 공식 번들 플러그인입니다.

## 관리자 사용

`오늘의 세일` 메뉴의 `세일 작성` 탭에서 행사 날짜와 제목을 입력하고, Android 앱에 보여 줄 세일 원문을 직접 작성하거나 `.txt` 파일에서 불러옵니다. TXT 파일은 UTF-8과 EUC-KR 인코딩을 지원합니다.

```text
★한라마트 특급세일★

○야채
- 파프리카 3개  3,980
- 백오이 5개  3,980

○청과
- 딸기 특 1팩  7,900
```

기호, 공백과 줄바꿈은 별도 변환 없이 원문 그대로 저장되고 공개 API의 `content_text`로 전달됩니다. 입력 내용은 50,000자까지 허용합니다. 초안을 저장한 다음 게시하며, 이전 글은 `게시 이력` 탭의 게시판형 목록에서 확인합니다. 매장과 FCM 정보는 `매장·알림 설정` 탭에서 관리합니다.

같은 매장과 날짜에 새 행사를 게시하면 기존 게시 행사는 `expired` 상태로 전환됩니다. 게시된 행사를 수정하면 초안 상태로 돌아가며 다시 게시할 때 버전이 증가합니다.

## Android 공개 API

오늘의 행사 조회입니다.

```http
GET /api/public/plugins/local-mart-sale/resources/sales/today?store_id=main
```

테스트나 과거 날짜 확인에는 `date=YYYY-MM-DD`를 전달할 수 있습니다.

```json
{
  "id": "f5cd9ce5-38e4-4c15-bdb3-365fb22cc79b",
  "store_id": "main",
  "store_name": "우리동네마트",
  "sale_date": "2026-07-31",
  "title": "오늘의 할인",
  "version": 1,
  "published_at": "2026-07-31T05:00:00+00:00",
  "content_text": "★한라마트 특급세일★\n\n○야채\n- 파프리카 3개  3,980\n- 백오이 5개  3,980",
  "items": []
}
```

응답이 `404`이면 해당 날짜에 게시된 행사가 없는 상태입니다. 앱은 `content_text`를 줄바꿈이 보존되는 텍스트 컴포넌트에 표시하고, 응답의 `version`과 `published_at`을 캐시 갱신 기준으로 사용할 수 있습니다. `items`는 이전 구조화 연동과의 호환을 위해 유지되는 필드입니다.

특정 게시 행사 조회입니다.

```http
GET /api/public/plugins/local-mart-sale/resources/sales/batches/{sale_batch_id}
```

초안과 종료된 행사는 공개 API에서 조회할 수 없습니다.

## FCM 데이터 계약

Android 앱은 설정 화면의 `fcm_topic`을 구독합니다. 기본값은 `mart-main`입니다. 게시 알림의 data payload는 다음과 같습니다.

```json
{
  "type": "daily_sale_published",
  "store_id": "main",
  "sale_batch_id": "f5cd9ce5-38e4-4c15-bdb3-365fb22cc79b",
  "version": "1",
  "sale_date": "2026-07-31"
}
```

앱은 알림 내용을 기준 데이터로 저장하지 않고 `sale_batch_id` 또는 오늘의 행사 API로 최신 정보를 다시 조회해야 합니다.

FCM을 켜지 않은 기본 상태에서는 게시 작업이 성공하며 `push_deliveries`에 `skipped`가 기록됩니다. FCM을 사용하려면 다음 설정이 필요합니다.

- `fcm_enabled`를 활성화합니다.
- Android 앱이 구독한 `fcm_topic`을 입력합니다.
- Firebase 서비스 계정 JSON을 API와 worker 컨테이너에 동일한 읽기 전용 경로로 마운트합니다.
- `fcm_service_account_file`에는 `/run/secrets/firebase.json` 같은 컨테이너 내부 경로를 입력합니다.

서비스 계정 JSON 내용은 ModuDock 설정, Git 저장소 또는 플러그인 SQLite에 저장하지 않습니다.

## 데이터

SQLite 파일은 `/plugin-data/local-mart-sale/sales.db`에 생성됩니다. WAL, 5초 busy timeout과 짧은 쓰기 transaction을 사용합니다. 플러그인 릴리스 업데이트나 롤백과 별개의 Docker volume에 보존됩니다.
