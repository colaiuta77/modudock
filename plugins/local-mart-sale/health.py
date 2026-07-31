# 당일 세일 플러그인의 필수 파일과 Python 실행 환경을 확인한다.
from pathlib import Path

required = ["resource.py", "notify.py", "modudock.yaml"]
missing = [name for name in required if not (Path(__file__).parent / name).is_file()]
if missing:
    raise SystemExit(f"필수 파일이 없습니다: {', '.join(missing)}")
