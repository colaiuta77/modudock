# ModuDock의 영속 엔터티와 상태 열거형을 정의한다.
import uuid
from datetime import UTC, datetime
from enum import StrEnum

from sqlalchemy import (
    JSON,
    Boolean,
    DateTime,
    Enum,
    ForeignKey,
    Integer,
    String,
    Text,
    UniqueConstraint,
)
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.db import Base


def new_id() -> str:
    return str(uuid.uuid4())


def utcnow() -> datetime:
    return datetime.now(UTC)


class Role(StrEnum):
    admin = "admin"
    operator = "operator"
    viewer = "viewer"


class PluginState(StrEnum):
    installing = "installing"
    active = "active"
    disabled = "disabled"
    failed = "failed"


class RunState(StrEnum):
    queued = "queued"
    running = "running"
    succeeded = "succeeded"
    failed = "failed"
    cancel_requested = "cancel_requested"
    cancelled = "cancelled"


class User(Base):
    __tablename__ = "users"

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=new_id)
    username: Mapped[str] = mapped_column(String(80), unique=True, index=True)
    password_hash: Mapped[str] = mapped_column(String(255))
    role: Mapped[Role] = mapped_column(Enum(Role), default=Role.viewer)
    is_active: Mapped[bool] = mapped_column(Boolean, default=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)


class Plugin(Base):
    __tablename__ = "plugins"

    id: Mapped[str] = mapped_column(String(120), primary_key=True)
    name: Mapped[str] = mapped_column(String(160))
    repository_url: Mapped[str] = mapped_column(Text)
    state: Mapped[PluginState] = mapped_column(Enum(PluginState), default=PluginState.installing)
    active_release_id: Mapped[str | None] = mapped_column(
        String(36), ForeignKey("plugin_releases.id", use_alter=True), nullable=True
    )
    installed_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    releases: Mapped[list["PluginRelease"]] = relationship(
        foreign_keys="PluginRelease.plugin_id",
        back_populates="plugin",
        cascade="all, delete-orphan",
    )


class PluginRelease(Base):
    __tablename__ = "plugin_releases"
    __table_args__ = (UniqueConstraint("plugin_id", "commit_sha"),)

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=new_id)
    plugin_id: Mapped[str] = mapped_column(ForeignKey("plugins.id"))
    version: Mapped[str] = mapped_column(String(80))
    commit_sha: Mapped[str] = mapped_column(String(64))
    install_path: Mapped[str] = mapped_column(Text)
    manifest: Mapped[dict] = mapped_column(JSON)
    healthy: Mapped[bool] = mapped_column(Boolean, default=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    plugin: Mapped[Plugin] = relationship(foreign_keys=[plugin_id], back_populates="releases")


class PluginSetting(Base):
    __tablename__ = "plugin_settings"

    plugin_id: Mapped[str] = mapped_column(ForeignKey("plugins.id"), primary_key=True)
    values: Mapped[dict] = mapped_column(JSON, default=dict)
    updated_by: Mapped[str | None] = mapped_column(ForeignKey("users.id"), nullable=True)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=utcnow, onupdate=utcnow
    )


class PluginMenuPreference(Base):
    __tablename__ = "plugin_menu_preferences"

    plugin_id: Mapped[str] = mapped_column(ForeignKey("plugins.id"), primary_key=True)
    menu_id: Mapped[str] = mapped_column(String(120), primary_key=True)
    title: Mapped[str] = mapped_column(String(160))
    enabled: Mapped[bool] = mapped_column(Boolean, default=True)
    position: Mapped[int] = mapped_column(Integer, default=0)
    updated_by: Mapped[str | None] = mapped_column(ForeignKey("users.id"), nullable=True)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=utcnow, onupdate=utcnow
    )


class TaskRun(Base):
    __tablename__ = "task_runs"

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=new_id)
    plugin_id: Mapped[str | None] = mapped_column(ForeignKey("plugins.id"), nullable=True)
    task_name: Mapped[str] = mapped_column(String(160))
    state: Mapped[RunState] = mapped_column(Enum(RunState), default=RunState.queued)
    progress: Mapped[int] = mapped_column(Integer, default=0)
    input: Mapped[dict] = mapped_column(JSON, default=dict)
    output: Mapped[dict | None] = mapped_column(JSON, nullable=True)
    error: Mapped[str | None] = mapped_column(Text, nullable=True)
    user_id: Mapped[str | None] = mapped_column(ForeignKey("users.id"), nullable=True)
    celery_task_id: Mapped[str | None] = mapped_column(String(80), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    started_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    finished_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)


class Schedule(Base):
    __tablename__ = "schedules"

    id: Mapped[str] = mapped_column(String(36), primary_key=True, default=new_id)
    name: Mapped[str] = mapped_column(String(160))
    plugin_id: Mapped[str] = mapped_column(ForeignKey("plugins.id"))
    task_name: Mapped[str] = mapped_column(String(160))
    cron: Mapped[str] = mapped_column(String(80))
    timezone: Mapped[str] = mapped_column(String(80), default="UTC")
    input: Mapped[dict] = mapped_column(JSON, default=dict)
    enabled: Mapped[bool] = mapped_column(Boolean, default=True)
    next_run_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    last_run_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    created_by: Mapped[str | None] = mapped_column(ForeignKey("users.id"), nullable=True)


class LogEntry(Base):
    __tablename__ = "log_entries"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    level: Mapped[str] = mapped_column(String(20), default="INFO")
    message: Mapped[str] = mapped_column(Text)
    plugin_id: Mapped[str | None] = mapped_column(String(120), nullable=True, index=True)
    task_id: Mapped[str | None] = mapped_column(String(160), nullable=True)
    run_id: Mapped[str | None] = mapped_column(String(36), nullable=True, index=True)
    user_id: Mapped[str | None] = mapped_column(String(36), nullable=True)
    timestamp: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow, index=True)
    context: Mapped[dict] = mapped_column(JSON, default=dict)


class SchedulerLease(Base):
    __tablename__ = "scheduler_leases"

    name: Mapped[str] = mapped_column(String(80), primary_key=True)
    owner_id: Mapped[str] = mapped_column(String(80))
    expires_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
