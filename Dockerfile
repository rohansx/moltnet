# syntax=docker/dockerfile:1
# Multi-stage build producing a tiny, static, non-root moltnetd image.
# The Go binary is pure Go (no cgo — modernc.org/sqlite), so a distroless static
# base is sufficient. The UI is a React/Vite SPA built in its own stage and
# served by moltnetd itself, so the image stays a single process.

# ---- 1. build the React SPA ----
FROM node:22-alpine AS web
WORKDIR /app
RUN corepack enable
# Install deps against the lockfile first so this layer caches across UI edits.
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build          # → /app/dist

# ---- 2. build the Go binaries ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/moltnetd ./cmd/moltnetd \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/molt ./cmd/molt

# ---- 3. runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/moltnetd /usr/local/bin/moltnetd
COPY --from=build /out/molt /usr/local/bin/molt
COPY --from=web /app/dist /app
EXPOSE 8787
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/moltnetd"]
CMD ["--db", "/data/moltnet.db", "--addr", ":8787", "--app", "/app", "--rate-limit", "120"]
# Health: GET /healthz (orchestrators should probe it; distroless has no shell).
