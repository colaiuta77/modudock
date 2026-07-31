# 플랫폼과 플러그인 작업 로그를 공통 필드로 저장한다.
import logging
from typing import Any

from sqlalchemy.orm import Session

from app.db import SessionLocal
from app.models import LogEntry

logger = logging.getLogger(__name__)


def write_log(
    db: Session,
    message: str,
    *,
    level: str = "INFO",
    plugin_id: str | None = None,
    task_id: str | None = None,
    run_id: str | None = None,
    user_id: str | None = None,
    context: dict[str, Any] | None = None,
) -> None:
    db.add(
        LogEntry(
            message=message,
            level=level,
            plugin_id=plugin_id,
            task_id=task_id,
            run_id=run_id,
            user_id=user_id,
            context=context or {},
        )
    )


def write_platform_log(
    component: str,
    message: str,
    *,
    level: str = "INFO",
    context: dict[str, Any] | None = None,
) -> None:
    try:
        with SessionLocal.begin() as db:
            write_log(
                db,
                message,
                level=level,
                context={**(context or {}), "component": component},
            )
    except Exception:
        logger.exception("플랫폼 로그를 저장하지 못했습니다")
