# 활성 플러그인의 선언된 리소스를 격리 subprocess로 실행한다.
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any


class PluginExecutionError(RuntimeError):
    pass


def plugin_python(release_dir: Path) -> Path | None:
    candidates = [
        release_dir / ".venv" / ("Scripts/python.exe" if sys.platform == "win32" else "bin/python"),
        release_dir / ".venv" / "bin" / "python",
        release_dir / ".venv" / "Scripts" / "python.exe",
    ]
    return next((candidate for candidate in candidates if candidate.is_file()), None)


def resolve_command(command: list[str], release_dir: Path) -> list[str]:
    if command[0] not in {"python", "python3"}:
        return command
    interpreter = plugin_python(release_dir)
    return [str(interpreter) if interpreter else sys.executable, *command[1:]]


def plugin_environment(
    plugin_id: str,
    data_root: Path,
    settings_values: dict[str, Any],
) -> dict[str, str]:
    data_dir = (data_root / plugin_id).resolve()
    data_root_resolved = data_root.resolve()
    if data_dir.parent != data_root_resolved:
        raise PluginExecutionError("플러그인 데이터 경로가 허용 범위를 벗어났습니다")
    data_dir.mkdir(parents=True, exist_ok=True)
    inherited = {
        key: value
        for key, value in os.environ.items()
        if key in {"PATH", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "LANG", "LC_ALL", "TZ"}
    }
    return {
        **inherited,
        "PYTHONIOENCODING": "utf-8",
        "PYTHONUTF8": "1",
        "MODUDOCK_PLUGIN_ID": plugin_id,
        "MODUDOCK_PLUGIN_DATA_DIR": str(data_dir),
        "MODUDOCK_PLUGIN_SETTINGS": json.dumps(settings_values, ensure_ascii=False),
    }


def execute_plugin_command(
    command: list[str],
    release_dir: Path,
    payload: dict[str, Any],
    plugin_id: str,
    data_root: Path,
    settings_values: dict[str, Any],
    timeout_seconds: int,
) -> dict[str, Any]:
    completed = subprocess.run(
        resolve_command(command, release_dir),
        cwd=release_dir,
        env=plugin_environment(plugin_id, data_root, settings_values),
        input=json.dumps(payload, ensure_ascii=False),
        text=True,
        encoding="utf-8",
        capture_output=True,
        timeout=timeout_seconds,
        check=False,
    )
    if completed.returncode != 0:
        error = completed.stderr.strip() or f"플러그인 프로세스가 {completed.returncode} 코드로 종료되었습니다"
        raise PluginExecutionError(error[-2000:])
    try:
        result = json.loads(completed.stdout or "{}")
    except json.JSONDecodeError as exc:
        raise PluginExecutionError("플러그인이 유효한 JSON 응답을 반환하지 않았습니다") from exc
    if not isinstance(result, dict):
        raise PluginExecutionError("플러그인 응답은 JSON 객체여야 합니다")
    return result
