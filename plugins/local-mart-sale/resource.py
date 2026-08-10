# 할인 행사 초안·게시·조회 요청을 플러그인 전용 SQLite에서 처리한다.
import json
import os
import re
import sqlite3
import sys
import uuid
from collections.abc import Iterator
from contextlib import contextmanager
from datetime import UTC, date, datetime, timedelta, timezone
from pathlib import Path
from typing import Any


class ResponseComplete(Exception):
    pass


def now() -> str:
    return datetime.now(UTC).isoformat()


def response(data: Any, status: int = 200, meta: dict[str, Any] | None = None) -> None:
    payload: dict[str, Any] = {"status": status, "data": data}
    if meta:
        payload["_modudock"] = meta
    print(json.dumps(payload, ensure_ascii=False))
    raise ResponseComplete


def fail(message: str, status: int = 400) -> None:
    response({"detail": message}, status)


def settings() -> dict[str, Any]:
    try:
        return json.loads(os.environ.get("MODUDOCK_PLUGIN_SETTINGS", "{}"))
    except json.JSONDecodeError:
        return {}


def database_path() -> Path:
    root = os.environ.get("MODUDOCK_PLUGIN_DATA_DIR")
    if not root:
        raise RuntimeError("MODUDOCK_PLUGIN_DATA_DIR가 설정되지 않았습니다")
    path = Path(root)
    path.mkdir(parents=True, exist_ok=True)
    return path / "sales.db"


@contextmanager
def connect() -> Iterator[sqlite3.Connection]:
    db = sqlite3.connect(database_path(), timeout=5)
    db.row_factory = sqlite3.Row
    db.execute("PRAGMA journal_mode=WAL")
    db.execute("PRAGMA busy_timeout=5000")
    db.execute("PRAGMA foreign_keys=ON")
    migrate(db)
    try:
        yield db
        db.commit()
    except ResponseComplete:
        db.commit()
        raise
    except Exception:
        db.rollback()
        raise
    finally:
        db.close()


def migrate(db: sqlite3.Connection) -> None:
    db.executescript(
        """
        CREATE TABLE IF NOT EXISTS schema_version (
            version INTEGER PRIMARY KEY
        );
        INSERT OR IGNORE INTO schema_version(version) VALUES (1);

        CREATE TABLE IF NOT EXISTS sale_batches (
            id TEXT PRIMARY KEY,
            store_id TEXT NOT NULL,
            sale_date TEXT NOT NULL,
            title TEXT NOT NULL,
            raw_text TEXT NOT NULL DEFAULT '',
            status TEXT NOT NULL CHECK(status IN ('draft', 'published', 'expired')),
            version INTEGER NOT NULL DEFAULT 0,
            created_by TEXT,
            created_at TEXT NOT NULL,
            updated_at TEXT NOT NULL,
            published_at TEXT
        );
        CREATE INDEX IF NOT EXISTS ix_sale_batches_date
            ON sale_batches(store_id, sale_date, status);

        CREATE TABLE IF NOT EXISTS sale_items (
            id TEXT PRIMARY KEY,
            batch_id TEXT NOT NULL REFERENCES sale_batches(id) ON DELETE CASCADE,
            name TEXT NOT NULL,
            regular_price INTEGER,
            sale_price INTEGER NOT NULL,
            unit TEXT NOT NULL DEFAULT '',
            note TEXT NOT NULL DEFAULT '',
            display_order INTEGER NOT NULL
        );
        CREATE INDEX IF NOT EXISTS ix_sale_items_batch
            ON sale_items(batch_id, display_order);

        CREATE TABLE IF NOT EXISTS push_deliveries (
            id TEXT PRIMARY KEY,
            batch_id TEXT NOT NULL REFERENCES sale_batches(id) ON DELETE CASCADE,
            version INTEGER NOT NULL,
            topic TEXT NOT NULL,
            status TEXT NOT NULL,
            fcm_message_id TEXT,
            error TEXT,
            sent_at TEXT NOT NULL,
            UNIQUE(batch_id, version)
        );
        """
    )


