# 이미지에 포함된 공식 플러그인을 영구 릴리스 경로에 등록한다.
import hashlib
import shutil
from pathlib import Path

from sqlalchemy import select

from app.config import settings
from app.db import SessionLocal
from app.logging_service import write_log
from app.manifest import load_manifest
from app.models import Plugin, PluginRelease, PluginState


def source_digest(source: Path) -> str:
    digest = hashlib.sha256()
    for path in sorted(source.rglob("*")):
        if not path.is_file() or ".venv" in path.parts or "__pycache__" in path.parts:
            continue
        digest.update(path.relative_to(source).as_posix().encode())
        digest.update(path.read_bytes())
    return digest.hexdigest()


def register_bundled_plugins() -> None:
    root = settings.bundled_plugin_root
    if not root.is_dir():
        return
    settings.plugin_root.mkdir(parents=True, exist_ok=True)
    for source in sorted(root.iterdir()):
        if not source.is_dir() or not (source / "modudock.yaml").is_file():
            continue
        manifest = load_manifest(source)
        digest = source_digest(source)
        release_dir = settings.plugin_root / manifest.id / "releases" / digest
        if not release_dir.exists():
            shutil.copytree(source, release_dir, ignore=shutil.ignore_patterns("__pycache__"))
        with SessionLocal.begin() as db:
            plugin = db.get(Plugin, manifest.id)
            created = plugin is None
            if plugin is None:
                plugin = Plugin(
                    id=manifest.id,
                    name=manifest.name,
                    repository_url=f"bundled://{manifest.id}",
                )
                db.add(plugin)
                db.flush()
            release = db.scalar(
                select(PluginRelease).where(
                    PluginRelease.plugin_id == manifest.id,
                    PluginRelease.commit_sha == digest,
                )
            )
            if release is None:
                release = PluginRelease(
                    plugin_id=manifest.id,
                    version=manifest.version,
                    commit_sha=digest,
                    install_path=str(release_dir),
                    manifest=manifest.model_dump(mode="json"),
                    healthy=True,
                )
                db.add(release)
                db.flush()
                write_log(db, "번들 플러그인 릴리스를 등록했습니다", plugin_id=manifest.id)
            if plugin.repository_url.startswith("bundled://") and (
                created or plugin.state == PluginState.active
            ):
                plugin.name = manifest.name
                plugin.active_release_id = release.id
                plugin.state = PluginState.active
