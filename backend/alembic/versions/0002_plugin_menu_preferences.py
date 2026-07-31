# 플러그인 메뉴의 이름과 노출 순서를 저장하는 설정 테이블을 추가한다.
import sqlalchemy as sa
from sqlalchemy import inspect

from alembic import op

revision = "0002_plugin_menu_preferences"
down_revision = "0001_initial"
branch_labels = None
depends_on = None


def upgrade() -> None:
    if inspect(op.get_bind()).has_table("plugin_menu_preferences"):
        return
    op.create_table(
        "plugin_menu_preferences",
        sa.Column("plugin_id", sa.String(length=120), nullable=False),
        sa.Column("menu_id", sa.String(length=120), nullable=False),
        sa.Column("title", sa.String(length=160), nullable=False),
        sa.Column("enabled", sa.Boolean(), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("updated_by", sa.String(length=36), nullable=True),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(["plugin_id"], ["plugins.id"]),
        sa.ForeignKeyConstraint(["updated_by"], ["users.id"]),
        sa.PrimaryKeyConstraint("plugin_id", "menu_id"),
    )


def downgrade() -> None:
    if inspect(op.get_bind()).has_table("plugin_menu_preferences"):
        op.drop_table("plugin_menu_preferences")