def parse_price(value: Any, field: str, required: bool = True) -> int | None:
    if value is None or str(value).strip() == "":
        if required:
            raise ValueError(f"{field}을 입력해 주세요")
        return None
    cleaned = re.sub(r"[\s,원₩]", "", str(value))
    if not cleaned.isdigit():
        raise ValueError(f"{field}은 0 이상의 원 단위 숫자여야 합니다")
    price = int(cleaned)
    if price > 1_000_000_000:
        raise ValueError(f"{field}이 허용 범위를 초과했습니다")
    return price


def validate_items(raw_items: Any) -> list[dict[str, Any]]:
    if raw_items is None or raw_items == []:
        return []
    if not isinstance(raw_items, list) or len(raw_items) > 200:
        fail("할인 품목은 200개 이하로 입력해 주세요")
    items: list[dict[str, Any]] = []
    for index, item in enumerate(raw_items):
        if not isinstance(item, dict):
            fail(f"{index + 1}번째 품목 형식이 올바르지 않습니다")
        name = str(item.get("name", "")).strip()
        if not name or len(name) > 120:
            fail(f"{index + 1}번째 품목명을 확인해 주세요")
        try:
            regular_price = parse_price(item.get("regular_price"), "정상가", required=False)
            sale_price = parse_price(item.get("sale_price"), "할인가")
        except ValueError as exc:
            fail(f"{index + 1}번째 품목의 {exc}")
        items.append(
            {
                "id": str(item.get("id") or uuid.uuid4()),
                "name": name,
                "regular_price": regular_price,
                "sale_price": sale_price,
                "unit": str(item.get("unit", "")).strip()[:80],
                "note": str(item.get("note", "")).strip()[:300],
                "display_order": index,
            }
        )
    return items


def parse_text(raw_text: str) -> list[dict[str, Any]]:
    if len(raw_text) > 50_000:
        fail("붙여넣기 텍스트는 50,000자 이하여야 합니다")
    items: list[dict[str, Any]] = []
    errors: list[str] = []
    for line_number, source in enumerate(raw_text.splitlines(), 1):
        line = source.strip()
        if not line:
            continue
        parts = [part.strip() for part in line.split("|")]
        if len(parts) < 3 or len(parts) > 5:
            errors.append(f"{line_number}행은 품목명 | 정상가 | 할인가 | 단위 | 설명 형식이어야 합니다")
            continue
        while len(parts) < 5:
            parts.append("")
        try:
            regular_price = parse_price(parts[1], "정상가", required=False)
            sale_price = parse_price(parts[2], "할인가")
        except ValueError:
            errors.append(f"{line_number}행의 가격 형식이 올바르지 않습니다")
            continue
        if not parts[0]:
            errors.append(f"{line_number}행의 품목명이 비어 있습니다")
            continue
        items.append(
            {
                "id": str(uuid.uuid4()),
                "name": parts[0][:120],
                "regular_price": regular_price,
                "sale_price": sale_price,
                "unit": parts[3][:80],
                "note": parts[4][:300],
            }
        )
    if errors:
        fail("\n".join(errors[:10]))
    if not items:
        fail("변환할 할인 품목이 없습니다")
    return items


def item_rows(db: sqlite3.Connection, batch_id: str) -> list[dict[str, Any]]:
    rows = db.execute(
        """
        SELECT id, name, regular_price, sale_price, unit, note, display_order
        FROM sale_items WHERE batch_id = ? ORDER BY display_order
        """,
        (batch_id,),
    ).fetchall()
    return [dict(row) for row in rows]


def delivery_row(db: sqlite3.Connection, batch_id: str, version: int) -> dict[str, Any] | None:
    row = db.execute(
        """
        SELECT status, topic, fcm_message_id, error, sent_at
        FROM push_deliveries WHERE batch_id = ? AND version = ?
        """,
        (batch_id, version),
    ).fetchone()
    return dict(row) if row else None


def batch_view(db: sqlite3.Connection, row: sqlite3.Row) -> dict[str, Any]:
    result = dict(row)
    result["items"] = item_rows(db, row["id"])
    result["item_count"] = len(result["items"])
    result["push_delivery"] = delivery_row(db, row["id"], row["version"])
    return result


