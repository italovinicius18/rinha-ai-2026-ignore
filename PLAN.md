# Rinha de Backend 2026 — Pure Go submission plan

> Persisted plan. Update this file when decisions change. Day-to-day progress is logged in `docs.md`.

## 1. Constraints

| Axis | Value | Implication |
|---|---|---|
| Endpoints | `GET /ready`, `POST /fraud-score` on port 9999 | Tiny surface — no router needed |
| Topology | ≥1 LB + ≥2 API instances, bridge net | LB = HAProxy (NOT app logic) |
| Total budget | **1 CPU, 350 MB RAM** | The hardest constraint |
| Dataset | **3M** labeled 14-dim vectors (≈284 MB raw JSON, 16 MB gz) | Must shrink + share across replicas |
| Load | k6 ramping → **900 rps** for 120 s (~54 k requests) | Tail latency is what matters |
| Scoring | `score_p99` (log10, sat at 1 ms = +3000) + `score_det` (FP=1, FN=3, Err=5; −3000 if failure>15%) | Optimize p99, never 500 |

Two governing observations:

- **Memory math.** Two copies of float32 vectors = 2 × 168 MB > 350 MB. Either quantize aggressively (int8 → ~42 MB) or share a single copy via mmap from a host volume.
- **Latency math.** Brute-force KNN over 3M × 14 dims at 900 rps under 1 CPU is borderline. Plan for an index.

## 2. Target architecture

```
                    +---------------+
client (port 9999)→ |   haproxy     |  round-robin only, no L7 logic
                    +-------+-------+
                            |
              +-------------+-------------+
              ↓                           ↓
        +-----------+               +-----------+
        |  api-1    |               |  api-2    |   pure Go, net/http,
        | (Go bin)  |               | (Go bin)  |   mmaps /data/index.bin
        +-----+-----+               +-----+-----+
              \                           /
               \                         /
                +------ shared volume --+
                | /data/index.bin        |
                | (precomputed offline)  |
                +------------------------+
```

