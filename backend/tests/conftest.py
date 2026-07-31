# 테스트마다 격리된 SQLite 데이터베이스와 관리자 계정을 준비한다.
import os
import tempfile
from pathlib import Path

TEST_ROOT = Path(tempfile.mkdtemp(prefix="modudock-tests-"))
os.environ["MODUDOCK_DATABASE_URL"] = f"sqlite:///{(TEST_ROOT / 'test.db').as_posix()}"
os.environ["MODUDOCK_SECRET_KEY"] = "test-secret-key-at-least-32-bytes-long"
os.environ["MODUDOCK_ADMIN_PASSWORD"] = "test-password"
os.environ["MODUDOCK_PLUGIN_ROOT"] = str(TEST_ROOT / "plugins")

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import delete

from app.db import Base, SessionLocal, engine
from app.main import app
from app.models import User
from app.security import hash_password


@pytest.fixture(autouse=True)
def database():
    Base.metadata.create_all(engine)
    with SessionLocal.begin() as db:
        db.execute(delete(User))
        db.add(User(username="admin", password_hash=hash_password("test-password"), role="admin"))
    yield
    Base.metadata.drop_all(engine)


@pytest.fixture
def client() -> TestClient:
    return TestClient(app)


@pytest.fixture
def token(client: TestClient) -> str:
    response = client.post(
        "/api/auth/token",
        json={"username": "admin", "password": "test-password"},
    )
    assert response.status_code == 200
    return response.json()["access_token"]