def public_batch_view(db: sqlite3.Connection, row: sqlite3.Row) -> dict[str, Any]:
    return {
        "id": row["id"],
        "store_id": row["store_id"],
        "sale_date": row["sale_date"],
        "title": row["title"],
        "version": row["version"],
        "published_at": row["published_at"],
        "content_text": row["raw_text"],
        "items": item_rows(db, row["id"]),
    }


def find_batch(db: sqlite3.Connection, batch_id: str) -> sqlite3.Row:
    row = db.execute("SELECT * FROM sale_batches WHERE id = ?", (batch_id,)).fetchone()
    if row is None:
        fail("할인 행사를 찾을 수 없습니다", 404)
    return row


def list_batches(db: sqlite3.Connection, query: dict[str, str]) -> None:
    store_id = query.get("store_id") or settings().get("store_id", "main")
    sale_date = query.get("sale_date")
    sql = "SELECT * FROM sale_batches WHERE store_id = ?"
    parameters: list[Any] = [store_id]
    if sale_date:
        sql += " AND sale_date = ?"
        parameters.append(sale_date)
    sql += " ORDER BY sale_date DESC, created_at DESC LIMIT 100"
    response([batch_view(db, row) for row in db.execute(sql, parameters).fetchall()])


def save_items(db: sqlite3.Connection, batch_id: str, items: list[dict[str, Any]]) -> None:
    db.execute("DELETE FROM sale_items WHERE batch_id = ?", (batch_id,))
    db.executemany(
        """
        INSERT INTO sale_items(
            id, batch_id, name, regular_price, sale_price, unit, note, display_order
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        """,
        [
            (
                item["id"],
                batch_id,
                item["name"],
                item["regular_price"],
                item["sale_price"],
                item["unit"],
                item["note"],
                item["display_order"],
            )
            for item in items
        ],
    )


def validate_batch(body: Any) -> tuple[str, str, str, str, list[dict[str, Any]]]:
    if not isinstance(body, dict):
        fail("요청 본문 형식이 올바르지 않습니다")
    configured = settings()
    store_id = str(body.get("store_id") or configured.get("store_id", "main")).strip()
    sale_date = str(body.get("sale_date", "")).strip()
    title = str(body.get("title", "")).strip()
    raw_text = str(body.get("raw_text", "")).replace("\r\n", "\n").replace("\r", "\n")
    if not store_id or len(store_id) > 80:
        fail("매장 ID를 확인해 주세요")
    try:
        date.fromisoformat(sale_date)
    except ValueError:
        fail("행사 날짜는 YYYY-MM-DD 형식이어야 합니다")
    if not title or len(title) > 160:
        fail("제목은 1자 이상 160자 이하로 입력해 주세요")
    if not raw_text.strip():
        fail("세일 원문을 입력하거나 TXT 파일을 선택해 주세요")
    if len(raw_text) > 50_000:
        fail("세일 원문은 50,000자 이하여야 합니다")
    return store_id, sale_date, title, raw_text, validate_items(body.get("items"))


def create_batch(db: sqlite3.Connection, body: Any, user_id: str | None) -> None:
    store_id, sale_date, title, raw_text, items = validate_batch(body)
    batch_id = str(uuid.uuid4())
    timestamp = now()
    db.execute(
        """
        INSERT INTO sale_batches(
            id, store_id, sale_date, title, raw_text, status, version,
            created_by, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, 'draft', 0, ?, ?, ?)
        """,
        (batch_id, store_id, sale_date, title, raw_text, user_id, timestamp, timestamp),
    )
    save_items(db, batch_id, items)
    response(batch_view(db, find_batch(db, batch_id)), 201)


def update_batch(db: sqlite3.Connection, batch_id: str, body: Any) -> None:
    current = find_batch(db, batch_id)
    if current["status"] == "expired":
        fail("종료된 행사는 변경할 수 없습니다", 409)
    store_id, sale_date, title, raw_text, items = validate_batch(body)
    db.execute(
        """
        UPDATE sale_batches
        SET store_id = ?, sale_date = ?, title = ?, raw_text = ?,
            status = 'draft', updated_at = ?
        WHERE id = ?
        """,
        (store_id, sale_date, title, raw_text, now(), batch_id),
    )
    save_items(db, batch_id, items)
    response(batch_view(db, find_batch(db, batch_id)))


