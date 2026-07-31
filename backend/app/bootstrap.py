# 데이터베이스를 마이그레이션하고 최초 관리자 계정을 준비한다.
from alembic.config import Config
from sqlalchemy import select

from alembic import command
from app.config import settings
from app.db import SessionLocal
from app.logging_service import write_platform_log
from app.models import Role, User
from app.security import hash_password


def main() -> None:
    settings.plugin_root.mkdir(parents=True, exist_ok=True)
    if settings.database_url.startswith("sqlite"):
        database_path = settings.database_url.removeprefix("sqlite:///")
        from pathlib import Path

        Path(database_path).parent.mkdir(parents=True, exist_ok=True)
    config = Config("alembic.ini")
    config.set_main_option("sqlalchemy.url", settings.database_url)
    command.upgrade(config, "head")
    with SessionLocal.begin() as db:
        existing = db.scalar(select(User).where(User.username == settings.admin_username))
        if existing is None:
            db.add(
                User(
                    username=settings.admin_username,
                    password_hash=hash_password(settings.admin_password),
                    role=Role.admin,
                )
            )
    write_platform_log("api", "ModuDock API 초기화를 완료했습니다")


if __name__ == "__main__":
    main()
