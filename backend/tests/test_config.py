# Docker 환경 변수의 쉼표 구분 CORS 출처가 정상적으로 파싱되는지 검증한다.
from app.config import Settings


def test_cors_origins_accepts_comma_separated_env(monkeypatch):
    monkeypatch.setenv(
        "MODUDOCK_CORS_ORIGINS",
        "http://localhost:5173, http://localhost:7438",
    )

    settings = Settings(_env_file=None)

    assert settings.cors_origins == [
        "http://localhost:5173",
        "http://localhost:7438",
    ]
