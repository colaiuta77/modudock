# ModuDock HTTP API와 실시간 작업 이벤트 스트림을 제공한다.
import asyncio
import json
from datetime import UTC, datetime
from pathlib import Path
from typing import Annotated
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from croniter import croniter
from fastapi import Depends, FastAPI, HTTPException, Query
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse
from fastapi.staticfiles import StaticFiles
from jsonschema import ValidationError as JsonSchemaError
from jsonschema import validate as validate_json
from sqlalchemy import desc, select
from sqlalchemy.orm import Session

from app.config import settings
from app.db import SessionLocal, get_db
from app.logging_service import write_log
from app.models import (
    LogEntry,
    Plugin,
    PluginMenuPreference,
    PluginRelease,
    PluginSetting,
    PluginState,
    Role,
    RunState,
    Schedule,
    TaskRun,
    User,
)
from app.schemas import (
    PasswordChange,
    PasswordReset,
    PluginInstallRequest,
    PluginMenuUpdate,
    PluginMenuView,
    PluginView,
    ScheduleCreate,
    ScheduleView,
    TaskRunRequest,
    TaskRunView,
    TokenRequest,
    TokenResponse,
    UserCreate,
    UserUpdate,
    UserView,
)
from app.security import create_token, current_user, hash_password, require_roles, verify_password
from app.worker import install_plugin, run_plugin_task

app = FastAPI(title="ModuDock API", version="0.1.0")
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

Db = Annotated[Session, Depends(get_db)]
CurrentUser = Annotated[User, Depends(current_user)]
Operator = Annotated[User, Depends(require_roles(Role.admin, Role.operator))]
Admin = Annotated[User, Depends(require_roles(Role.admin))]


@app.get("/healthz")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/api/auth/token", response_model=TokenResponse)
def login(payload: TokenRequest, db: Db) -> TokenResponse:
    user = db.scalar(select(User).where(User.username == payload.username))
    if user is None or not user.is_active or not verify_password(payload.password, user.password_hash):
        raise HTTPException(401, "사용자 이름 또는 비밀번호가 올바르지 않습니다")
    return TokenResponse(access_token=create_token(user))


@app.get("/api/auth/me", response_model=UserView)
def me(user: CurrentUser) -> User:
    return user


@app.put("/api/auth/password", status_code=204)
def change_password(payload: PasswordChange, user: CurrentUser, db: Db) -> None:
    if not verify_password(payload.current_password, user.password_hash):
        raise HTTPException(400, "현재 비밀번호가 올바르지 않습니다")
    user.password_hash = hash_password(payload.new_password)
    write_log(db, "사용자가 비밀번호를 변경했습니다", user_id=user.id)
    db.commit()


@app.get("/api/users", response_model=list[UserView])
def list_users(_user: Admin, db: Db) -> list[User]:
    return list(db.scalars(select(User).order_by(User.username)))


@app.post("/api/users", response_model=UserView, status_code=201)
def create_user(payload: UserCreate, actor: Admin, db: Db) -> User:
    if db.scalar(select(User).where(User.username == payload.username)):
        raise HTTPException(409, "이미 존재하는 사용자 이름입니다")
    user = User(
        username=payload.username,
        password_hash=hash_password(payload.password),
        role=payload.role,
    )
    db.add(user)
    db.flush()
    write_log(
        db,
        "사용자 계정을 추가했습니다",
        user_id=actor.id,
        context={"target_user_id": user.id},
    )
    db.commit()
    db.refresh(user)
    return user


def active_admin_count(db: Session) -> int:
    return len(list(db.scalars(select(User).where(User.role == Role.admin, User.is_active.is_(True)))))


