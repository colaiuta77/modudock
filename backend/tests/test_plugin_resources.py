# 선언된 플러그인 리소스의 관리자·공개 API와 후속 작업 연결을 검증한다.
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import pytest
from celery.exceptions import Retry
from sqlalchemy import select

from app.db import SessionLocal
from app.manifest import load_manifest
from app.models import Plugin, PluginRelease, PluginState, Role, RunState, TaskRun, User
from app.security import create_token, hash_password
from app.worker import run_plugin_task

PLUGIN_SOURCE = Path(__file__).parents[2] / "plugins" / "local-mart-sale"


def auth(token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {token}"}


def install_sale_plugin() -> None:
    manifest = load_manifest(PLUGIN_SOURCE)
    with SessionLocal.begin() as db:
        plugin = Plugin(
            id=manifest.id,
            name=manifest.name,
            repository_url="bundled://local-mart-sale",
            state=PluginState.active,
        )
        db.add(plugin)
        db.flush()
        release = PluginRelease(
            plugin_id=plugin.id,
            version=manifest.version,
            commit_sha="b" * 64,
            install_path=str(PLUGIN_SOURCE),
            manifest=manifest.model_dump(mode="json"),
            healthy=True,
        )
        db.add(release)
        db.flush()
        plugin.active_release_id = release.id


def viewer_token() -> str:
    with SessionLocal.begin() as db:
        viewer = User(
            username="sale-viewer",
            password_hash=hash_password("viewer-password"),
            role=Role.viewer,
        )
        db.add(viewer)
        db.flush()
        return create_token(viewer)


def assert_committed_before_enqueue(run_id: str) -> SimpleNamespace:
    with SessionLocal() as db:
        run = db.get(TaskRun, run_id)
        assert run is not None
        assert run.state == RunState.queued
    return SimpleNamespace(id="celery-task")


def test_sale_resource_draft_publish_and_public_read(client, token):
    install_sale_plugin()
    headers = auth(token)
    content_text = "★한라마트 특급세일★\r\n\r\n○야채\r\n- 파프리카 3개  3,980"

    created = client.post(
        "/api/plugins/local-mart-sale/resources/sales/batches",
        headers=headers,
        json={
            "store_id": "main",
            "sale_date": "2026-07-31",
            "title": "오늘의 할인",
            "raw_text": content_text,
        },
    )
    assert created.status_code == 201
    batch_id = created.json()["id"]
    assert created.json()["status"] == "draft"
    assert created.json()["raw_text"] == content_text.replace("\r\n", "\n")
    assert created.json()["items"] == []

    with patch("app.main.run_plugin_task.delay", side_effect=assert_committed_before_enqueue) as delay:
        published = client.post(
            f"/api/plugins/local-mart-sale/resources/sales/batches/{batch_id}/publish",
            headers=headers,
        )
    assert published.status_code == 200
    assert published.json()["status"] == "published"
    delay.assert_called_once()

    public = client.get(
        "/api/public/plugins/local-mart-sale/resources/sales/today",
        params={"date": "2026-07-31", "store_id": "main"},
    )
    assert public.status_code == 200
    assert public.json()["content_text"] == content_text.replace("\r\n", "\n")
    assert public.json()["items"] == []


def test_manual_plugin_task_is_committed_before_enqueue(client, token):
    install_sale_plugin()
    with patch("app.main.run_plugin_task.delay", side_effect=assert_committed_before_enqueue):
        response = client.post(
            "/api/plugins/local-mart-sale/tasks/send-notification/runs",
            headers=auth(token),
            json={"input": {"sale_batch_id": "batch-id", "version": 1}},
        )
    assert response.status_code == 202
    assert response.json()["state"] == "queued"
    with SessionLocal() as db:
        run = db.scalar(select(TaskRun).order_by(TaskRun.created_at.desc()))
        assert run is not None
        assert run.celery_task_id == "celery-task"


def test_plugin_task_enqueue_failure_is_recorded(client, token):
    install_sale_plugin()
    with patch("app.main.run_plugin_task.delay", side_effect=RuntimeError("broker unavailable")):
        response = client.post(
            "/api/plugins/local-mart-sale/tasks/send-notification/runs",
            headers=auth(token),
            json={"input": {"sale_batch_id": "batch-id", "version": 1}},
        )
    assert response.status_code == 503
    with SessionLocal() as db:
        run = db.scalar(select(TaskRun).order_by(TaskRun.created_at.desc()))
        assert run is not None
        assert run.state == RunState.failed
        assert run.error == "작업 대기열 전송 실패: broker unavailable"


def test_worker_retries_when_run_is_not_visible():
    with (
        patch.object(run_plugin_task, "retry", side_effect=Retry()) as retry,
        pytest.raises(Retry),
    ):
        run_plugin_task.run("missing-run-id")
    retry.assert_called_once()
    assert retry.call_args.kwargs["countdown"] == 1
    assert retry.call_args.kwargs["max_retries"] == 3


def test_admin_resource_requires_authentication(client):
    response = client.get("/api/plugins/local-mart-sale/resources/sales/batches")
    assert response.status_code == 401


def test_viewer_can_read_but_cannot_change_resource(client):
    install_sale_plugin()
    headers = auth(viewer_token())
    assert client.get(
        "/api/plugins/local-mart-sale/resources/sales/batches",
        headers=headers,
    ).status_code == 200
    assert client.post(
        "/api/plugins/local-mart-sale/resources/sales/parse",
        headers=headers,
        json={"raw_text": "사과 | 3000 | 1990"},
    ).status_code == 403


def test_public_resource_rejects_unpublished_batch(client, token):
    install_sale_plugin()
    created = client.post(
        "/api/plugins/local-mart-sale/resources/sales/batches",
        headers=auth(token),
        json={
            "store_id": "main",
            "sale_date": "2026-08-01",
            "title": "초안",
            "raw_text": "○유제품\n- 우유 1L  1,900",
        },
    )
    response = client.get(
        f"/api/public/plugins/local-mart-sale/resources/sales/batches/{created.json()['id']}"
    )
    assert response.status_code == 404


def test_sale_resource_rejects_empty_content(client, token):
    install_sale_plugin()
    response = client.post(
        "/api/plugins/local-mart-sale/resources/sales/batches",
        headers=auth(token),
        json={
            "store_id": "main",
            "sale_date": "2026-08-10",
            "title": "내용 없는 초안",
            "raw_text": "  \n  ",
        },
    )
    assert response.status_code == 400
    assert response.json()["detail"] == "세일 원문을 입력하거나 TXT 파일을 선택해 주세요"
