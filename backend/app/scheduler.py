# DB lease를 가진 단일 스케줄러가 실행 시각이 된 작업을 Celery에 발행한다.
import socket
import time
import uuid
from datetime import UTC, datetime, timedelta
from zoneinfo import ZoneInfo

from croniter import croniter
from sqlalchemy import delete, select
from sqlalchemy.exc import IntegrityError

from app.config import settings
from app.db import SessionLocal
from app.logging_service import write_log
from app.models import RunState, Schedule, SchedulerLease, TaskRun
from app.worker import run_plugin_task

OWNER_ID = f"{socket.gethostname()}-{uuid.uuid4()}"


def acquire_lease() -> bool:
    now = datetime.now(UTC)
    with SessionLocal.begin() as db:
        db.execute(
            delete(SchedulerLease).where(
                SchedulerLease.name == "primary", SchedulerLease.expires_at <= now
            )
        )
        lease = db.get(SchedulerLease, "primary")
        if lease and lease.owner_id != OWNER_ID:
            return False
        if lease:
            lease.expires_at = now + timedelta(seconds=settings.scheduler_lease_seconds)
            return True
        db.add(
            SchedulerLease(
                name="primary",
                owner_id=OWNER_ID,
                expires_at=now + timedelta(seconds=settings.scheduler_lease_seconds),
            )
        )
        try:
            db.flush()
        except IntegrityError:
            return False
        return True


def dispatch_due() -> None:
    now = datetime.now(UTC)
    run_ids: list[str] = []
    with SessionLocal.begin() as db:
        due = list(
            db.scalars(
                select(Schedule)
                .where(Schedule.enabled.is_(True), Schedule.next_run_at <= now)
                .order_by(Schedule.next_run_at)
            )
        )
        for schedule in due:
            run = TaskRun(
                plugin_id=schedule.plugin_id,
                task_name=schedule.task_name,
                input=schedule.input,
                state=RunState.queued,
                user_id=schedule.created_by,
            )
            db.add(run)
            db.flush()
            run_ids.append(run.id)
            schedule.last_run_at = now
            local_now = now.astimezone(ZoneInfo(schedule.timezone))
            schedule.next_run_at = (
                croniter(schedule.cron, local_now).get_next(datetime).astimezone(UTC)
            )
            write_log(
                db,
                "스케줄 작업을 대기열에 추가했습니다",
                plugin_id=run.plugin_id,
                task_id=run.task_name,
                run_id=run.id,
                user_id=run.user_id,
            )
    for run_id in run_ids:
        result = run_plugin_task.delay(run_id)
        with SessionLocal.begin() as db:
            run = db.get(TaskRun, run_id)
            if run:
                run.celery_task_id = result.id


def main() -> None:
    while True:
        if acquire_lease():
            dispatch_due()
        time.sleep(settings.scheduler_poll_seconds)


if __name__ == "__main__":
    main()
