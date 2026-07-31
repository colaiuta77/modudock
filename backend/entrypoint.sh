#!/bin/sh
# ModuDock 백엔드 프로세스 유형에 맞는 서비스를 시작한다.
set -eu

python -m app.bootstrap

case "${1:-api}" in
  api)
    exec uvicorn app.main:app --host 0.0.0.0 --port 8000
    ;;
  worker)
    exec celery -A app.worker.celery_app worker --loglevel=INFO --concurrency=2
    ;;
  scheduler)
    exec python -m app.scheduler
    ;;
  *)
    exec "$@"
    ;;
esac
