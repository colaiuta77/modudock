# 게시된 할인 행사의 Firebase 주제 알림을 보내고 발송 이력을 기록한다.
import json
import os
import sqlite3
import sys
import uuid
from datetime import UTC, datetime
from pathlib import Path
from typing import Any


def configured_settings() -> dict[str, Any]:
    return json.loads(os.environ.get("MODUDOCK_PLUGIN_SETTINGS", "{}"))


def database_path() -> Path:
    return Path(os.environ["MODUDOCK_PLUGIN_DATA_DIR"]) / "sales.db"


def record_delivery(
    db: sqlite3.Connection,
    batch_id: str,
    version: int,
    topic: str,
    status: str,
    message_id: str | None = None,
    error: str | None = None,
) -> None:
    db.execute(
        """
        INSERT INTO push_deliveries(
            id, batch_id, version, topic, status, fcm_message_id, error, sent_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(batch_id, version) DO UPDATE SET
            topic = excluded.topic,
            status = excluded.status,
            fcm_message_id = excluded.fcm_message_id,
            error = excluded.error,
            sent_at = excluded.sent_at
        """,
        (
            str(uuid.uuid4()),
            batch_id,
            version,
            topic,
            status,
            message_id,
            error,
            datetime.now(UTC).isoformat(),
        ),
    )


def main(payload: dict[str, Any]) -> dict[str, Any]:
    batch_id = str(payload["sale_batch_id"])
    version = int(payload["version"])
    settings = configured_settings()
    topic = str(settings.get("fcm_topic") or "mart-main")
    with sqlite3.connect(database_path(), timeout=5) as db:
        db.row_factory = sqlite3.Row
        db.execute("PRAGMA busy_timeout=5000")
        batch = db.execute(
            "SELECT id, store_id, sale_date, title, status, version FROM sale_batches WHERE id = ?",
            (batch_id,),
        ).fetchone()
        if batch is None or batch["status"] != "published" or batch["version"] != version:
            raise RuntimeError("게시 버전과 일치하는 할인 행사를 찾을 수 없습니다")
        existing = db.execute(
            "SELECT status, fcm_message_id FROM push_deliveries WHERE batch_id = ? AND version = ?",
            (batch_id, version),
        ).fetchone()
        if existing and existing["status"] == "sent":
            return {"status": "already_sent", "message_id": existing["fcm_message_id"]}
        if not settings.get("fcm_enabled", False):
            record_delivery(db, batch_id, version, topic, "skipped", error="FCM 알림이 비활성화되어 있습니다")
            return {"status": "skipped", "reason": "FCM disabled"}

        try:
            import firebase_admin
            from firebase_admin import credentials, messaging

            if not firebase_admin._apps:
                service_account_file = str(settings.get("fcm_service_account_file", "")).strip()
                credential = credentials.Certificate(service_account_file) if service_account_file else None
                firebase_admin.initialize_app(credential)
            message_id = messaging.send(
                messaging.Message(
                    topic=topic,
                    notification=messaging.Notification(
                        title=str(settings.get("store_name") or "우리동네마트"),
                        body=batch["title"],
                    ),
                    data={
                        "type": "daily_sale_published",
                        "store_id": batch["store_id"],
                        "sale_batch_id": batch_id,
                        "version": str(version),
                        "sale_date": batch["sale_date"],
                    },
                )
            )
            record_delivery(db, batch_id, version, topic, "sent", message_id=message_id)
            return {"status": "sent", "message_id": message_id}
        except Exception as exc:
            record_delivery(db, batch_id, version, topic, "failed", error=str(exc)[:1000])
            raise


if __name__ == "__main__":
    print(json.dumps(main(json.load(sys.stdin)), ensure_ascii=False))
