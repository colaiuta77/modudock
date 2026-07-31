# 인증과 보호된 API의 기본 동작을 검증한다.
def test_health(client):
    assert client.get("/healthz").json() == {"status": "ok"}


def test_login_rejects_wrong_password(client):
    response = client.post("/api/auth/token", json={"username": "admin", "password": "wrong"})
    assert response.status_code == 401


def test_empty_plugin_list_requires_authentication(client, token):
    assert client.get("/api/plugins").status_code == 401
    response = client.get("/api/plugins", headers={"Authorization": f"Bearer {token}"})
    assert response.status_code == 200
    assert response.json() == []


def test_install_rejects_unsafe_git_transport(client, token):
    response = client.post(
        "/api/plugins/install",
        headers={"Authorization": f"Bearer {token}"},
        json={"repository_url": "ext::sh -c evil", "ref": "HEAD"},
    )
    assert response.status_code == 422
