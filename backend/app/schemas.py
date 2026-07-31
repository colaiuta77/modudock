# API 요청과 응답의 검증 스키마를 정의한다.
from datetime import datetime
from typing import Any
from urllib.parse import urlparse

from pydantic import BaseModel, ConfigDict, Field, field_validator

from app.models import PluginState, Role, RunState


class TokenRequest(BaseModel):
    username: str
    password: str


class TokenResponse(BaseModel):
    access_token: str
    token_type: str = "bearer"


class UserView(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: str
    username: str
    role: Role
    is_active: bool


class UserCreate(BaseModel):
    username: str = Field(min_length=3, max_length=80)
    password: str = Field(min_length=10, max_length=128)
    role: Role = Role.viewer


class UserUpdate(BaseModel):
    username: str = Field(min_length=3, max_length=80)
    role: Role
    is_active: bool


class PasswordChange(BaseModel):
    current_password: str
    new_password: str = Field(min_length=10, max_length=128)


class PasswordReset(BaseModel):
    new_password: str = Field(min_length=10, max_length=128)


class PluginMenuView(BaseModel):
    plugin_id: str
    plugin_name: str
    menu_id: str
    title: str
    enabled: bool
    position: int


class PluginMenuUpdate(BaseModel):
    title: str = Field(min_length=1, max_length=160)
    enabled: bool
    position: int = Field(ge=0, le=10000)


class PluginInstallRequest(BaseModel):
    repository_url: str
    ref: str = "HEAD"

    @field_validator("repository_url")
    @classmethod
    def safe_repository_url(cls, value: str) -> str:
        if urlparse(value).scheme not in {"https", "ssh"}:
            raise ValueError("Git 저장소 URL은 https 또는 ssh 방식이어야 합니다")
        return value


class PluginView(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: str
    name: str
    repository_url: str
    state: PluginState
    active_release_id: str | None


class TaskRunRequest(BaseModel):
    input: dict[str, Any] = Field(default_factory=dict)


class TaskRunView(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: str
    plugin_id: str | None
    task_name: str
    state: RunState
    progress: int
    input: dict[str, Any]
    output: dict[str, Any] | None
    error: str | None
    created_at: datetime


class ScheduleCreate(BaseModel):
    name: str
    plugin_id: str
    task_name: str
    cron: str
    timezone: str = "UTC"
    input: dict[str, Any] = Field(default_factory=dict)
    enabled: bool = True


class ScheduleView(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: str
    name: str
    plugin_id: str
    task_name: str
    cron: str
    timezone: str
    input: dict[str, Any]
    enabled: bool
    next_run_at: datetime
    last_run_at: datetime | None