def publish_batch(db: sqlite3.Connection, batch_id: str) -> None:
    current = find_batch(db, batch_id)
    if not current["raw_text"].strip() and not item_rows(db, batch_id):
        fail("게시할 세일 원문이 없습니다", 409)
    next_version = current["version"] + 1
    timestamp = now()
    db.execute(
        """
        UPDATE sale_batches SET status = 'expired', updated_at = ?
        WHERE store_id = ? AND sale_date = ? AND status = 'published' AND id <> ?
        """,
        (timestamp, current["store_id"], current["sale_date"], batch_id),
    )
    db.execute(
        """
        UPDATE sale_batches
        SET status = 'published', version = ?, published_at = ?, updated_at = ?
        WHERE id = ?
        """,
        (next_version, timestamp, timestamp, batch_id),
    )
    result = batch_view(db, find_batch(db, batch_id))
    response(
        result,
        meta={
            "enqueue_task": {
                "task": "send-notification",
                "input": {"sale_batch_id": batch_id, "version": next_version},
            }
        },
    )


def delete_batch(db: sqlite3.Connection, batch_id: str) -> None:
    current = find_batch(db, batch_id)
    if current["status"] != "draft":
        fail("초안만 삭제할 수 있습니다", 409)
    db.execute("DELETE FROM sale_batches WHERE id = ?", (batch_id,))
    response(None, 204)


def public_today(db: sqlite3.Connection, query: dict[str, str]) -> None:
    configured = settings()
    store_id = query.get("store_id") or configured.get("store_id", "main")
    korea = timezone(timedelta(hours=9))
    sale_date = query.get("date") or datetime.now(korea).date().isoformat()
    row = db.execute(
        """
        SELECT * FROM sale_batches
        WHERE store_id = ? AND sale_date = ? AND status = 'published'
        ORDER BY published_at DESC LIMIT 1
        """,
        (store_id, sale_date),
    ).fetchone()
    if row is None:
        fail("게시된 당일 할인 행사가 없습니다", 404)
    result = public_batch_view(db, row)
    result["store_name"] = configured.get("store_name", "우리동네마트")
    response(result)


def dispatch(request: dict[str, Any]) -> None:
    method = str(request.get("method", "")).upper()
    path = str(request.get("path", "/"))
    query = request.get("query") if isinstance(request.get("query"), dict) else {}
    body = request.get("body")
    context = request.get("context") if isinstance(request.get("context"), dict) else {}
    public = bool(context.get("public"))
    segments = [segment for segment in path.split("/") if segment]

    with connect() as db:
        if public:
            if method == "GET" and segments == ["today"]:
                public_today(db, query)
            if method == "GET" and len(segments) == 2 and segments[0] == "batches":
                row = find_batch(db, segments[1])
                if row["status"] != "published":
                    fail("게시된 할인 행사를 찾을 수 없습니다", 404)
                response(public_batch_view(db, row))
            fail("공개 리소스를 찾을 수 없습니다", 404)

        if method == "POST" and segments == ["parse"]:
            raw_text = str(body.get("raw_text", "")) if isinstance(body, dict) else ""
            response({"items": parse_text(raw_text)})
        if method == "GET" and segments == ["batches"]:
            list_batches(db, query)
        if method == "POST" and segments == ["batches"]:
            create_batch(db, body, context.get("user_id"))
        if len(segments) == 2 and segments[0] == "batches":
            if method == "GET":
                response(batch_view(db, find_batch(db, segments[1])))
            if method == "PUT":
                update_batch(db, segments[1], body)
            if method == "DELETE":
                delete_batch(db, segments[1])
        if method == "POST" and len(segments) == 3 and segments[0] == "batches" and segments[2] == "publish":
            publish_batch(db, segments[1])
        fail("플러그인 리소스를 찾을 수 없습니다", 404)


if __name__ == "__main__":
    try:
        dispatch(json.load(sys.stdin))
    except ResponseComplete:
        pass
    except (BrokenPipeError, KeyboardInterrupt):
        raise SystemExit(1) from None
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        raise SystemExit(1) from exc