- **LB:** HAProxy (smaller footprint than nginx; ~10–20 MB). Pure-Go restriction applies to the *API*, not the LB.
- **API:** single static Go binary built with `CGO_ENABLED=0`. Stdlib `net/http`. Two replicas.
- **Index:** built at Docker build time, written to a shared volume (or baked into image, mmap'd `MAP_SHARED` so OS page-cache deduplicates).

### Suggested resource split (1 CPU / 350 MB)

| Service | CPU | Memory |
|---|---|---|
| haproxy | 0.10 | 20 MB |
| api-1 | 0.45 | 160 MB |
| api-2 | 0.45 | 160 MB |
| **total** | **1.00** | **340 MB** |

## 3. Data pipeline (build time, once)

1. Stream-parse `references.json.gz` with `compress/gzip` + `encoding/json` decoder (token mode).
2. Quantize each of 14 dims: [0,1] → uint8 [0,255]; encode the −1 sentinel as a reserved byte value or a parallel bit mask.
3. Layout the binary (little-endian):
   ```
   header (64 B): magic | version | count | dim | quant_scale | crc32 | reserved
   vectors      : count × 14 × uint8     (≈ 42 MB)
   labels       : count × 1 bit packed   (≈ 376 KB)
   index meta   : IVF centroids + posting lists (see §4)
   ```
4. Include CRC32 in the header — API refuses a mismatched index.

Lives in `cmd/build-index/main.go`. Multi-stage Dockerfile keeps the gz out of the runtime image.

## 4. Search algorithm

14 dimensions is *low*:

- **Brute force** with SIMD-friendly int8: ~42 M byte-ops per query, likely 30–80 ms.
- **IVF** (k-means coarse quantizer, `nlist≈1024`, `nprobe=8`): touches ~24 k vectors per query → sub-ms.
- **HNSW**: textbook answer but graph memory (~20 B/node) is bad for our budget.
- **KD-tree / VP-tree**: workable but degrade vs. IVF at this dim.

Recommendation in matrix: **IVF int8 + IVF-PQ** as the two front-runners; brute-force variants as oracles.

## 5. HTTP server design

Pure stdlib:

- One `net/http.Server` per API, single mux with two handlers.
- `SetKeepAlivesEnabled(true)`, all timeouts set explicitly.
- `sync.Pool` for: response buffer, decoded request struct, `[14]uint8` quantized vector, top-5 heap.
- Pre-encode the 12 possible response bodies (`approved ∈ {true,false}` × `fraud_score ∈ {0, 0.2, 0.4, 0.6, 0.8, 1.0}`) and `w.Write` one of them.
- Skip the request `id`.
- Custom RFC3339 fast-path for `requested_at` (only hour & day-of-week needed).

## 6. Index loading at boot

- `mmap` `/data/index.bin` with `PROT_READ | MAP_SHARED` via `syscall.Mmap` (or `golang.org/x/sys/unix`).
- Verify magic + CRC, expose typed slices over the mapped bytes (no copies).
- `madvise(MADV_RANDOM)` for posting lists.
- `GET /ready` returns 200 only after the index is verified.

## 7. Deployment artifacts

- `Dockerfile` (multi-stage): `golang:1.23-alpine` builds → `scratch` or `gcr.io/distroless/static` runs.
- `docker-compose.yml` on `submission` branch: haproxy + api-1 + api-2 + named volume for the index.
- `info.json` at repo root.

## 8. Validation strategy

- **Smoke:** `k6 run test/smoke.js`.
- **Correctness oracle:** brute-force exact KNN under a build tag; measure IVF recall vs. exact.
- **Full load:** `bash run.sh` from official repo; aim p99 < 5 ms, failure_rate well under 15 %.
- **Memory:** `docker stats` during the run; no OOM-kill.

## 9. Repo layout

```
.
├── cmd/
│   ├── api/main.go               # baseline HTTP server (gets promoted to winning experiment)
│   └── build-index/main.go       # offline preprocessing, --format flag
├── internal/
│   ├── vec/                      # vectorize + normalize (14 dims, clamp, mcc_risk)
│   ├── quantize/                 # float32 ↔ int8
│   ├── ivf/                      # IVF index build/query
│   ├── mmap/                     # syscall.Mmap wrapper
│   ├── httpx/                    # response pool, JSON fast-paths
│   └── bench/                    # local k6 runner + result parser
├── experiments/
│   ├── exp01-brute-float32/
│   ├── exp02-brute-int8/
│   ├── exp03-brute-int8-asm/
│   ├── exp04-ivf-int8/
│   ├── exp05-ivf-pq/
│   ├── exp06-bucket-lut/
│   └── (exp07-decision-tree, exp08-no-net-http — backlog)
├── bench/
│   ├── run-all.sh
│   ├── compare.go
│   └── leaderboard.md            # auto-generated, committed
├── submission/                   # mirror of winning experiment
├── Dockerfile
├── docker-compose.yml
├── haproxy.cfg
├── info.json
├── PLAN.md
├── docs.md                       # running development log
└── README.md
```

## 10. Phased execution

| Phase | Goal | Exit criterion |
|---|---|---|
| **P1 — Skeleton** | Hardcoded `/fraud-score` response, LB+2 replicas wired, compose passes smoke.js | All checks green |
| **P2 — Vectorize** | `internal/vec` end-to-end; unit tests against DETECTION_RULES.md flow examples | Vectors match spec |
| **P3 — Index builder** | `cmd/build-index --format=flat` produces a baseline binary | File on disk, CRC verified |
| **P4 — exp01 brute-float32** | Wire end-to-end, get a *correct* slow baseline | First leaderboard entry |
| **P5 — exp02..exp03** | Quantization + SIMD asm | Decide whether ANN is needed |
| **P6 — exp04..exp05** | IVF and IVF-PQ | Pick search winner |
| **P7 — exp06** | Bucket-LUT layered on the winner | Confirm or roll back |
| **P8 — Hot-path tuning** | sync.Pool, pre-encoded responses, GOGC, GOMAXPROCS | Leaderboard delta |
| **P9 — Submission branch** | Mirror winning experiment to `submission/` | PR opened |

## 11. Decisions locked in

- **LB:** HAProxy.
- **Dependencies:** stdlib + `golang.org/x/sys` (for `unix.Mmap` with `MADV_RANDOM`). No web frameworks.
- **HNSW dropped** from first matrix wave.
- **Custom HTTP server** is a follow-up against the winning search variant only.

## 12. Experiment harness

### Shared contract

Every `experiments/expXX-*/main.go` exposes:

```go
type Searcher interface {
    LoadIndex(path string) error
    Query(qv [14]uint8) (fraudCount uint8) // 0..5
}
```

The HTTP server, vectorize, response encoding, and mmap glue come from `internal/`. Switching experiments = changing one import + one binary.

### Index format negotiation

`cmd/build-index --format=<name> --out=/data/expXX.bin` produces a binary tagged with a 4-byte format magic. Each experiment refuses to start if magic doesn't match.

### Bench harness

`bench/run-all.sh`:
1. For each `experiments/expXX-*/`: build + compose up + wait /ready + `k6 run test/test.js` + capture `test/results.json` + `docker stats --no-stream` + compose down.
2. `go run bench/compare.go` renders `bench/leaderboard.md`.

### Recommended matrix

| # | Experiment | Tests which fork | Hypothesis | Stop condition |
|---|---|---|---|---|
| **exp01** | `brute-float32` | Correctness oracle | Recall = 100%, p99 ≈ 200 ms | Truth for recall in others |
| **exp02** | `brute-int8` | Quantization cost | Recall ≥ 99.5%, p99 ≈ 50 ms | Confirms int8 acceptable |
| **exp03** | `brute-int8-asm` | AVX2 SIMD kernel | Same recall as exp02, p99 ≈ 10 ms | If close to ANN, simplicity wins |
| **exp04** | `ivf-int8` | IVF baseline | p99 < 5 ms, recall ≥ 97% | Baseline ANN |
| **exp05** | `ivf-pq` | IVF-PQ | p99 < 2 ms, mem < 100 MB, recall ≥ 96% | Likely winner |
| **exp06** | `bucket-lut` | Result memoization | p99 < 1 ms post-warmup; risk = recall | Layer on top of winner |

### Rules to keep the matrix honest

- **One variable per experiment.**
- **Recall measured vs. exp01**, not vs. test labels.
- **Three k6 runs per experiment**, report median final_score.
- **`leaderboard.md` is committed**; each commit references experiment + final_score.
