# 전체 로그 API의 레벨과 메시지 검색 필터를 검증한다.
from app.db import SessionLocal
from app.logging_service import write_platform_log
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


def test_platform_log_includes_component(client, token):
    write_platform_log("scheduler", "ModuDock scheduler를 시작했습니다")

    response = client.get(
        "/api/logs?query=scheduler",
        headers={"Authorization": f"Bearer {token}"},
    )

    assert response.status_code == 200
    assert response.json()[0]["context"]["component"] == "scheduler"


def test_expected_http_error_does_not_fill_platform_log(client, token):
    missing = client.get(
        "/api/plugins/missing/manifest",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert missing.status_code == 404

    response = client.get(
        "/api/logs?query=HTTP 요청",
        headers={"Authorization": f"Bearer {token}"},
    )

    assert response.status_code == 200
    assert response.json() == []
