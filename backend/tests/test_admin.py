# 사용자 관리와 비밀번호 변경 API의 권한 및 보호 규칙을 검증한다.
from app.db import SessionLocal
from app.models import Role, User
from app.security import verify_password


def auth(token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {token}"}


def test_admin_can_create_update_reset_and_disable_user(client, token):
    created = client.post(
        "/api/users",
        headers=auth(token),
        json={"username": "member", "password": "initial-password", "role": "viewer"},
    )
    assert created.status_code == 201
    user_id = created.json()["id"]

    updated = client.put(
        f"/api/users/{user_id}",
        headers=auth(token),
        json={"username": "member-renamed", "role": "admin", "is_active": True},
    )
    assert updated.status_code == 200
    assert updated.json()["role"] == "admin"

    reset = client.put(
        f"/api/users/{user_id}/password",
        headers=auth(token),
        json={"new_password": "replacement-password"},
    )
    assert reset.status_code == 204
    with SessionLocal() as db:
        stored = db.get(User, user_id)
        assert verify_password("replacement-password", stored.password_hash)

    deleted = client.delete(f"/api/users/{user_id}", headers=auth(token))
    assert deleted.status_code == 204
    with SessionLocal() as db:
        assert db.get(User, user_id).is_active is False


def test_admin_cannot_delete_self_or_remove_last_admin(client, token):
    me = client.get("/api/auth/me", headers=auth(token)).json()

    assert client.delete(f"/api/users/{me['id']}", headers=auth(token)).status_code == 409
    response = client.put(
        f"/api/users/{me['id']}",
        headers=auth(token),
        json={"username": me["username"], "role": "viewer", "is_active": True},
    )
    assert response.status_code == 409


def test_user_can_change_own_password(client, token):
    wrong = client.put(
        "/api/auth/password",
        headers=auth(token),
        json={"current_password": "wrong-password", "new_password": "new-password-123"},
    )
    assert wrong.status_code == 400

    changed = client.put(
        "/api/auth/password",
        headers=auth(token),
        json={"current_password": "test-password", "new_password": "new-password-123"},
    )
    assert changed.status_code == 204
    assert (
        client.post(
            "/api/auth/token",
            json={"username": "admin", "password": "new-password-123"},
        ).status_code
        == 200
    )


def test_inactive_user_cannot_login(client):
    with SessionLocal.begin() as db:
        from app.security import hash_password

        db.add(
            User(
                username="disabled",
                password_hash=hash_password("disabled-password"),
                role=Role.viewer,
                is_active=False,
            )
        )
    response = client.post(
        "/api/auth/token",
        json={"username": "disabled", "password": "disabled-password"},
    )
    assert response.status_code == 401
