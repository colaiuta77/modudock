# 플러그인 매니페스트 형식과 안전한 argv 작업 선언을 검증한다.
from pathlib import Path

import pytest

from app.manifest import load_manifest


def test_loads_valid_manifest(tmp_path: Path):
    (tmp_path / "modudock.yaml").write_text(
        """
schema_version: 1
id: hello-world
name: Hello World
version: 0.1.0
tasks:
  - id: greet
    title: 인사하기
    command: [python, task.py]
""",
        encoding="utf-8",
    )
    manifest = load_manifest(tmp_path)
    assert manifest.id == "hello-world"
    assert manifest.tasks[0].command == ["python", "task.py"]


def test_rejects_invalid_plugin_id(tmp_path: Path):
    (tmp_path / "modudock.yaml").write_text(
        "id: '../escape'\nname: Bad\nversion: 1.0.0\n",
        encoding="utf-8",
    )
    with pytest.raises(ValueError):
        load_manifest(tmp_path)


def test_public_resource_only_accepts_get(tmp_path: Path):
    (tmp_path / "modudock.yaml").write_text(
        """
id: sale-plugin
name: Sale
version: 1.0.0
resources:
  - id: sales
    command: [python, resource.py]
    public_methods: [POST]
""",
        encoding="utf-8",
    )
    with pytest.raises(ValueError):
        load_manifest(tmp_path)
