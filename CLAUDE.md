# Project memory — Rinha de Backend 2026 (pure Go)

This file is loaded automatically. Read `PLAN.md` and `docs.md` next — they hold the design and the running journal. Do not duplicate them here.

## What this project is

A submission for the **Rinha de Backend 2026** challenge — fraud-detection API via k=5 KNN over 3M labeled 14-dim vectors, written in **pure Go (stdlib `net/http`, no web frameworks)**. Read the upstream docs at `../rinha-de-backend-2026/docs/en/` (especially `DETECTION_RULES.md`, `EVALUATION.md`, `ARCHITECTURE.md`).

## Repo layout

- `PLAN.md` — static plan: constraints, architecture, experiment matrix, phased execution.
- `docs.md` — running development log. **Every iteration appends one entry.** Read the latest entry to see where work stopped and the "Next iteration agenda" block at the end of it.
- `cmd/api/` — placeholder baseline binary (P1/P2). Not used by experiments.
- `cmd/build-index/` — offline index builder. Streams `references.json.gz` → `internal/idxfmt` binary.
- `internal/vec/` — request payload → 14-dim vector. Tested against both spec flow examples.
- `internal/idxfmt/` — on-disk file format (magic `RINH2026`, 64-byte header, body + bit-packed labels, CRC32). Formats: `FormatFlat32=1` (float32, 168 MB), `FormatInt8=2` (uint8, 42 MB). Has `FlatWriter`, `Int8Writer`, `FlatReader`, `QuantizeFloat64`, and an `inspect/` debug CLI.
- `internal/mmapfile/` — stdlib `syscall.Mmap` wrapper. `MAP_SHARED`, `PROT_READ`.
- `internal/brutef32/` — brute-force k=5 Euclidean KNN. Correctness oracle. `unsafe`-casts the mmap'd bytes to `*[14]float32`.
- `internal/brutei8/` — brute-force k=5 KNN over FormatInt8 index. uint32 squared distance. Same API as brutef32.
- `cmd/recall-check/` — offline accuracy harness. Walks `test-data.json`, compares predicted vs expected, prints TP/TN/FP/FN/accuracy.
- `experiments/expXX-*/` — one folder per benchmarked strategy. Each has its own `main.go`, `Dockerfile`, `docker-compose.yml`, `haproxy.cfg`, `README.md`, `results/`.
- `bench/leaderboard.md` — single source of truth for experiment outcomes. Committed.
- `resources/references.json.gz` — vendored from upstream. **Gitignored.**

## Working directory

`/home/italo/rinha-2026/solution/`. The two clones beside it (`rinha-de-backend-2026/`, `rinha-de-backend-2025/`) are **read-only references** — do not modify.

## Hard environmental facts

- **Go is NOT installed locally.** Use Docker (`golang:1.23-alpine`) for builds + tests. There's no `go` in PATH and no `golang-*` apt package.
- **Docker:** rootless, v29.x. `docker info | head` shows `Context: rootless`.
- **k6:** also not installed; use `grafana/k6:latest` image.
- **WSL2 on Linux 6.6, i7-14700HX.** Mac-Mini-Late-2014 (test rig) is ~2× slower per core. Halve all single-core benchmark numbers when projecting to the test rig.

## Build + test incantations

```bash
# All unit tests
docker run --rm -v "$PWD":/src -w /src golang:1.23-alpine \
    sh -c 'go vet ./... && go test ./...'

# Build the index from the vendored references (writes to /tmp/rinha-out/full.bin)
mkdir -p /tmp/rinha-out
docker run --rm \
  -v "$PWD":/src \
  -v "$PWD/resources":/refs:ro \
  -v /tmp/rinha-out:/out \
  -w /src golang:1.23-alpine sh -c '
    go build -o /usr/local/bin/build-index ./cmd/build-index &&
    build-index --in=/refs/references.json.gz --out=/out/full.bin'
# Expect: 3M entries, 168375064 bytes, ~6s, heap ~5MB.

# Bench brutef32 on the real 3M index
docker run --rm \
  -v "$PWD":/src -v /tmp/rinha-out:/out:ro \
  -e RINHA_INDEX=/out/full.bin \
  -w /src golang:1.23-alpine sh -c '
    go test -run=^$ -bench=BenchmarkSearchKNN_Full3M -benchtime=10x -count=1 ./internal/brutef32'
# Expect: ~14.5 ms/query on this WSL i7.

# Run an experiment
cd experiments/exp01-brute-float32
docker compose up -d --build
until [ "$(curl -s -o /dev/null -w '%{http_code}' http://localhost:9999/ready)" = "200" ]; do sleep 1; done

# k6 smoke
docker run --rm -i --network host \
  -v /home/italo/rinha-2026/rinha-de-backend-2026/test:/test:ro \
  grafana/k6:latest run /test/smoke.js

# k6 full load — needs writable test/ subdir for handleSummary
rm -rf /tmp/k6-stage && mkdir -p /tmp/k6-stage/test
cp /home/italo/rinha-2026/rinha-de-backend-2026/test/*.{js,json} /tmp/k6-stage/test/
chmod -R 777 /tmp/k6-stage
docker run --rm -i --network host \
  -v /tmp/k6-stage:/work -w /work --user "$(id -u):$(id -g)" \
  grafana/k6:latest run test/test.js
cat /tmp/k6-stage/test/results.json
docker compose -p rinha-2026-exp01 down
```

## Non-obvious gotchas (learned the hard way)

