# 전체 로그 API의 레벨과 메시지 검색 필터를 검증한다.
from app.db import SessionLocal
from app.models import LogEntry


def test_logs_filter_by_level_and_query(client, token):
    with SessionLocal.begin() as db:
        db.add(LogEntry(level="INFO", message="scheduler started"))
        db.add(LogEntry(level="ERROR", message="plugin execution failed"))

    response = client.get(
        "/api/logs?level=ERROR&query=execution",
        headers={"Authorization": f"Bearer {token}"},
    )

    assert response.status_code == 200
    assert [entry["message"] for entry in response.json()] == ["plugin execution failed"]
