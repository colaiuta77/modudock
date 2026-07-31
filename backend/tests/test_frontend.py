# 빌드된 React 정적 파일을 FastAPI가 같은 포트에서 제공하는지 검증한다.
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.main import mount_frontend


def test_mount_frontend_serves_index(tmp_path):
    (tmp_path / "index.html").write_text("<h1>ModuDock</h1>", encoding="utf-8")
    application = FastAPI()
    mount_frontend(application, tmp_path)

    response = TestClient(application).get("/")

    assert response.status_code == 200
    assert "ModuDock" in response.text