@app.put("/api/users/{user_id}", response_model=UserView)
def update_user(user_id: str, payload: UserUpdate, actor: Admin, db: Db) -> User:
    target = db.get(User, user_id)
    if target is None:
        raise HTTPException(404, "사용자를 찾을 수 없습니다")
    duplicate = db.scalar(select(User).where(User.username == payload.username, User.id != user_id))
    if duplicate:
        raise HTTPException(409, "이미 존재하는 사용자 이름입니다")
    removes_admin = (
        target.role == Role.admin and target.is_active and (payload.role != Role.admin or not payload.is_active)
    )
    if removes_admin and active_admin_count(db) <= 1:
        raise HTTPException(409, "마지막 활성 관리자는 변경할 수 없습니다")
    target.username = payload.username
    target.role = payload.role
    target.is_active = payload.is_active
    write_log(
        db,
        "사용자 계정 정보를 변경했습니다",
        user_id=actor.id,
        context={"target_user_id": target.id},
    )
    db.commit()
    db.refresh(target)
    return target


@app.delete("/api/users/{user_id}", status_code=204)
def delete_user(user_id: str, actor: Admin, db: Db) -> None:
    target = db.get(User, user_id)
    if target is None:
        raise HTTPException(404, "사용자를 찾을 수 없습니다")
    if target.id == actor.id:
        raise HTTPException(409, "현재 로그인한 계정은 삭제할 수 없습니다")
    if target.role == Role.admin and target.is_active and active_admin_count(db) <= 1:
        raise HTTPException(409, "마지막 활성 관리자는 삭제할 수 없습니다")
    target.is_active = False
    write_log(
        db,
        "사용자 계정을 비활성화했습니다",
        user_id=actor.id,
        context={"target_user_id": target.id},
    )
    db.commit()


@app.put("/api/users/{user_id}/password", status_code=204)
def reset_user_password(user_id: str, payload: PasswordReset, actor: Admin, db: Db) -> None:
    target = db.get(User, user_id)
    if target is None:
        raise HTTPException(404, "사용자를 찾을 수 없습니다")
    target.password_hash = hash_password(payload.new_password)
    write_log(
        db,
        "관리자가 사용자 비밀번호를 재설정했습니다",
        user_id=actor.id,
        context={"target_user_id": target.id},
    )
    db.commit()


@app.get("/api/plugins", response_model=list[PluginView])
def list_plugins(_user: CurrentUser, db: Db) -> list[Plugin]:
    return list(db.scalars(select(Plugin).order_by(Plugin.name)))


@app.post("/api/plugins/install", status_code=202)
def request_install(payload: PluginInstallRequest, user: Admin) -> dict[str, str]:
    result = install_plugin.delay(payload.repository_url, payload.ref, user.id)
    return {"job_id": result.id}


@app.get("/api/plugins/{plugin_id}/manifest")
def plugin_manifest(plugin_id: str, _user: CurrentUser, db: Db) -> dict:
    plugin = db.get(Plugin, plugin_id)
    release = db.get(PluginRelease, plugin.active_release_id) if plugin else None
    if release is None:
        raise HTTPException(404, "활성 플러그인 릴리스를 찾을 수 없습니다")
    return release.manifest


def declared_plugin_menus(db: Session) -> list[dict]:
    items: list[dict] = []
    plugins = db.scalars(select(Plugin).where(Plugin.state == PluginState.active).order_by(Plugin.name))
    default_position = 0
    for plugin in plugins:
        release = db.get(PluginRelease, plugin.active_release_id)
        if release is None:
            continue
        for declared in release.manifest.get("menus", []):
            menu_id = declared.get("id")
            if not menu_id:
                continue
            preference = db.get(PluginMenuPreference, (plugin.id, menu_id))
            items.append(
                {
                    "plugin_id": plugin.id,
                    "plugin_name": plugin.name,
                    "menu_id": menu_id,
                    "title": preference.title if preference else declared.get("title", menu_id),
                    "enabled": preference.enabled if preference else True,
                    "position": preference.position if preference else default_position,
                }
            )
            default_position += 10
    return sorted(items, key=lambda item: (item["position"], item["title"]))


@app.get("/api/plugin-menus", response_model=list[PluginMenuView])
def list_plugin_menus(_user: CurrentUser, db: Db) -> list[dict]:
    return declared_plugin_menus(db)


