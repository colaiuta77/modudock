# local-mart-sale

동네마트의 당일 할인 행사를 ModuDock에서 작성·게시하고 Android 앱에 제공하는 공식 번들 플러그인입니다.

## 관리자 사용

`오늘의 세일` 메뉴에서 행사 날짜와 제목을 입력하고 다음 형식의 품목을 붙여넣습니다.

```text
사과 | 3,000 | 1,990 | 1봉 | 한정 수량
대패삼겹살 | 12,000 | 8,900 | 600g
우유 | 2,800 | 1,980 | 1L | 1인 2개 한정
```

열 순서는 `품목명 | 정상가 | 할인가 | 단위 | 설명`입니다. 단위와 설명은 생략할 수 있고 정상가는 빈 값으로 둘 수 있습니다. `텍스트 변환` 후 표에서 내용을 검토하고 초안을 저장한 다음 게시합니다.

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
  "items": [
    {
      "id": "76340412-37bd-439d-a033-697481242b4c",
      "name": "사과",
      "regular_price": 3000,
      "sale_price": 1990,
      "unit": "1봉",
      "note": "한정 수량",
      "display_order": 0
    }
  ]
}
```

응답이 `404`이면 해당 날짜에 게시된 행사가 없는 상태입니다. 앱은 응답의 `version`과 `published_at`을 캐시 갱신 기준으로 사용할 수 있습니다.

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
