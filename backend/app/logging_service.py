# 플랫폼과 플러그인 작업 로그를 공통 필드로 저장한다.
from typing import Any

from sqlalchemy.orm import Session

from app.models import LogEntry


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
