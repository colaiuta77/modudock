FROM node:24-alpine AS web-build

WORKDIR /web
COPY frontend/package*.json ./
RUN npm ci
COPY frontend .
RUN npm run build

FROM python:3.14-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY backend/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY plugins /app/bundled-plugins
RUN python -m venv /app/bundled-plugins/local-mart-sale/.venv \
    && /app/bundled-plugins/local-mart-sale/.venv/bin/pip install \
        --no-cache-dir -r /app/bundled-plugins/local-mart-sale/requirements.txt
COPY backend /app
COPY --from=web-build /web/dist /app/static
RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]
