# 플러그인 manifest 메뉴의 기본값과 관리자 편집 저장을 검증한다.
from app.db import SessionLocal
from app.models import Plugin, PluginRelease, PluginState


def auth(token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {token}"}


def install_menu_fixture() -> None:
    with SessionLocal.begin() as db:
        plugin = Plugin(
            id="menu-plugin",
            name="메뉴 플러그인",
            repository_url="https://example.com/plugin.git",
            state=PluginState.active,
        )
        db.add(plugin)
        db.flush()
        release = PluginRelease(
            plugin_id=plugin.id,
            version="1.0.0",
            commit_sha="a" * 40,
            install_path="/plugins/menu-plugin",
            healthy=True,
            manifest={
                "id": plugin.id,
                "name": plugin.name,
                "version": "1.0.0",
                "menus": [{"id": "dashboard", "title": "플러그인 현황"}],
                "tasks": [],
            },
        )
        db.add(release)
        db.flush()
        plugin.active_release_id = release.id


def test_plugin_menu_can_be_renamed_hidden_and_reordered(client, token):
    install_menu_fixture()

    initial = client.get("/api/plugin-menus", headers=auth(token))
    assert initial.status_code == 200
    assert initial.json()[0]["title"] == "플러그인 현황"

    updated = client.put(
        "/api/plugin-menus/menu-plugin/dashboard",
        headers=auth(token),
        json={"title": "새 메뉴명", "enabled": False, "position": 30},
    )
    assert updated.status_code == 200
    assert updated.json()["title"] == "새 메뉴명"
    assert updated.json()["enabled"] is False

    saved = client.get("/api/plugin-menus", headers=auth(token)).json()[0]
    assert saved["position"] == 30
    assert saved["title"] == "새 메뉴명"