- **`test.js` `handleSummary` writes to relative `test/results.json`.** k6 must run with a working directory that *contains* a writable `test/` subdir. Mounting the upstream `rinha-de-backend-2026/test` read-only fails; mounting it writable but with the wrong uid also fails. The working incantation is the `/tmp/k6-stage` staging dance above.
- **`docker compose deploy.resources.limits` is honored by compose v2 without Swarm.** No special flags needed.
- **`MAP_SHARED` mmap of a file in a named volume from two containers → page cache deduplicates** → file-backed RSS does **not** count toward either consumer's cgroup memory. Confirmed in P4b. This is the only way to fit a 168 MB index inside the 350 MB total budget with two replicas.
- **Day-of-week convention:** Go's `time.Weekday` is Sun=0..Sat=6; the spec is Mon=0..Sun=6. Mapping: `(int(t.Weekday())+6) % 7`.
- **The vector at dim 5/6 uses `-1` as a sentinel when `last_transaction` is null.** The dataset uses the same convention — don't filter or replace.
- **`json.Marshal(float64(0.0))` emits `0`, not `0.0`.** The smoke check `typeof === 'number'` accepts both, but worth knowing when hand-rolling responses.
- **Pre-encoded responses:** only 6 possible `fraud_score` values (`{0, 0.2, 0.4, 0.6, 0.8, 1.0}`) since k=5. exp01 already uses `responseBodies [6][]byte`.

## Current state (as of last iteration in docs.md)

Completed: **P1 → P10** (skeleton, vectorize, index builder, mmap reader, brutef32, IVF format/search, IVF-F32 fast unsafe.Pointer search, per-request profiling, HAProxy CFS-throttle fix).

**BEST result: exp17 (IVF-F32, C=512, nprobe=8, HAProxy 0.15 CPU):** `final_score ≈ +5557` (stable median of two runs: 5575 / 5538). p99 = **1.7 ms**, 1 FP, 1 FN, 0 http_errors. Same model as exp09 — the +1991-point jump is **entirely from rebalancing the 1 CPU budget**, not algorithm changes.

**Key discovery (P9/P10):** Handler-only timing was ~290 µs typical / 770 µs p99, but k6 reported p99 = 170 ms. Sequential probe of `/ready` showed ~100 ms spikes at the CFS scheduling period — HAProxy at `cpus: "0.05"` got a 5 ms quota per 100 ms period and blocked once exhausted. `docker stats` showed HAProxy pegged at 100% of its limit. Giving HAProxy headroom resolves it cleanly:

| HAProxy CPU | api CPU each | p99 | score |
|---:|---:|---:|---:|
| 0.05 (exp09) | 0.45 | 172 ms | +3566 |
| 0.10 (exp16) | 0.425 | 21 ms | +4460 |
| **0.15 (exp17)** | **0.40** | **1.7 ms** | **+5557** |

**Key discovery (P7):** All 70 FP/FN errors in exp04 were caused by **int8 quantization distortion**, not ANN cluster misses. FormatIVF_F32 (float32 cluster vectors) eliminates this — 69/70 errors drop.

**CRITICAL ivf.go rule (P8):** The centroid scan `for j := 0; j < Dim; j++ { d += (c[j]-q[j])^2 }` over `[]float32` is auto-vectorized by Go's compiler (SIMD). Do NOT apply unsafe.Pointer unrolling to this loop — it disables vectorization and causes a 2× regression. Only `searchAndCountF32` benefits from the unsafe.Pointer+unroll pattern (because mmap data is `[]byte`, not typed).

**P8 finding:** C=1024 centroids did NOT help. Centroid scan (57KB data) spills L1 cache; non-uniform cluster sizes (max=13908) hurt p99. C=512 with its 28KB centroid data (fits L1) remains optimal.

**CFS-throttle rule (P10):** Under a tight CPU budget, each container's measured CPU% should sit well below its cgroup limit (target < 70%). If `docker stats` shows a container near 100% of its limit, CFS will block it for the rest of every 100 ms period — p99 spikes to ~100 ms regardless of what the application does. **Watch utilization-vs-limit, not absolute CPU%.**

**Key index sizes:** FormatFlat32 = 168 MB (MAP_SHARED). FormatIVF (int8) = 42 MB. FormatIVF_F32 (float32) = 168 MB (MAP_SHARED → 0 RSS per replica).

**Promotion (P11, 2026-05-13):** exp17's config is now the canonical submission. `cmd/api/main.go`, top-level `Dockerfile`, `docker-compose.yml`, and `haproxy.cfg` have been overwritten with the exp17 versions (build context adjusted from `experiments/exp17-haproxy-cpu2/` to `./cmd/api`). Verified on a fresh build: smoke 5/5, full k6 = p99 1.35ms / score **+5658.57** (`bench/results/canonical-promote-2026-05-13.json`).

**Next agenda:** fill in `info.json` (participant name + GitHub link), then optionally test the cold-rebuild path on a clean Docker daemon and explore HAProxy 0.20 / api 0.375 if any p99 headroom is desired.

## How to work on this project

1. **Read `docs.md`'s last entry first.** The "Next iteration agenda" tells you exactly what to do.
2. **Create tasks via TaskCreate** before doing multi-step work; mark `in_progress` as you go.
3. **Every iteration appends one entry to `docs.md`** with sections: Goals, Decisions captured, Files committed, Verification, Outcome, Open follow-ups, Next iteration agenda.
4. **Bench results go to `bench/leaderboard.md`** (overwrite the row for that experiment; history is in `git log` + `experiments/expXX/results/`).
5. **Do not modify PLAN.md unless a structural decision changes.** Day-to-day decisions belong in docs.md.
6. **Test via Docker, not local Go.** See incantations above.
7. **Smoke is `k6 smoke.js`.** Full is `k6 test.js`. The full takes ~2 minutes.
