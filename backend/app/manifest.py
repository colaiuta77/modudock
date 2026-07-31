# 플러그인 선언 파일의 호환성과 실행 명세를 검증한다.
import re
from pathlib import Path
from typing import Any

import yaml
from pydantic import BaseModel, Field, field_validator

PLUGIN_ID = re.compile(r"^[a-z][a-z0-9-]{1,62}$")


class TaskDeclaration(BaseModel):
    id: str
    title: str
    command: list[str]
    input_schema: dict[str, Any] = Field(default_factory=dict)

    @field_validator("command")
    @classmethod
    def command_must_be_argv(cls, value: list[str]) -> list[str]:
        if not value or any(not part for part in value):
            raise ValueError("command는 비어 있지 않은 argv 배열이어야 합니다")
        return value


class HealthCheck(BaseModel):
    command: list[str]
    timeout_seconds: int = Field(default=30, ge=1, le=300)


class PluginManifest(BaseModel):
    schema_version: int = 1
    id: str
    name: str
    version: str
    platform: str = ">=0.1,<1"
    tasks: list[TaskDeclaration] = Field(default_factory=list)
    menus: list[dict[str, Any]] = Field(default_factory=list)
    settings_schema: dict[str, Any] = Field(default_factory=dict)
    schedules: list[dict[str, Any]] = Field(default_factory=list)
    permissions: list[str] = Field(default_factory=list)
    dependencies: list[str] = Field(default_factory=list)
    health_check: HealthCheck | None = None

    @field_validator("id")
    @classmethod
    def valid_id(cls, value: str) -> str:
        if not PLUGIN_ID.fullmatch(value):
            raise ValueError("플러그인 ID 형식이 올바르지 않습니다")
        return value


def load_manifest(release_dir: Path) -> PluginManifest:
    path = release_dir / "modudock.yaml"
    if not path.is_file():
        raise ValueError("modudock.yaml이 없습니다")
    raw = yaml.safe_load(path.read_text(encoding="utf-8"))
    return PluginManifest.model_validate(raw)
