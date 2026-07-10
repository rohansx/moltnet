# syntax=docker/dockerfile:1
# Multi-stage build producing a tiny, static, non-root moltnetd image.
# The binary is pure Go (no cgo — modernc.org/sqlite), so a distroless static
# base is sufficient. Build command mirrors the one verified in CI.

# ---- build ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/moltnetd ./cmd/moltnetd \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/molt ./cmd/molt

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/moltnetd /usr/local/bin/moltnetd
COPY --from=build /out/molt /usr/local/bin/molt
COPY --from=build /src/web /web
EXPOSE 8787
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/moltnetd"]
CMD ["--db", "/data/moltnet.db", "--addr", ":8787", "--web", "/web"]
# Health: GET /healthz (orchestrators should probe it; distroless has no shell).
