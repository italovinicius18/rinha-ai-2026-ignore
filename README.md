# Rinha de Backend 2026 — submission

Runtime files only. Source code lives on the `main` branch:
https://github.com/italovinicius18/rinha-ai-2026-ignore

## Stack

- **API**: pure Go (stdlib `net/http`), IVF-F32 ANN (C=512 centroids, nprobe=8) over 3M labeled 14-dim vectors.
- **LB**: HAProxy 2.9 (single-thread event loop), `http-reuse always`.
- **Index**: 168 MB float32, MAP_SHARED-mmapped from a shared volume → 0 RSS per replica.

## Resource budget (total 1.00 CPU / 350 MB)

| service | CPU | memory |
|---|---:|---:|
| haproxy | 0.15 | 15 MB |
| api1 | 0.40 | 155 MB |
| api2 | 0.40 | 155 MB |
| data-prep | 0.05 | 25 MB |

## Images

- `italovinicius18/rinha-2026-api:v1`
- `italovinicius18/rinha-2026-data-prep:v1` (ships the ~168 MB IVF-F32 index baked in)
