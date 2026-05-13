# syntax=docker/dockerfile:1.7
#
# Build context: solution/ root.
# Stages:
#   builder      — compiles cmd/build-index and the exp01 API binary
#   index-baker  — runs build-index against resources/references.json.gz
#                  and emits /data/index.bin (~168 MB, FLAT32, CRC-validated)
#   data-prep    — runtime image used by the "data-prep" compose service;
#                  copies /data/index.bin into a shared volume and exits
#   api          — distroless runtime for the API binary

FROM --platform=linux/amd64 golang:1.23-alpine AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

COPY go.mod ./
RUN go mod download || true

COPY cmd/         ./cmd/
COPY internal/    ./internal/
COPY experiments/ ./experiments/

RUN go build -trimpath -ldflags="-s -w" -o /out/api          ./experiments/exp01-brute-float32 \
 && go build -trimpath -ldflags="-s -w" -o /out/build-index  ./cmd/build-index

# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 alpine:3.20 AS index-baker
WORKDIR /work
COPY --from=builder /out/build-index /usr/local/bin/build-index
COPY resources/references.json.gz ./refs.json.gz
RUN mkdir -p /data \
 && build-index --in=./refs.json.gz --out=/data/index.bin \
 && rm ./refs.json.gz \
 && ls -la /data/

# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 busybox:1.36 AS data-prep
COPY --from=index-baker /data/index.bin /seed/index.bin
# Copy seed to the named volume, then exit 0. Compose's
# `service_completed_successfully` gates api1/api2 until this succeeds.
ENTRYPOINT ["sh", "-c", "mkdir -p /shared && cp /seed/index.bin /shared/index.bin && echo 'data-prep: index in place ('$(stat -c%s /shared/index.bin)' bytes)'"]

# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 gcr.io/distroless/static:nonroot AS api
COPY --from=builder /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
