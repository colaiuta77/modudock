# 환경 변수에서 ModuDock 런타임 설정을 읽는다.
from functools import lru_cache
from pathlib import Path
from typing import Annotated

from pydantic import field_validator
from pydantic_settings import BaseSettings, NoDecode, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="MODUDOCK_", env_file=".env", extra="ignore")

    secret_key: str = "development-only-change-me"
    admin_username: str = "admin"
    admin_password: str = "admin"
    database_url: str = "sqlite:///./data/modudock.db"
    redis_url: str = "redis://localhost:6379/0"
    plugin_root: Path = Path("./plugins")
    cors_origins: Annotated[list[str], NoDecode] = [
        "http://localhost:5173",
        "http://localhost:7438",
    ]
    access_token_minutes: int = 60
    scheduler_poll_seconds: int = 5
    scheduler_lease_seconds: int = 20

    @field_validator("cors_origins", mode="before")
    @classmethod
    def split_origins(cls, value: object) -> object:
        if isinstance(value, str):
            return [item.strip() for item in value.split(",") if item.strip()]
        return value


@lru_cache
def get_settings() -> Settings:
    return Settings()


settings = get_settings()
