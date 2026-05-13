# syntax=docker/dockerfile:1.7
#
# Build context: solution/ root.
# Stages:
#   builder      — compiles cmd/build-index and the exp02 API binary
#   index-baker  — runs build-index --format=int8 against resources/references.json.gz
#                  emits /data/index.bin (~42 MB, FormatInt8, CRC-validated)
#   data-prep    — copies /data/index.bin into a shared volume and exits
#   api          — distroless runtime for the API binary

FROM --platform=linux/amd64 golang:1.23-alpine AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

COPY go.mod ./
RUN go mod download || true

COPY cmd/         ./cmd/
COPY internal/    ./internal/
COPY experiments/ ./experiments/

RUN go build -trimpath -ldflags="-s -w" -o /out/api          ./experiments/exp02-brute-int8 \
 && go build -trimpath -ldflags="-s -w" -o /out/build-index  ./cmd/build-index

# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 alpine:3.20 AS index-baker
WORKDIR /work
COPY --from=builder /out/build-index /usr/local/bin/build-index
COPY resources/references.json.gz ./refs.json.gz
RUN mkdir -p /data \
 && build-index --in=./refs.json.gz --out=/data/index.bin --format=int8 \
 && rm ./refs.json.gz \
 && ls -la /data/

# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 busybox:1.36 AS data-prep
COPY --from=index-baker /data/index.bin /seed/index.bin
ENTRYPOINT ["sh", "-c", "mkdir -p /shared && cp /seed/index.bin /shared/index.bin && echo 'data-prep: index in place ('$(stat -c%s /shared/index.bin)' bytes)'"]

# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 gcr.io/distroless/static:nonroot AS api
COPY --from=builder /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