@app.put(
    "/api/plugin-menus/{plugin_id}/{menu_id}",
    response_model=PluginMenuView,
)
def update_plugin_menu(
    plugin_id: str,
    menu_id: str,
    payload: PluginMenuUpdate,
    user: Admin,
    db: Db,
) -> dict:
    declared = next(
        (item for item in declared_plugin_menus(db) if item["plugin_id"] == plugin_id and item["menu_id"] == menu_id),
        None,
    )
    if declared is None:
        raise HTTPException(404, "플러그인 메뉴를 찾을 수 없습니다")
    preference = db.get(PluginMenuPreference, (plugin_id, menu_id))
    if preference is None:
        preference = PluginMenuPreference(plugin_id=plugin_id, menu_id=menu_id)
        db.add(preference)
    preference.title = payload.title
    preference.enabled = payload.enabled
    preference.position = payload.position
    preference.updated_by = user.id
    write_log(
        db,
        "플러그인 메뉴 설정을 변경했습니다",
        plugin_id=plugin_id,
        user_id=user.id,
        context={"menu_id": menu_id},
    )
    db.commit()
    return {
        **declared,
        "title": preference.title,
        "enabled": preference.enabled,
        "position": preference.position,
    }


@app.get("/api/plugins/{plugin_id}/settings")
def get_plugin_settings(plugin_id: str, _user: CurrentUser, db: Db) -> dict:
    if db.get(Plugin, plugin_id) is None:
        raise HTTPException(404, "플러그인을 찾을 수 없습니다")
    stored = db.get(PluginSetting, plugin_id)
    return stored.values if stored else {}


@app.put("/api/plugins/{plugin_id}/settings")
def put_plugin_settings(plugin_id: str, values: dict, user: Operator, db: Db) -> dict:
    plugin = db.get(Plugin, plugin_id)
    release = db.get(PluginRelease, plugin.active_release_id) if plugin else None
    if release is None:
        raise HTTPException(404, "활성 플러그인 릴리스를 찾을 수 없습니다")
    try:
        validate_json(instance=values, schema=release.manifest.get("settings_schema", {}))
    except JsonSchemaError as exc:
        raise HTTPException(422, f"설정 스키마 검증에 실패했습니다: {exc.message}") from exc
    stored = db.get(PluginSetting, plugin_id)
    if stored is None:
        stored = PluginSetting(plugin_id=plugin_id)
        db.add(stored)
    stored.values = values
    stored.updated_by = user.id
    write_log(db, "플러그인 설정을 변경했습니다", plugin_id=plugin_id, user_id=user.id)
    db.commit()
    return values


@app.get("/api/plugins/{plugin_id}/releases")
def plugin_releases(plugin_id: str, _user: CurrentUser, db: Db) -> list[dict]:
    releases = db.scalars(
        select(PluginRelease).where(PluginRelease.plugin_id == plugin_id).order_by(desc(PluginRelease.created_at))
    )
    return [
        {
            "id": release.id,
            "version": release.version,
            "commit_sha": release.commit_sha,
            "healthy": release.healthy,
            "created_at": release.created_at,
        }
        for release in releases
    ]


@app.post("/api/plugins/{plugin_id}/releases/{release_id}/activate")
def activate_release(plugin_id: str, release_id: str, user: Admin, db: Db) -> dict[str, str]:
    plugin = db.get(Plugin, plugin_id)
    release = db.get(PluginRelease, release_id)
    if plugin is None or release is None or release.plugin_id != plugin_id or not release.healthy:
        raise HTTPException(404, "활성화할 수 있는 릴리스를 찾을 수 없습니다")
    plugin.active_release_id = release.id
    plugin.state = PluginState.active
    write_log(db, "플러그인 릴리스를 활성화했습니다", plugin_id=plugin_id, user_id=user.id)
    db.commit()
    return {"active_release_id": release.id}


