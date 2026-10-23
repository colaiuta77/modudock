# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:24.22.1-trixie-slim AS node
FROM --platform=$BUILDPLATFORM golang:1.27.3-trixie AS build
COPY --from=node /usr/local/ /usr/local/
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY frontend/package.json frontend/package-lock.json ./frontend/
RUN cd frontend && npm ci
COPY . .
RUN node scripts/notices.mjs && cd frontend && npm run build
RUN cp -r frontend/dist/. internal/web/dist/
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/modudock ./cmd/modudock
RUN mkdir -p /out/data && chmod 700 /out/data

FROM gcr.io/distroless/static-debian13:nonroot
WORKDIR /app
COPY --from=build /out/modudock /app/modudock
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 7438
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/modudock", "healthcheck"]
ENTRYPOINT ["/app/modudock"]
CMD ["serve", "--data-dir", "/data", "--listen", "0.0.0.0:7438"]
