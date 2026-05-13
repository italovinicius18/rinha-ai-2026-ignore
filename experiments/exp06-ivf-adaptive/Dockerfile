# syntax=docker/dockerfile:1.7
#
# Build context: solution/ root.
# Stages:
#   builder      — compiles build-index, build-ivf, and the exp06 API binary
#   index-baker  — builds FLAT32 index, then IVF index (k-means C=512)
#   data-prep    — copies IVF index into shared volume and exits
#   api          — distroless runtime

FROM --platform=linux/amd64 golang:1.23-alpine AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

COPY go.mod ./
RUN go mod download || true

COPY cmd/         ./cmd/
COPY internal/    ./internal/
COPY experiments/ ./experiments/

RUN go build -trimpath -ldflags="-s -w" -o /out/api          ./experiments/exp06-ivf-adaptive \
 && go build -trimpath -ldflags="-s -w" -o /out/build-index  ./cmd/build-index \
 && go build -trimpath -ldflags="-s -w" -o /out/build-ivf    ./cmd/build-ivf

# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 golang:1.23-alpine AS index-baker
COPY --from=builder /out/build-index /usr/local/bin/build-index
COPY --from=builder /out/build-ivf   /usr/local/bin/build-ivf
COPY resources/references.json.gz /tmp/refs.json.gz
RUN mkdir -p /data \
 && echo "Step 1: building FLAT32 index..." \
 && build-index --in=/tmp/refs.json.gz --out=/tmp/flat.bin --format=flat \
 && echo "Step 2: building IVF index (k-means C=512)..." \
 && build-ivf --in=/tmp/flat.bin --out=/data/index.bin --centroids=512 --iters=30 \
 && rm /tmp/flat.bin /tmp/refs.json.gz \
 && ls -la /data/

# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 busybox:1.36 AS data-prep
COPY --from=index-baker /data/index.bin /seed/index.bin
ENTRYPOINT ["sh", "-c", "mkdir -p /shared && cp /seed/index.bin /shared/index.bin && echo 'data-prep: IVF index in place ('$(stat -c%s /shared/index.bin)' bytes)'"]

# -----------------------------------------------------------------------------
FROM --platform=linux/amd64 gcr.io/distroless/static:nonroot AS api
COPY --from=builder /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