@app.post("/api/plugins/{plugin_id}/disable")
def disable_plugin(plugin_id: str, user: Admin, db: Db) -> dict[str, str]:
    plugin = db.get(Plugin, plugin_id)
    if plugin is None:
        raise HTTPException(404, "플러그인을 찾을 수 없습니다")
    plugin.state = PluginState.disabled
    write_log(db, "플러그인을 비활성화했습니다", plugin_id=plugin_id, user_id=user.id)
    db.commit()
    return {"state": plugin.state.value}


@app.post("/api/plugins/{plugin_id}/tasks/{task_name}/runs", response_model=TaskRunView, status_code=202)
def create_run(plugin_id: str, task_name: str, payload: TaskRunRequest, user: Operator, db: Db) -> TaskRun:
    plugin = db.get(Plugin, plugin_id)
    release = db.get(PluginRelease, plugin.active_release_id) if plugin else None
    declared = release and any(item["id"] == task_name for item in release.manifest.get("tasks", []))
    if plugin is None or plugin.state != PluginState.active or not declared:
        raise HTTPException(404, "실행 가능한 플러그인 작업을 찾을 수 없습니다")
    task = next(item for item in release.manifest["tasks"] if item["id"] == task_name)
    try:
        validate_json(instance=payload.input, schema=task.get("input_schema", {}))
    except JsonSchemaError as exc:
        raise HTTPException(422, f"작업 입력 검증에 실패했습니다: {exc.message}") from exc
    run = TaskRun(plugin_id=plugin_id, task_name=task_name, input=payload.input, user_id=user.id)
    db.add(run)
    db.flush()
    result = run_plugin_task.delay(run.id)
    run.celery_task_id = result.id
    write_log(
        db,
        "작업을 대기열에 추가했습니다",
        plugin_id=plugin_id,
        task_id=task_name,
        run_id=run.id,
        user_id=user.id,
    )
    db.commit()
    db.refresh(run)
    return run


@app.get("/api/runs", response_model=list[TaskRunView])
def list_runs(_user: CurrentUser, db: Db, limit: int = Query(50, ge=1, le=200)) -> list[TaskRun]:
    return list(db.scalars(select(TaskRun).order_by(desc(TaskRun.created_at)).limit(limit)))


@app.post("/api/runs/{run_id}/cancel", response_model=TaskRunView)
def cancel_run(run_id: str, user: Operator, db: Db) -> TaskRun:
    run = db.get(TaskRun, run_id)
    if run is None:
        raise HTTPException(404, "작업 실행을 찾을 수 없습니다")
    if run.state in {RunState.queued, RunState.running}:
        run.state = RunState.cancel_requested
        write_log(
            db,
            "작업 취소를 요청했습니다",
            plugin_id=run.plugin_id,
            task_id=run.task_name,
            run_id=run.id,
            user_id=user.id,
        )
        db.commit()
        db.refresh(run)
    return run


@app.get("/api/runs/{run_id}/events")
async def run_events(run_id: str, _user: CurrentUser) -> StreamingResponse:
    async def stream():
        last_log_id = 0
        while True:
            with SessionLocal() as db:
                run = db.get(TaskRun, run_id)
                if run is None:
                    yield 'event: error\ndata: {"message":"not found"}\n\n'
                    return
                logs = list(
                    db.scalars(
                        select(LogEntry)
                        .where(LogEntry.run_id == run_id, LogEntry.id > last_log_id)
                        .order_by(LogEntry.id)
                    )
                )
                for entry in logs:
                    last_log_id = entry.id
                    yield f"event: log\ndata: {json.dumps({'level': entry.level, 'message': entry.message})}\n\n"
                payload = {
                    "state": run.state.value,
                    "progress": run.progress,
                    "output": run.output,
                    "error": run.error,
                }
                yield f"event: status\ndata: {json.dumps(payload)}\n\n"
                if run.state in {RunState.succeeded, RunState.failed, RunState.cancelled}:
                    return
            await asyncio.sleep(1)

    return StreamingResponse(stream(), media_type="text/event-stream")


