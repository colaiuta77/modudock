# JSON 표준 입력에서 이름을 받아 인사 결과를 출력하는 샘플 작업이다.
import json
import sys

payload = json.load(sys.stdin)
print(json.dumps({"message": f"안녕하세요, {payload.get('name', 'ModuDock')}!"}, ensure_ascii=False))
