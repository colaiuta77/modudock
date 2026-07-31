# 플러그인 설치와 작업 실행을 격리된 Celery worker에서 처리한다.
import json
import shutil
import subprocess
import sys
import tempfile
from datetime import UTC, datetime
from pathlib import Path

from celery import Celery
from celery.signals import worker_ready, worker_shutdown
from sqlalchemy import select

from app.config import settings
from app.db import SessionLocal
from app.logging_service import write_log, write_platform_log
from app.manifest import load_manifest
from app.models import Plugin, PluginRelease, PluginState, RunState, TaskRun

celery_app = Celery("modudock", broker=settings.redis_url, backend=settings.redis_url)
celery_app.conf.update(task_track_started=True, task_serializer="json", result_serializer="json")


@worker_ready.connect
def log_worker_ready(**_kwargs) -> None:
    write_platform_log("worker", "ModuDock worker가 준비되었습니다")


@worker_shutdown.connect
def log_worker_shutdown(**_kwargs) -> None:
    write_platform_log("worker", "ModuDock worker가 종료되었습니다")


def run_checked(argv: list[str], cwd: Path, timeout: int = 300) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        argv, cwd=cwd, text=True, capture_output=True, check=True, timeout=timeout
    )


@celery_app.task(name="modudock.install_plugin")
def install_plugin(repository_url: str, ref: str, user_id: str | None) -> str:
    settings.plugin_root.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=settings.plugin_root) as temp:
        checkout = Path(temp) / "checkout"
        try:
            run_checked(
                ["git", "clone", "--no-checkout", repository_url, str(checkout)], Path(temp)
            )
            run_checked(["git", "checkout", "--detach", ref], checkout)
            commit_sha = run_checked(["git", "rev-parse", "HEAD"], checkout).stdout.strip()
            manifest = load_manifest(checkout)
            release_dir = settings.plugin_root / manifest.id / "releases" / commit_sha
            if release_dir.exists():
                shutil.rmtree(release_dir)
            release_dir.parent.mkdir(parents=True, exist_ok=True)
            shutil.copytree(checkout, release_dir, ignore=shutil.ignore_patterns(".git"))

            requirements = release_dir / "requirements.txt"
            venv_dir = release_dir / ".venv"
            run_checked([sys.executable, "-m", "venv", str(venv_dir)], release_dir)
            if requirements.is_file():
                pip = venv_dir / ("Scripts/pip.exe" if sys.platform == "win32" else "bin/pip")
                run_checked(
                    [str(pip), "install", "--no-input", "-r", str(requirements)], release_dir, 600
                )
            if manifest.health_check:
                run_checked(
                    manifest.health_check.command,
                    release_dir,
                    manifest.health_check.timeout_seconds,
                )

            with SessionLocal.begin() as db:
                plugin = db.get(Plugin, manifest.id)
                if plugin is None:
                    plugin = Plugin(
                        id=manifest.id, name=manifest.name, repository_url=repository_url
                    )
                    db.add(plugin)
                    db.flush()
                release = db.scalar(
                    select(PluginRelease).where(
                        PluginRelease.plugin_id == manifest.id,
                        PluginRelease.commit_sha == commit_sha,
                    )
                )
                if release is None:
                    release = PluginRelease(
                        plugin_id=manifest.id,
                        version=manifest.version,
                        commit_sha=commit_sha,
                        install_path=str(release_dir),
                        manifest=manifest.model_dump(mode="json"),
                        healthy=True,
                    )
                    db.add(release)
                    db.flush()
                plugin.name = manifest.name
                plugin.repository_url = repository_url
                plugin.active_release_id = release.id
                plugin.state = PluginState.active
                write_log(
                    db, "플러그인을 설치하고 활성화했습니다", plugin_id=plugin.id, user_id=user_id
                )
            return manifest.id
        except Exception as exc:
            with SessionLocal.begin() as db:
                write_log(
                    db,
                    f"플러그인 설치 실패: {exc}",
                    level="ERROR",
                    user_id=user_id,
                    context={"repository_url": repository_url, "ref": ref},
                )
            raise


@celery_app.task(bind=True, name="modudock.run_plugin_task")
def run_plugin_task(self, run_id: str) -> None:
    with SessionLocal.begin() as db:
        run = db.get(TaskRun, run_id)
        if run is None or run.state == RunState.cancel_requested:
            return
        plugin = db.get(Plugin, run.plugin_id)
        release = db.get(PluginRelease, plugin.active_release_id) if plugin else None
        if release is None:
            run.state = RunState.failed
            run.error = "활성 플러그인 릴리스가 없습니다"
            return
        task = next(
            (item for item in release.manifest["tasks"] if item["id"] == run.task_name), None
        )
        if task is None:
            run.state = RunState.failed
            run.error = "선언되지 않은 작업입니다"
            return
        run.state = RunState.running
        run.started_at = datetime.now(UTC)
        run.progress = 10
        command = task["command"]
        install_path = Path(release.install_path)
        input_data = run.input
        plugin_id = run.plugin_id

    try:
        completed = subprocess.run(
            command,
            cwd=install_path,
            input=json.dumps(input_data),
            text=True,
            capture_output=True,
            timeout=3600,
            check=True,
        )
        output = json.loads(completed.stdout or "{}")
        with SessionLocal.begin() as db:
            run = db.get(TaskRun, run_id)
            if run.state == RunState.cancel_requested:
                run.state = RunState.cancelled
                run.output = None
            else:
                run.state = RunState.succeeded
                run.output = output
            run.progress = 100
            run.finished_at = datetime.now(UTC)
            write_log(
                db,
                "취소 요청에 따라 작업 결과를 폐기했습니다"
                if run.state == RunState.cancelled
                else "작업이 완료되었습니다",
                plugin_id=plugin_id,
                task_id=run.task_name,
                run_id=run.id,
            )
    except Exception as exc:
        with SessionLocal.begin() as db:
            run = db.get(TaskRun, run_id)
            run.state = RunState.failed
            run.error = str(exc)
            run.finished_at = datetime.now(UTC)
            write_log(
                db,
                f"작업 실패: {exc}",
                level="ERROR",
                plugin_id=run.plugin_id,
                task_id=run.task_name,
                run_id=run.id,
            )
        raise