def next_cron(cron: str, timezone: str) -> datetime:
    try:
        zone = ZoneInfo(timezone)
        next_local = croniter(cron, datetime.now(zone)).get_next(datetime)
    except (ValueError, ZoneInfoNotFoundError) as exc:
        raise HTTPException(422, f"유효하지 않은 cron 또는 시간대입니다: {exc}") from exc
    return next_local.astimezone(UTC)


@app.get("/api/schedules", response_model=list[ScheduleView])
def list_schedules(_user: CurrentUser, db: Db) -> list[Schedule]:
    return list(db.scalars(select(Schedule).order_by(Schedule.name)))


@app.post("/api/schedules", response_model=ScheduleView, status_code=201)
def create_schedule(payload: ScheduleCreate, user: Operator, db: Db) -> Schedule:
    plugin = db.get(Plugin, payload.plugin_id)
    release = db.get(PluginRelease, plugin.active_release_id) if plugin else None
    if release is None or not any(item["id"] == payload.task_name for item in release.manifest.get("tasks", [])):
        raise HTTPException(404, "스케줄할 플러그인 작업을 찾을 수 없습니다")
    schedule = Schedule(
        **payload.model_dump(),
        next_run_at=next_cron(payload.cron, payload.timezone),
        created_by=user.id,
    )
    db.add(schedule)
    db.commit()
    db.refresh(schedule)
    return schedule


@app.put("/api/schedules/{schedule_id}", response_model=ScheduleView)
def update_schedule(schedule_id: str, payload: ScheduleCreate, _user: Operator, db: Db) -> Schedule:
    schedule = db.get(Schedule, schedule_id)
    if schedule is None:
        raise HTTPException(404, "스케줄을 찾을 수 없습니다")
    plugin = db.get(Plugin, payload.plugin_id)
    release = db.get(PluginRelease, plugin.active_release_id) if plugin else None
    if release is None or not any(item["id"] == payload.task_name for item in release.manifest.get("tasks", [])):
        raise HTTPException(404, "스케줄할 플러그인 작업을 찾을 수 없습니다")
    for key, value in payload.model_dump().items():
        setattr(schedule, key, value)
    schedule.next_run_at = next_cron(payload.cron, payload.timezone)
    db.commit()
    db.refresh(schedule)
    return schedule


@app.delete("/api/schedules/{schedule_id}", status_code=204)
def delete_schedule(schedule_id: str, _user: Operator, db: Db) -> None:
    schedule = db.get(Schedule, schedule_id)
    if schedule is None:
        raise HTTPException(404, "스케줄을 찾을 수 없습니다")
    db.delete(schedule)
    db.commit()


@app.get("/api/logs")
def list_logs(
    _user: CurrentUser,
    db: Db,
    plugin_id: str | None = None,
    run_id: str | None = None,
    level: str | None = None,
    query: str | None = None,
    limit: int = Query(100, ge=1, le=500),
) -> list[dict]:
    statement = select(LogEntry)
    if plugin_id:
        statement = statement.where(LogEntry.plugin_id == plugin_id)
    if run_id:
        statement = statement.where(LogEntry.run_id == run_id)
    if level:
        statement = statement.where(LogEntry.level == level.upper())
    if query:
        statement = statement.where(LogEntry.message.contains(query))
    entries = db.scalars(statement.order_by(desc(LogEntry.timestamp)).limit(limit))
    return [
        {
            "id": entry.id,
            "level": entry.level,
            "message": entry.message,
            "plugin_id": entry.plugin_id,
            "task_id": entry.task_id,
            "run_id": entry.run_id,
            "user_id": entry.user_id,
            "timestamp": entry.timestamp,
            "context": entry.context,
        }
        for entry in entries
    ]


def mount_frontend(application: FastAPI, static_dir: Path) -> None:
    if static_dir.is_dir():
        application.mount("/", StaticFiles(directory=static_dir, html=True), name="frontend")


mount_frontend(app, Path(__file__).resolve().parent.parent / "static")
