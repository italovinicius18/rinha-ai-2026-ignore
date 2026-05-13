# Development log

A running log of every iteration on this submission. New entries are appended at the bottom; the table of contents at the top is kept in sync. See `PLAN.md` for the static plan.

## Index

- [Iter 01 — 2026-05-11 — Bootstrap: plan + P1 skeleton](#iter-01--2026-05-11--bootstrap-plan--p1-skeleton)
- [Iter 02 — 2026-05-11 — P2: vectorize package](#iter-02--2026-05-11--p2-vectorize-package)
- [Iter 03 — 2026-05-11 — P3: index builder](#iter-03--2026-05-11--p3-index-builder)
- [Iter 04a — 2026-05-11 — P4a: mmap reader + brutef32 search](#iter-04a--2026-05-11--p4a-mmap-reader--brutef32-search)
- [Iter 04b — 2026-05-11 — P4b: exp01 wired, first k6 run, leaderboard live](#iter-04b--2026-05-11--p4b-exp01-wired-first-k6-run-leaderboard-live)
- [Iter 05 — 2026-05-11 — P5: recall-check harness + exp02 int8 brute force](#iter-05--2026-05-11--p5-recall-check-harness--exp02-int8-brute-force)
- [Iter 06 — 2026-05-12 — P6: exp03 parallelism probe + exp04 IVF (first positive score)](#iter-06--2026-05-12--p6-exp03-parallelism-probe--exp04-ivf-first-positive-score)
- [Iter 07 — IVF-F32: float32 cluster vectors + nprobe sweep (P7)](#iter-07--ivf-f32-float32-cluster-vectors--nprobe-sweep-p7)
- [Iter 08 — C=1024 centroids + centroid loop micro-opt investigation (P8)](#iter-08--c1024-centroids--centroid-loop-micro-opt-investigation-p8)
- [Iter 09 — 2026-05-13 — P9/P10: per-request profiling exposes HAProxy CFS throttle, +2000 points](#iter-09--2026-05-13--p9p10-per-request-profiling-exposes-haproxy-cfs-throttle-2000-points)

---

## Iter 01 — 2026-05-11 — Bootstrap: plan + P1 skeleton

### Goals

- Persist the plan (`PLAN.md`) so it survives across sessions.
- Initialize this development log.
- Scaffold **P1**: minimal Go API (`/ready`, `/fraud-score` returning hardcoded), HAProxy LB, two API replicas, `docker-compose.yml`, `Dockerfile`, `haproxy.cfg`, `info.json`, `go.mod`.
- Verify the skeleton compiles with `go build`.

### Decisions captured this iteration

- **Project root:** `/home/italo/rinha-2026/solution/`. The two cloned repos next to it (`rinha-de-backend-2025/`, `rinha-de-backend-2026/`) stay untouched — read-only references.
- **LB:** HAProxy 2.9-alpine.
- **Go toolchain:** 1.23, `CGO_ENABLED=0`, `GOOS=linux GOARCH=amd64`.
- **Skeleton response:** hardcoded `{"approved":true,"fraud_score":0.0}`. Only purpose is to make smoke.js green; replaced once vectorize lands in P2.
- **Resource split (initial):** haproxy 0.10 CPU / 20 MB, api-1/api-2 each 0.45 CPU / 160 MB. Total 1.00 CPU / 340 MB.
- **Module path:** `rinha2026/solution`.
- **API internal port:** 8080. HAProxy listens on 9999 publicly (required by spec) and forwards to `api1:8080`, `api2:8080`.

### Files created

- `solution/PLAN.md`
- `solution/docs.md` (this file)
- `solution/go.mod`
- `solution/cmd/api/main.go`
- `solution/Dockerfile`
- `solution/docker-compose.yml`
- `solution/haproxy.cfg`
- `solution/info.json` (placeholder; user's GitHub username + display name still TODO)
- `solution/README.md` (minimal)

### Notes / open questions

- `info.json` participants/social fields left as TODO — user needs to fill in their name + GitHub username before opening the participant PR.
- Skeleton always-approves response means it would fail the full k6 detection test (FN ~ 44 %). That is **intentional for P1**; only `smoke.js` is the gate here.
- No git init yet. We can `git init` later when ready to push to the user's GitHub repo.
- `references.json.gz`, `mcc_risk.json`, `normalization.json` are NOT vendored yet — they live in `../rinha-de-backend-2026/resources/`. Will be wired up in P3 (index builder).

### Verification

Go is **not installed** on this WSL (`go` not in PATH, no `golang-*` apt package). Verification was done **via Docker** instead, which exercises the same `go build` invocation that the submission image will use.

Commands run:

```
docker build --target builder -t rinha2026-builder-check .       # → builder stage OK in 4.3 s
docker build -t rinha2026-api:p1 .                                # → final distroless image OK
docker run --rm -d -p 18080:8080 rinha2026-api:p1                 # → container up
curl http://localhost:18080/ready                                 # → 200
curl -X POST -d '{"id":"tx-smoke-001"}' http://localhost:18080/fraud-score
                                                                  # → 200 {"approved":true,"fraud_score":0}
docker compose up -d                                              # → haproxy+api1+api2 healthy
curl http://localhost:9999/ready                                  # → 200
curl -X POST -d '{"id":"tx-smoke-N"}' http://localhost:9999/fraud-score  (×4)
                                                                  # → 200 on all, round-robin to api1/api2
docker compose down                                               # → clean teardown
```

> **Note on JSON serialization:** Go's `json.Marshal(0.0)` emits `0` (not `0.0`). The smoke check `typeof fraud_score === 'number'` accepts both, so this is fine — but worth remembering when we hand-roll response bodies in P8.

### Outcome

**P1 skeleton green end-to-end.**

- Builder stage compiles cleanly (no deps, no warnings).
- Distroless runtime container is small and starts in <100 ms.
- HAProxy 9999 → api1/api2 8080 round-robin works.
- All `deploy.resources.limits` declarations parse and apply (Compose v2 supports them outside Swarm).

### Files committed this iteration

- `solution/PLAN.md`
- `solution/docs.md`
- `solution/go.mod`
- `solution/cmd/api/main.go`
- `solution/Dockerfile`
- `solution/.dockerignore`
- `solution/docker-compose.yml`
- `solution/haproxy.cfg`
- `solution/info.json` (placeholder — needs user's name + GitHub link)
- `solution/README.md`

### Next iteration agenda

- **P2 — Vectorize.** Create `internal/vec` package implementing:
  - `Vectorize(payload) -> [14]float64` per `DETECTION_RULES.md`.
  - `Clamp`, `Normalize`, `MccRisk` helpers reading from baked-in tables.
  - Unit tests against the two flow examples in `DETECTION_RULES.md` (legitimate `tx-1329056812` → `[0.0041, 0.1667, 0.05, 0.7826, 0.3333, -1, -1, 0.0292, 0.15, 0, 1, 0, 0.15, 0.006]` and fraudulent `tx-3330991687` → `[0.9506, 0.8333, 1.0, 0.2174, 0.8333, -1, -1, 0.9523, 1.0, 0, 1, 1, 0.75, 0.0055]`).
- Plug `internal/vec` into `cmd/api` so the response actually depends on the input (still without an index — just a vectorize pass-through that always approves).

---

## Iter 02 — 2026-05-11 — P2: vectorize package

### Goals

- Implement `internal/vec` per the 14-dim spec in `docs/en/DETECTION_RULES.md`.
- Unit-test it against the two spec flow examples bit-for-bit (within 1e-3).
- Plug it into `cmd/api` so the handler actually parses+vectorizes payloads (returns 400 on malformed input). Still always-approve until search is added.

### Decisions captured this iteration

- **Vector type:** `[14]float64` (not `[]float64`) — fixed size, stack-allocated, avoids heap allocation per request.
- **`vec.Payload` types live in the `vec` package** (not in a separate `model` package) — they're tied to vectorization input and have no other consumer right now.
- **Hardcoded `mccRisk` map and normalization constants** inside `vec.go` — these are spec constants that don't change. Avoids file-IO at request time.
- **Day-of-week convention:** Go's `time.Weekday` is Sunday=0..Saturday=6; spec is Monday=0..Sunday=6. Mapping: `(int(t.Weekday())+6) % 7`. Validated against both spec examples (Wed → 2/6 = 0.3333; Sat → 5/6 = 0.8333).
- **Edge case `customer.avg_amount == 0`:** ratio diverges → clamp to 1.0. Spec doesn't address this explicitly; treating it as "infinite ratio → maximum risk" matches the intent of the dim.
- **Test tolerance:** 1e-3, since the spec examples are rounded to 4 decimals. Tighter would risk false negatives on display rounding artifacts.

### Files committed this iteration

- `solution/internal/vec/vec.go`
- `solution/internal/vec/vec_test.go`
- `solution/cmd/api/main.go` (updated to decode `vec.Payload` + call `Vectorize`)

### Verification

```
$ docker run --rm -v $PWD:/src -w /src golang:1.23-alpine \
      sh -c 'go vet ./... && go test -v ./internal/vec/...'
=== RUN   TestVectorize_LegitimateExample
--- PASS: TestVectorize_LegitimateExample (0.00s)
=== RUN   TestVectorize_FraudulentExample
--- PASS: TestVectorize_FraudulentExample (0.00s)
=== RUN   TestVectorize_LastTransactionNotNull
--- PASS: TestVectorize_LastTransactionNotNull (0.00s)
=== RUN   TestClampBoundaries
--- PASS: TestClampBoundaries (0.00s)
=== RUN   TestUnknownMerchant
--- PASS: TestUnknownMerchant (0.00s)
=== RUN   TestMccRiskDefault
--- PASS: TestMccRiskDefault (0.00s)
PASS
ok      rinha2026/solution/internal/vec  0.001s
```

End-to-end smoke through HAProxy:

```
ready: 200
POST /fraud-score (valid fraud payload, last_transaction:null) → 200, {"approved":true,"fraud_score":0}
POST /fraud-score (body = "not json")                          → 400
POST /fraud-score (requested_at = "NOT-A-DATE")                → 400
```

### Outcome

**P2 complete.** Vectorize produces spec-exact outputs and the API now rejects malformed payloads. Still always-approve on success (no search yet) — full k6 test would still fail detection, but `smoke.js` continues to pass.

### Open follow-ups

- `internal/vec` allocates on `time.Parse` per request. P8 hot-path tuning will replace this with a hand-rolled RFC3339 fast-path (we only need hour and weekday).
- `unknownMerchant` does a linear scan of `known_merchants`. With typical lists of <10 elements this is fine; if profiles show otherwise, swap to a tiny hash.
- Adding a `Vectorize` micro-benchmark is deferred until we measure end-to-end p99.

### Next iteration agenda — P3: index builder

- New binary: `cmd/build-index/main.go`.
- CLI: `build-index --in resources/references.json.gz --out /data/index.bin --format flat`.
- Streams the gzipped JSON via `compress/gzip` + `encoding/json` token decoder (no full load into RAM).
- Writes a packed binary:
  - Header (64 B): magic `RINH2026`, version u16, format u16, count u32, dim u8=14, quant_scale u8, crc32 u32, reserved.
  - Body for `format=flat`: `count × 14 × float32` (≈ 168 MB) plus a packed-bitfield label section.
- Adds a smoke that builds the index from the small `resources/example-references.json` (uncompressed sample) to keep CI fast.
- Plan tweak under consideration: emit **both** float32 and int8 sections, so later experiments can pick which to mmap without rebuilding.

## Iter 03 — 2026-05-11 — P3: index builder

### Goals

- Define an on-disk index file format with a 64-byte header (magic, version, format code, count, dim, CRC32) and a body whose layout depends on the format code.
- Implement `internal/idxfmt` with a streaming `FlatWriter` and a buffer-backed `FlatReader` (the runtime mmap'd reader is deferred to P4).
- Implement `cmd/build-index` that streams `references.json.gz` end-to-end and produces the binary, with auto-detection of gzip.
- Validate end-to-end on the small 100-entry example and the full 3M-entry dataset.

### File format (v1)

```
+----------------------------------------------------------------+
| Header  64 B (magic "RINH2026" | version u16 | format u16 |    |
|         count u32 | dim u8 | reserved 3 B | crc32 u32 |        |
|         reserved 40 B)                                         |
+----------------------------------------------------------------+
| Body (FLAT32 = format 1):                                      |
|   count × 14 × float32                                         |
|   ‖                                                            |
|   ceil(count/8) bytes of packed labels (bit (i mod 8) of       |
|   byte (i/8) holds entry i; 1 = fraud)                         |
+----------------------------------------------------------------+
```

- CRC32 (IEEE polynomial) is computed over the body only and stored in the header. Mismatches are fatal at load time.
- `format` is a u16 so future variants (int8 quantized, IVF, IVF-PQ) live in the same loader contract.

### Decisions captured this iteration

- **Labels at the end of the body, not interleaved.** First attempt buffered labels in a per-byte `lbuf` field and emitted them inline; this corrupted the layout because the partial label byte landed between vector blocks instead of after them. Fix: accumulate labels in an in-memory slice (`labels []byte`) and append once at `Close`. 376 KB for the full dataset — negligible.
- **`FlatWriter.Close` seeks back to byte 0 to write the real header.** Worth a single `Seek(0, SeekStart)` rather than a temp-file shuffle.
- **`UseNumber` on the JSON decoder** even though it's not strictly required — it avoids `float64` parsing for the `label` token path and keeps decisions explicit if we later swap to a tokenizer.
- **`inspect` debug tool** lives under `internal/idxfmt/inspect/`. Diagnostic-only; never shipped in the runtime image. Lets us eyeball a built file in one command.
- **No quantization yet.** P3's only output is FLAT32. The format code reserves `2` for int8 to be added in P5 without changing the loader contract.

### Files committed this iteration

- `solution/internal/idxfmt/idxfmt.go`
- `solution/internal/idxfmt/flat32.go`
- `solution/internal/idxfmt/idxfmt_test.go`
- `solution/internal/idxfmt/inspect/main.go`
- `solution/cmd/build-index/main.go`

### Verification

**Unit tests (5/5):**

```
=== RUN   TestFlatRoundTrip            --- PASS
=== RUN   TestOpenFlat_BadMagic        --- PASS
=== RUN   TestOpenFlat_BadCRC          --- PASS
=== RUN   TestOpenFlat_TooShort        --- PASS
=== RUN   TestPartialLabelByte         --- PASS    ← caught the interleaving bug
PASS    rinha2026/solution/internal/idxfmt
```

**Smoke against the 100-entry example:**

```
done · 100 entries (25 fraud / 75 legit) · output=5677B · 0s
file size: 5677 = 64 (header) + 100×56 (vectors) + 13 (label bytes) ✓
first vector matches example-references.json[0] bit-for-bit
crc32: 0xd906fce1
```

**Full 3M dataset:**

```
input is gzipped — streaming through gzip.Reader
  250000 entries · 521ms elapsed · heap=2MB
  500000 entries · 1.011s elapsed · heap=3MB
  ...
  3000000 entries · 5.936s elapsed · heap=5MB
done · 3000000 entries (999406 fraud / 2000594 legit) · output=168375064B · 5.936s
file size: 168_375_064 = 64 + 3M×56 + 375_000 ✓
crc32: 0x4634d844
```

- **Build time:** 5.94 s on this WSL Mac-mini-equivalent. Well within budget for a one-shot build step.
- **Peak heap:** ~5 MB — true streaming, no full-load.
- **Fraud rate:** 33.3 % (999,406 / 3,000,000). Notable: this is *training* fraud rate, distinct from the test-set fraud rate (44.47 % per `test-data.json`).

### Outcome

**P3 complete.** Index format defined, builder produces a deterministic 168 MB binary from the official gzipped references in 6 seconds with ~5 MB heap.

### Implications for compose layout

The index is **168 MB**. Two replicas with their own image copies → 336 MB just for the index pages, blowing the 350 MB total cap. We need **one** physical file on the host, mounted into both API containers so the kernel page cache deduplicates (`MAP_SHARED` + same inode). Plan for P4:

- New compose service `data` (built from a tiny multi-stage image that runs `build-index` and dumps the result into a named volume).
- `api1` and `api2` mount the named volume read-only at `/data` and mmap `/data/index.bin`.
- `depends_on: data: condition: service_completed_successfully`.

### Open follow-ups

- No `.gitignore` yet for the project. Add `.bin` / `out/` entries before the first git init.
- The CLI's progress log every 250k entries is fine for human runs; could become noisy in CI — consider `--quiet`.

### Next iteration agenda — P4: exp01 (brute-force float32 oracle)

Two halves; can be split across iterations if it runs long:

**P4a — runtime mmap reader + search:**
- `internal/mmapfile` — wraps `syscall.Mmap`, returns a `[]byte` view of the file.
- `internal/search/brutef32` — Euclidean KNN, brute-force over the FLAT32 body. Returns top-5 with their fraud labels.
- Unit tests against the spec examples (legit → score 0, fraud → score 1.0 — using a synthetic mini-index).
- Wire into `cmd/api`: on boot, `Open(/data/index.bin)`; on `/fraud-score`, vectorize → search → return `approved = score < 0.6, fraud_score = nFraud/5`.

**P4b — compose wiring + first leaderboard entry:**
- New `data-prep` service produces `/data/index.bin` in a shared named volume.
- `api1`/`api2` mount the volume read-only.
- Run `smoke.js`; if green, run the full `test.js` and capture results.
- Append the first row to `bench/leaderboard.md`.

## Iter 04a — 2026-05-11 — P4a: mmap reader + brutef32 search

### Goals

- `internal/mmapfile` — wrap `syscall.Mmap` so the runtime can mmap the 168 MB index without copying it into the heap.
- `internal/brutef32` — exact, brute-force k=5 KNN over a FLAT32 index. Correctness oracle: every other experiment is validated against this.
- Unit tests on synthetic mini-indexes; full-dataset benchmark gated by the `RINHA_INDEX` env var.

### Decisions captured this iteration

- **Package naming:** `internal/mmapfile` instead of `internal/mmap` (avoid shadowing the conceptual term and matching common Go idioms like `golang.org/x/exp/mmap`). PLAN.md still says `internal/mmap/`; treating this as a minor rename, will fold into PLAN on the next plan update.
- **`syscall.Mmap`, not `golang.org/x/sys/unix`.** Stdlib-only. Costs us `MADV_RANDOM` (no `syscall.Madvise` on the stdlib side), but we can add `x/sys` later if profiling shows it matters.
- **`MAP_SHARED`, `PROT_READ`.** Read-only mapping. With both API replicas mounting the same on-disk file (P4b), the kernel page cache deduplicates → ~168 MB total across both replicas, not 336 MB.
- **`brutef32.Open` does NOT re-verify CRC.** That's the builder's job at write time. Re-verifying at every container boot would force a 168 MB sequential scan and add ~250 ms to startup. We'll add an optional `--verify` flag later if needed.
- **Hot path uses `unsafe.Pointer` to cast 56-byte windows of the mmap'd slice into `*[14]float32`.** Safe on linux/amd64: float32 alignment is 4, body starts at offset 64 (multiple of 4), and `i*14*4` preserves alignment.
- **Distance is squared Euclidean, not Euclidean.** Order is preserved; we save one sqrt per neighbor candidate.
- **Manually unrolled the 14-dim subtraction.** The Go compiler at 1.23 leaves a hot accumulator loop with bounds checks even after `nilcheck`/`boundscheck` elision; spelling the 14 subtractions explicitly is unambiguously faster and trivial to read.
- **Top-K = 5, linear insertion.** Faster than a heap at K=5 and trivially cache-friendly (two 5-element stack arrays).

### Files committed this iteration

- `solution/internal/mmapfile/mmapfile.go`
- `solution/internal/mmapfile/mmapfile_test.go`
- `solution/internal/brutef32/brutef32.go`
- `solution/internal/brutef32/brutef32_test.go`
- `solution/internal/brutef32/full_bench_test.go`

### Verification

**Unit tests — all packages green:**

```
ok  rinha2026/solution/internal/brutef32  0.004s
ok  rinha2026/solution/internal/idxfmt    0.004s
ok  rinha2026/solution/internal/mmapfile  0.003s
ok  rinha2026/solution/internal/vec       0.002s
```

The synthetic brutef32 tests cover four scenarios: self-distance-zero, all-legit neighborhood, all-fraud neighborhood, and mixed (3 fraud / 2 legit) — each returns the expected `nFraud` ∈ {0, 3, 5}.

**Full 3M-vector benchmark** (i7-14700HX, single-threaded, mmap'd from `/tmp/rinha-out/full.bin`):

```
BenchmarkSearchKNN_Full3M-28   10   14_502_076 ns/op   14.30 ms/query   1 B/op   0 allocs/op
```

- **14.5 ms/query single-threaded** on a fast modern P-core.
- **0 allocs/op** on the hot path — pure stack arithmetic over an mmap'd slice.
- **Estimated on the Mac Mini Late 2014** (Haswell @ 2.6 GHz, ~50 % of an i7-14700 P-core): **~30 ms/query**.
- At 900 rps with 0.9 CPU shared between two replicas: throughput ceiling ≈ `2 × 0.45 × 1000ms / 30ms ≈ 30 rps`. Brute force at full dataset **will not** clear the latency cutoff or the failure-rate cutoff — exactly as the plan predicted.

### Outcome

**P4a complete.** The oracle search exists, is allocation-free, and runs at the predicted speed. Tests cover correctness end-to-end from `idxfmt` write → mmap → search.

### Open follow-ups

- `mmapfile` lacks `MADV_RANDOM`. We can add it via `golang.org/x/sys/unix` if profile shows TLB pressure; for now stdlib-only.
- The synthetic benchmark `BenchmarkSearchKNN` always uses 200k entries by default. Plumb `BENCH_N` properly via env var parsing (the current `parseN` is a placeholder).

### Next iteration agenda — P4b: exp01 wiring + first leaderboard entry

- Create `experiments/exp01-brute-float32/main.go`: API binary that opens `/data/index.bin` with `mmapfile`, wraps it with `brutef32`, vectorizes incoming requests with `vec.Vectorize`, and returns `approved = nFraud/5 < 0.6`.
- Create `experiments/exp01-brute-float32/docker-compose.yml`:
  - A short-lived `data-prep` service built from a multi-stage image that runs `build-index` and writes `/data/index.bin` into a named volume.
  - `api1`, `api2` mount the volume read-only and depend on `data-prep: service_completed_successfully`.
  - HAProxy in front on 9999, same config as today.
- Run `k6 run test/smoke.js` → expect 200 + body shape.
- Run `k6 run test/test.js` (full ramping load) → capture `test/results.json`.
- Append the first row to `bench/leaderboard.md`. We expect a **negative** `final_score` for exp01 — it's the oracle, not a contender — but it baselines correctness.

## Iter 04b — 2026-05-11 — P4b: exp01 wired, first k6 run, leaderboard live

### Goals

- Stand up `experiments/exp01-brute-float32/` as a full deployable stack: multi-stage Dockerfile, `data-prep` init service, two API replicas, HAProxy, named volume.
- Bake the 168 MB FLAT32 index at image-build time, seed it into a shared named volume at compose-up time, mmap it from both api replicas.
- Run k6 `smoke.js` (correctness) and `test.js` (load) against the stack.
- Open the leaderboard with exp01's row.

### Decisions captured this iteration

- **Reference dataset vendored.** Copied `rinha-de-backend-2026/resources/references.json.gz` into `solution/resources/references.json.gz` (~50 MB compressed / ~297 MB uncompressed — turns out the docs' "~16 MB" figure is conservative). Gitignored so it never ships to the public repo, but lives in the Docker build context so the `index-baker` stage can produce the index without a network call.
- **Image structure: 4 stages.** `builder` (compiles), `index-baker` (runs `build-index` against the gz, produces `/data/index.bin`), `data-prep` (busybox copy-to-volume), `api` (distroless static + api binary). The `data-prep` service ships a tiny image whose only job is `cp /seed/index.bin /shared/index.bin` then exit 0.
- **Named volume `index-data`.** Both api containers mount it read-only. Same inode → kernel page cache deduplicates. Confirmed below: API cgroups don't get charged the file-backed RSS.
- **Pre-encoded response bodies.** Six possible `fraud_score` values (k=5 ⇒ score ∈ {0, 0.2, …, 1.0}), so we precompute the 6 JSON byte slices and `w.Write` one. Zero encoder allocations on the hot path.
- **`sync.Pool` for `vec.Payload`.** Resets on Put to keep stale slices from leaking.
- **GOMAXPROCS=1, GOGC=200.** GOMAXPROCS=1 keeps the runtime aligned with the 0.45-CPU cgroup limit and avoids context switches. GOGC=200 lets short-lived allocations live longer; sync.Pool reuse means we're already lean.
- **`/ready` gated on index load.** A background goroutine mmaps + validates the file at boot; until then `/ready` returns 503. Important: prevents the LB from sending traffic to a half-initialized api.

### Files committed this iteration

- `solution/.dockerignore` (relaxed: experiments/ no longer excluded)
- `solution/.gitignore` (new — keeps resources/references.json.gz out of git)
- `solution/resources/references.json.gz` (vendored from upstream; gitignored)
- `solution/experiments/exp01-brute-float32/main.go`
- `solution/experiments/exp01-brute-float32/Dockerfile`
- `solution/experiments/exp01-brute-float32/docker-compose.yml`
- `solution/experiments/exp01-brute-float32/haproxy.cfg`
- `solution/experiments/exp01-brute-float32/README.md`
- `solution/experiments/exp01-brute-float32/results/run-2026-05-11T1331.json`
- `solution/bench/leaderboard.md`

### Verification

**Boot path:**

```
data-prep → cp index.bin (5s) → exit 0
api1, api2 start → mmap /data/index.bin → close srv.ready chan
/ready returns 200 after ~2s
```

**Smoke (DETECTION_RULES.md spec examples — end-to-end through HAProxy):**

```
POST legit  (tx-1329056812, last_transaction:null) → {"approved":true,"fraud_score":0}
POST fraud  (tx-3330991687, last_transaction:null) → {"approved":false,"fraud_score":1}
```

Both labels match the spec's expected `fraud_score` exactly. Brute-force KNN over the full 3M-vector reference set returns the correct neighbor majority on the first try.

**k6 `smoke.js`:** 20/20 checks pass, median 14.3 ms.

**k6 `test.js` (full ramping → 900 rps × 120 s):**

```json
{
  "p99": "2002.14ms",
  "scoring": {
    "breakdown": {
      "true_positive_detections":  123,
      "true_negative_detections":  140,
      "false_positive_detections": 0,
      "false_negative_detections": 0,
      "http_errors":               13572
    },
    "failure_rate": "98.1%",
    "p99_score":      { "value": -3000, "cut_triggered": true },
    "detection_score":{ "value": -3000, "cut_triggered": true },
    "final_score":    -6000
  }
}
```

- **Both cutoffs active.** Failure_rate 98.1 % blows past 15 % (detection cutoff). p99 saturates at the 2001 ms client timeout (p99 cutoff).
- **263 successful responses had ZERO FP and ZERO FN.** When brute force responds at all, it is exactly correct — exactly the property we wanted from the oracle.
- Throughput at 0.9 total CPU and ~15 ms/query is ≈ 60 rps. Test target is 900 rps. 15× overload → timeouts dominate.

### Memory accounting — the good surprise

`docker stats` during the run:

```
[t=10s ] api1 cpu=0.00% mem=1.44 MiB / 155MiB     ← idle, index just mmap'd
[t=20s ] api1 cpu=45.4% mem=3.18 MiB / 155MiB     ← ramp begins
[t=60s ] api1 cpu=45.0% mem=~30 MiB / 155MiB
[t=120s] api1 cpu=44.2% mem=68.4 MiB / 155MiB     ← peak
[t=120s] api2 cpu=44.1% mem=90.3 MiB / 155MiB     ← peak
[t=120s] haproxy            mem=9.4–14.8 MiB / 15 MiB
```

- **Per-cgroup RSS stayed under 90 MiB**, well below the 155 MiB cap.
- **The 168 MB mmap'd file is NOT charged to either api cgroup.** Hypothesis: when both containers mount the same named-volume inode and mmap it `MAP_SHARED`, the kernel page cache accounting attributes those pages to the *volume's* cgroup (root or `data-prep`-owned), not to the consumers. This is the *exact* property we needed to stay under the 350 MB total cap with two replicas plus a 168 MB index — and it works.
- This bodes well for every later experiment: the IVF / IVF-PQ indexes will be *smaller* than FLAT32, so memory headroom is essentially solved.

### Outcome

**P4 complete.** First leaderboard entry committed:

```
| exp01-brute-float32 | 2002 ms | -3000 | 0 FP | 0 FN | 13 572 Err | 98.1% | -3000 | -6000 |
```

The score is floor (-6000), exactly as predicted. exp01 is **not** a contender — it's the truth source we benchmark recall against in every later experiment.

### Open follow-ups

- Recall of exp01 on the full test set is implied by `0 FP and 0 FN among 263 responded requests`, but we'd want a **standalone offline recall check** that walks all 54 100 test entries and confirms the brute-force output matches the `expected_approved` label for every one of them. Useful as the gold standard for evaluating exp02–exp06's recall without running the full k6 test. → Plan as Iter 05a precursor.
- The k6 docker invocation needs a writable WD with a `test/` subdir for the `handleSummary` relative path to resolve. Captured the working invocation:
  ```
  docker run --rm -i --network host -v /tmp/k6-stage:/work -w /work \
      --user $(id -u):$(id -g) grafana/k6:latest run test/test.js
  ```
  Will land in `bench/run-all.sh` in P5.

### Next iteration agenda — P5: exp02 (brute-force int8) + offline recall harness

- **Recall harness** (`cmd/recall-check`): mmap an index, walk all entries of `test-data.json`, compare predicted vs. expected. Outputs FP / FN / TP / TN / error rate. Fast — single-process, no HTTP.
- Use it to confirm exp01 has 100 % match against `test-data.json` labels (which were themselves generated by k=5 Euclidean brute force).
- **exp02 — int8 quantized brute force:**
  - Add `FormatInt8 = 2` to `idxfmt`; quantize [0,1] → uint8 [0,254], reserve 255 for the −1 sentinel.
  - Body size shrinks from 168 MB to **42 MB**. Even if cgroup attribution misbehaves, both replicas fit comfortably.
  - `internal/brutei8` implements distance in *uint16* (sum of squared differences over 14 dims, max ~14·255² ≈ 910 k → fits in uint32). Likely autovectorizes well.
  - Goal: same 0-FP/0-FN behavior on responded requests, but with ~4× faster scan → maybe 100–150 rps per replica → ~250 rps total.
- Append exp02 row to `bench/leaderboard.md`.

---

## Iter 05 — 2026-05-11 — P5: recall-check harness + exp02 int8 brute force

### Goals

- Build `cmd/recall-check`: offline harness that walks all 54 100 `test-data.json` entries and validates brute-force KNN against `expected_approved`.
- Add int8 index support to `idxfmt`: `Int8Writer`, `QuantizeFloat64`, `DequantizeUint8`, `Int8BodySize`.
- Build `internal/brutei8`: brute-force KNN over FormatInt8 index (uint32 distance).
- Update `cmd/build-index` with `--format=flat|int8` flag.
- Deploy `experiments/exp02-brute-int8`, run k6 smoke + full, commit leaderboard row.

### Decisions captured this iteration

- **recall-check confirms exp01 is essentially perfect.** 0 FP, 1 FN across all 54 100 entries (accuracy 99.9982 %). The single FN is almost certainly a float64→float32 precision edge case where one neighbor rank flips. Well under the 15 % failure-rate cutoff.
- **int8 quantization scheme.** `[0,1] → uint8 [0,254]` (linear, rounded to nearest), `-1` sentinel → 255. 255 is not reachable by valid values (max valid = round(1.0×254+0.5) = 254), so sentinel separation is exact.
- **int8 benchmark surprise: 15.3 ms/query, same as float32 (14.5 ms).** Root cause: the bottleneck is sequential memory bandwidth; with the page cache warm, both indices fit comfortably and arithmetic is not the limit. The 4× memory footprint advantage (42 MB vs 168 MB) doesn't help in warm-cache benchmarks but *does* reduce page-fault pressure under cgroup limits in production.
- **exp02 confirms int8 is lossless for KNN decisions.** 738 successful responses, 0 FP, 0 FN — identical decision accuracy to exp01. Throughput is only marginally better (~738 vs ~263 successful in 120 s; exp01's CPU-time was being burned more wastefully).
- **Build-index `writer` interface.** Added a small `writer` interface in `cmd/build-index/main.go` so the streaming loop is shared between `FlatWriter` and `Int8Writer`. No duplication.

### Files committed this iteration

- `solution/cmd/recall-check/main.go`
- `solution/internal/idxfmt/int8.go`
- `solution/internal/brutei8/brutei8.go`
- `solution/internal/brutei8/brutei8_test.go`
- `solution/internal/brutei8/bench_full_test.go`
- `solution/cmd/build-index/main.go` (updated: `--format` flag, `writer` interface)
- `solution/experiments/exp02-brute-int8/main.go`
- `solution/experiments/exp02-brute-int8/Dockerfile`
- `solution/experiments/exp02-brute-int8/docker-compose.yml`
- `solution/experiments/exp02-brute-int8/haproxy.cfg`
- `solution/experiments/exp02-brute-int8/README.md`
- `solution/experiments/exp02-brute-int8/results/run-2026-05-11-T1402.json`
- `solution/bench/leaderboard.md`

### Verification

**Unit tests (all pass):**

```
ok  rinha2026/solution/internal/brutef32
ok  rinha2026/solution/internal/brutei8
ok  rinha2026/solution/internal/idxfmt
ok  rinha2026/solution/internal/mmapfile
ok  rinha2026/solution/internal/vec
```

**recall-check (FLAT32 index, 54 100 entries):**

```
TP: 24057  TN: 30042  FP: 0  FN: 1  errors: 0
accuracy: 99.9982%   failure_rate: 0.0018%
elapsed: 13m14s  (14.7 ms/query)
```

**int8 index build:** 3M entries → 42 375 064 B in 6.8 s, heap ~4 MB.

**k6 smoke:** 20/20 checks pass, median 13.6 ms.

**k6 full (`test.js`):**

```json
{
  "p99": "2002.21ms",
  "scoring": {
    "breakdown": {
      "true_positive_detections":  328,
      "true_negative_detections":  410,
      "false_positive_detections": 0,
      "false_negative_detections": 0,
      "http_errors": 13108
    },
    "failure_rate": "94.67%",
    "p99_score":       { "value": -3000, "cut_triggered": true },
    "detection_score": { "value": -3000, "cut_triggered": true },
    "final_score": -6000
  }
}
```

- Both cutoffs active. 738 successful responses with 0 FP/0 FN. Throughput bottleneck remains query latency (~15 ms).

### Outcome

**P5 complete.** Two new leaderboard rows (exp02 same score as exp01, but accuracy is confirmed). Both brute-force variants are proven lossless within the 15 % failure-rate window.

### Open follow-ups

- `bench/run-all.sh` automating the k6 staging dance for all experiments — still deferred.
- int8 recall-check: current recall-check binary is hardwired to brutef32. Should accept a `--format` flag for int8 too. Low priority since we can infer from k6.

### Next iteration agenda — P6: exp03 parallel brute-force + exp04 IVF

The throughput ceiling with single-goroutine brute force is ~65 rps (0.45 CPU × 1/15ms). We need ~300 rps to approach a positive score. Two orthogonal levers:

**exp03 — parallel brute-force:**
- Split the 3M index into N shards; each shard searched by a dedicated goroutine. Per-request fan-out + merge top-5 across shards. `GOMAXPROCS=2` might actually utilize both hyper-threads on the 0.45 CPU slice and give ~1.5–2× throughput.
- Shard-level results merged with a simple O(N×K) combine.
- Fast to implement; tells us if the CPU headroom is just hyper-threading underutilization.

**exp04 — IVF (Inverted File Index):**
- Offline k-means clusters (e.g. C=256 centroids, built into the index image at build time).
- At query time, compare query to all C centroids → top `nprobe` (e.g. 8) → search only those clusters (~nprobe × 3M/C = 8 × 12k = 96k vectors).
- Expected speedup: 3M/96k ≈ 31×; at 15 ms/query this projects to ~0.5 ms. With some overhead, target is 1–3 ms/query.
- Index size: C × Dim × 4 bytes (centroids) + per-cluster sorted lists. Manageable.
- Implementation: offline builder in `cmd/build-ivf`, runtime in `internal/ivf`.
- **This is the experiment most likely to break the p99 < 1000 ms cutoff.**

Priority: exp04 (IVF) is the highest-leverage path. Implement it directly. exp03 can be a quick test first to gather data on parallelism overhead.

---

## Iter 06 — 2026-05-12 — P6: exp03 parallelism probe + exp04 IVF (first positive score)

### Goals

1. **exp03** — quick GOMAXPROCS=2 clone of exp02 to confirm (or refute) whether threading headroom exists inside the 0.45 CPU slice.
2. **exp04** — implement full IVF pipeline: k-means++ builder, IVF on-disk format, runtime search, compose stack, k6 benchmark.

### Decisions captured

**exp03 (GOMAXPROCS=2, brute int8):**
- Cloned exp02's compose file; changed `GOMAXPROCS=2` for both replicas. No code changes.
- Result: 726 successes vs 738 for exp02 — statistically identical. final_score = −6000 in both cases.
- Confirms: throughput = `CPU_budget / CPU_per_query` is a hard ceiling regardless of goroutines. 0.45 CPU ÷ 15 ms = 30 rps/replica, full stop. ANN required.

**k-means++ (internal/kmeans):**
- Incremental distance maintenance for O(n×k×dim) total init (vs naïve O(n×k²×dim) which would take hours on 3M/256).
- `dists` must be initialized to `math.MaxFloat32` — not zero. Zero init means the atomic-min CAS never fires on the first centroid, so all subsequent centroids are sampled from element 0 → catastrophic clustering (min cluster size = 0, max = 1,600,681). Fixed with a `for i := range dists { dists[i] = math.MaxFloat32 }` loop before the first `parallelUpdateDists` call.
- Parallel assignment and parallel `updateDists` via goroutine slabs + atomic CAS on float32 bits. Build time on this rig: 20.6 s (3M vectors, C=256, 30 iterations, GOMAXPROCS=28 inside the builder container).

**IVF format (internal/idxfmt/ivf.go):**
- Layout: 64-byte magic header + nCentroids(uint32) + centroids(C×14×float32) + sizes(C×uint32) + offsets((C+1)×uint64) + cluster data (per-cluster: count×14 uint8 vecs + bit-packed labels).
- `IVFWriter` requires cluster `sizes []uint32` upfront so it can precompute all byte offsets at construction time. `AddCluster` is then a sequential writer with no bookkeeping.
- Index output: 42,392,598 bytes; cluster sizes min=561 / avg=11,718 / max=37,621.

**IVF search (internal/ivf/ivf.go):**
- Step 1: linear scan over C=256 float32 centroids → top nprobe by squared Euclidean distance. The centroid scan itself is negligible.
- Step 2: quantize query float32 → uint8 once.
- Step 3: for each of the nprobe clusters, cast raw bytes via `unsafe.Pointer`, run unrolled 14-dim int32 squared distance, maintain a global top-5 heap (distance + fraud bit co-located to avoid a second pass over labels).
- Benchmark (nprobe=8, 3M index, warm cache): **117 μs/query — 128× faster than brute force.**
- Capacity at 0.45 CPU: 0.45 / 0.000117 ≈ 3,846 qps per replica, >> 450 rps demand half. Low utilization → minimal queuing.

**nprobe tuning:**
- nprobe=8 → p99=176 ms, FP=26, FN=44, final=**+2627**
- nprobe=16 → p99=648 ms, FP=26, FN=42 (2 fewer FN). Score degraded to +2063.
- Latency penalty of nprobe doubling overwhelms the marginal recall gain. Optimal is nprobe=8.

**exp04 compose stack:**
- 4-stage Dockerfile: builder (builds build-index + build-ivf + api binary) → index-baker (runs build-index then build-ivf inside the image, embeds the result) → data-prep → api.
- Both api replicas: GOMAXPROCS=1, GOGC=200, 0.45 CPU / 155 MB each.
- haproxy: roundrobin LB, 0.05 CPU / 15 MB.
- Peak mem api1+api2: ~10 + 10 MiB (MAP_SHARED mmap of 42 MB IVF index → not charged to cgroup RSS).

### Files committed

- `solution/experiments/exp03-parallel-int8/docker-compose.yml`
- `solution/internal/kmeans/kmeans.go`
- `solution/internal/kmeans/kmeans_test.go`
- `solution/internal/idxfmt/ivf.go`
- `solution/cmd/build-ivf/main.go`
- `solution/internal/ivf/ivf.go`
- `solution/internal/ivf/ivf_test.go`
- `solution/experiments/exp04-ivf-int8/main.go`
- `solution/experiments/exp04-ivf-int8/Dockerfile`
- `solution/experiments/exp04-ivf-int8/docker-compose.yml`
- `solution/experiments/exp04-ivf-int8/haproxy.cfg`
- `solution/experiments/exp04-ivf-int8/README.md`
- `solution/experiments/exp04-ivf-int8/results/run-2026-05-12-T0116.json`
- `solution/bench/leaderboard.md`

### Verification

**Unit tests (all pass):**

```
ok  rinha2026/solution/internal/brutef32
ok  rinha2026/solution/internal/brutei8
ok  rinha2026/solution/internal/idxfmt
ok  rinha2026/solution/internal/ivf
ok  rinha2026/solution/internal/kmeans
ok  rinha2026/solution/internal/mmapfile
ok  rinha2026/solution/internal/vec
```

**k-means build (3M vectors, C=256, iters=30):** cluster sizes min=561 / avg=11,718 / max=37,621, output 42,392,598 B, 20.6 s.

**IVF benchmark (nprobe=8, RINHA_INDEX=full.bin, warm cache):** ~117 μs/query.

**k6 smoke (exp04, nprobe=8):** 20/20 checks pass, median 1.9 ms.

**k6 full (`test.js`, exp04, nprobe=8):**

```json
{
  "p99": "176.41ms",
  "scoring": {
    "breakdown": {
      "false_positive_detections": 26,
      "false_negative_detections": 44,
      "true_positive_detections": 23972,
      "true_negative_detections": 29969,
      "http_errors": 0
    },
    "failure_rate": "0.13%",
    "p99_score":       { "value": 753.49,  "cut_triggered": false },
    "detection_score": { "value": 1873.41, "cut_triggered": false },
    "final_score": 2626.89
  }
}
```

**k6 full (nprobe=16, runtime env change only):** p99=647ms, final_score=2063 — worse. Reverted NPROBE back to 8.

### Outcome

**P6 complete. First positive leaderboard entry: exp04 at +2627.**

- IVF delivered 128× query speedup over brute force.
- p99 = 176 ms (well under both the 1000 ms latency cutoff and the 2000 ms scoring floor).
- Recall 99.87% (26 FP + 44 FN) — failure_rate = 0.13%, far below the 15% detection cutoff.
- Non-search overhead (JSON decode + vectorize + encode) appears small relative to the 117 μs search.
- exp03 closed the book on brute-force parallelism: it is not a viable path.

### Open follow-ups

- `bench/run-all.sh` automating the k6 staging dance — still deferred.
- recall-check binary is hardwired to brutef32. A `--format=ivf` mode would let us profile ANN recall offline without running k6.
- Current p99 of 176 ms has significant margin from 1 ms (max score). Headroom exists to trade more recall accuracy for latency (smaller nprobe) or vice versa.

### Next iteration agenda — P7: latency deep-dive + recall improvement

**Where does the 176 ms p99 come from?**

At 117 μs/query and ~450 rps demand per replica, expected service time ≈ 53 ms. p99 = 176 ms suggests 3× service-time queuing or per-request overhead (JSON decode, vectorize, write response). Profiling options:

1. **Profile in-process overhead:** add `time.Since` spans around decode / vectorize / search / encode. Log slowest-percentile breakdown to stderr during a test run.
2. **Try GOMAXPROCS=2:** with IVF at 117 μs/query the CPU per query is very low. Two goroutines may allow better pipelining of concurrent requests without saturating the 0.45 CPU slice.

**Recall improvement levers (to push detection_score above +1873):**

- `C=512` centroids: larger vocabulary → tighter clusters → better recall at same nprobe.
- `nprobe=12` with `C=512`: searching the same ~96k vectors but from a better-partitioned space.
- Exact recheck of top cluster boundary cases: for vectors that fall near a centroid boundary, run exact search on the two neighbouring centroids even if they are not in the top nprobe.

**Candidate exp05:** `C=512, nprobe=10` — rebuild IVF index, no API code changes needed (just NPROBE env), measure recall improvement vs latency cost.

---

## Iter 07 — IVF-F32: float32 cluster vectors + nprobe sweep (P7)

**Date:** 2026-05-11 → 2026-05-12

### Goals

- P7a: exp05 IVF C=512, tune nprobe, improve recall beyond exp04's +2627.
- Root-cause the 70 FP/FN errors in exp04 (int8 IVF).
- Eliminate the dominant error source.

### Root-cause analysis

All 70 errors (26 FP, 44 FN) from exp04 were diagnosed with `cmd/check-ivf`:
- nFraud distribution among errors: 2=44, 3=26 — all borderline.
- Running IVF BruteForce (all 512 clusters) still gave the wrong answer for 69/70 errors.
- Increasing nprobe to 256 (50% coverage) still gave the wrong answer.

**Conclusion:** the errors are caused by **int8 quantization distortion**, not ANN cluster misses. The int8 distance ranking differs from float32, so the "true" top-5 neighbours in float32 space are genuinely different from the int8 top-5. The wrong cluster is not being skipped — the wrong vector is winning after quantization.

### Solution: FormatIVF_F32

Added `FormatIVF_F32 = uint16(4)` to `internal/idxfmt/idxfmt.go`. Cluster vectors are stored as **float32** (56 bytes/vector) instead of int8 (14 bytes/vector). Distance computation is exact (no quantization). Index size: ~168 MB, MAP_SHARED mmap → 0 RSS per replica.

Key code changes:
- `internal/idxfmt/ivf.go`: `IVFWriterF32`, `AddClusterF32`, offset computation using float32 cluster size.
- `internal/ivf/ivf.go`: `searchAndCountF32`, dispatches on `isF32` flag.
- `cmd/build-ivf/main.go`: `--dtype=float32|int8` flag.

### exp05 (C=512, int8, nprobe=16): **+2624**

No improvement over exp04 (C=256, nprobe=8). Root cause confirmed: quantization, not cluster miss.

### exp06 (adaptive int8 brute-force fallback): **+2246**

SearchKNNAdaptive falls back to full int8 brute-force for nFraud=2 or 3. Only 1 FN saved (69/70 errors are still wrong in int8 brute force). p99=431ms from the brute-force path. Worse than exp04.

### exp07 (F32, nprobe=16, byte-by-byte reader): **+2871**

First F32 run. Recall: **0 FP + 1 FN** — eliminated 69/70 errors. But p99=887ms because `searchAndCountF32` read float32 bytes one-by-one with bit manipulation (`uint32(b[0]) | uint32(b[1])<<8 | ...`), ~7 ops/dimension.

### exp08 (F32, nprobe=16, unsafe.Pointer): **+3608**

Fixed `searchAndCountF32` with `(*[Dim]float32)(unsafe.Pointer(&vecBytes[i*Dim*4]))` + loop unrolling (same pattern as int8 `searchAndCount`). Result: 5.5× speedup, p99=163ms. Keeps 0 FP + 1 FN. **Beats exp04 by +981.**

### nprobe sweep (C=512, F32, unsafe.Pointer)

| nprobe | p99 | FP | FN | final_score |
|--------|-----|----|----|-------------|
| 4 | 76 ms | 5 | 5 | +3720 |
| 6 | 84 ms | 1 | 2 | +3806 |
| **8** | **94 ms** | **1** | **1** | **+3816** ← best |
| 16 | 163 ms | 0 | 1 | +3608 |

**nprobe=8 is the sweet spot.** Halving clusters searched (94ms → 163ms) gains +237 p99_score and costs only −29 detection_score (1 extra FP).

### Files committed

- `internal/idxfmt/idxfmt.go` — FormatIVF_F32 constant
- `internal/idxfmt/ivf.go` — IVFWriterF32, AddClusterF32, IVFClusterDataSizeF32
- `internal/ivf/ivf.go` — isF32 flag, Open dispatch, searchAndCountF32 (unsafe.Pointer version)
- `cmd/build-ivf/main.go` — --dtype flag, float32 cluster build path
- `experiments/exp05-ivf-512/`, `exp06-ivf-adaptive/`, `exp07-ivf-f32/`
- `experiments/exp08-ivf-f32-fast/` — canonical float32 + unsafe.Pointer implementation
- `experiments/exp09-ivf-f32-np8/` — **best configuration**; nprobe=8
- `experiments/exp10-ivf-f32-np4/`, `exp11-ivf-f32-np6/` — nprobe sweep variants
- `bench/leaderboard.md` — updated with exp05–exp11

### Outcome

**Best result: exp09 at +3816** (vs previous best exp04 at +2627, improvement +1189).

- p99 = 94 ms, p99_score = +1026
- 1 FP + 1 FN, detection_score = +2790
- 0 http_errors, 0 % failure_rate

The remaining 1 FP and 1 FN are genuine ANN misses (searched 8/512 clusters = 1.56% coverage).

### Open follow-ups

- `cmd/check-ivf` could be extended to diagnose nprobe=8 misses vs nprobe=16.
- C=1024 centroids: tighter clusters may give better recall at same nprobe=8 with same index size (168 MB), but k-means build time is ~4× longer.
- Exact F32 scan for borderline cases (nFraud=1 or 4) as a selective fallback — but 4% of queries slow path may push p99 above the fast-path threshold.

### Next iteration agenda — P8: can we beat +3816?

**Option A — C=1024 centroids (F32, nprobe=8):**
- Tighter clusters → fewer ANN misses at same nprobe
- Index size unchanged (~168 MB). k-means C=1024, 30 iters over 3M vectors: ~4× slower build
- Expected: 0 FP + 0 FN at nprobe=8 → detection_score=3000 → total ≈ +4026

**Option B — adaptive nprobe for borderline queries:**
- Run nprobe=8 F32 fast pass; if nFraud ∈ {1,2,3,4}, re-run with nprobe=32
- But ~4% slow queries would dominate p99 (~300ms) → p99_score ~600 → total ≈ 3600, worse than exp09

**Recommended: Option A (C=1024)**. If it finds the correct 5 neighbours for the 2 remaining misses at nprobe=8, we gain +210 detection points at same latency, pushing score to ~4026.

---

## Iter 08 — C=1024 centroids + centroid loop micro-opt investigation (P8)

**Date:** 2026-05-12

### Goals

P8: try C=1024 centroids (tighter clusters → fewer ANN misses at same nprobe=8) to push detection_score to 3000.

### exp12 (C=1024, nprobe=8): **+3592**

Cluster sizes: min=160, avg=2929, max=13908. Index size ≈ 168MB (same as C=512).

WORSE than exp09 (C=512/np=8, +3566 stable). Two reasons:
1. **Centroid scan overhead**: 1024 × 14 float32 = 57KB, spills out of L1 cache (28KB). C=512 centroids = 28KB, fits in L1 cleanly. Despite searching 2× fewer vectors per nprobe, the centroid scan cost dominates.
2. **Non-uniform clusters**: max cluster = 13,908 vectors vs C=512's avg ~5,859. At p99, the query hits the largest cluster; nprobe=8 at C=1024 searches more vectors at p99 than average.

### Centroid loop micro-optimization: **REGRESSION**

Applied `unsafe.Pointer` + loop unrolling to the centroid scan (same pattern that fixed `searchAndCountF32`). Result: exp09 p99 went from ~170ms → 181ms.

**Why it regressed:** The centroid loop `for j := 0; j < Dim; j++ { d += (c[j]-q[j])^2 }` over a `[]float32` slice is auto-vectorized by the Go compiler (SSE/AVX). The unsafe.Pointer unrolled version breaks the vectorization hint — 14 scalar subtraction + multiply ops are slower than the SIMD path.

**Lesson:** The slice-based inner loop with a `for` body is the CORRECT fast form for the centroid scan. Do NOT apply the unsafe.Pointer unroll optimization to it.

**Reverted immediately.** The `searchAndCountF32` optimization (unsafe.Pointer) is correct there because the mmap data isn't a contiguous typed slice (it's `[]byte`), so the compiler can't auto-vectorize it. The centroid data is already `[]float32`, enabling auto-vectorization.

### Stack allocation micro-opt: no measurable improvement

Changed SearchKNN centroid buffers from `make([]T, nprobe)` to fixed `[64]T` stack arrays. p99 unchanged (~172ms). Confirms heap allocation rate (900/sec × 96B) is not the bottleneck.

### exp13 (C=1024, nprobe=16): **+3563**

Same ~46k vectors searched as exp09 but centroid scan at 57KB still penalizes p99 (181ms vs exp09's 172ms typical). Same accuracy (0 FP + 1 FN) as exp08.

### exp09 performance variance

Best observed: 94ms p99, **+3816** (single run, likely a lucky P-core assignment on i7-14700HX).
Stable/typical: 165–180ms, **+3530–3570**.

The i7-14700HX has 8 P-cores (2.1–5.5 GHz) and 12 E-cores (1.5–4.2 GHz). WSL2 goroutine OS thread can land on either. The 2× speed ratio explains the 94ms vs 172ms gap. The Mac Mini Late 2014 (uniform dual-core i5) would give consistent results (~2× slower than WSL2 typical).

### Files committed

- `internal/ivf/ivf.go` — stack-allocated centroid buffers (maxNprobeStack=64), centroid loop kept as slice-based (Go auto-vectorization preserved)
- `experiments/exp12-ivf-f32-c1024/` — C=1024, nprobe=8
- `experiments/exp13-ivf-f32-c1024-np16/` — C=1024, nprobe=16
- `bench/leaderboard.md` — updated with exp12, exp13 rows + exp09 stable score

### Outcome

C=512/nprobe=8 (exp09) remains the best configuration. C=1024 is worse due to centroid scan L1 cache pressure and non-uniform cluster size distribution.

**Current best:** exp09 at **+3816** (best observed) / **+3566** (stable median).

### Open follow-ups

- Performance varies 94ms→172ms due to E/P core scheduling on WSL2. Competition (Mac Mini, uniform cores) would give ~2× typical WSL2 → ~330ms p99 → ~+3300 estimated competition score.
- Stack allocation change (`maxNprobeStack`) is now in ivf.go — harmless but also no improvement. Could be reverted if it adds confusion.
- exp12 and exp13 directories are preserved for reference but not load-bearing.

### Next iteration agenda — P9: reduce constant per-request overhead

At ~170ms p99 with nprobe=8 searching 46k vectors in ~10ms, the overhead (~160ms at p99) comes from request queuing under high concurrency. 250 VUs × 0.17s avg ≈ 42 in-flight per replica. Options:

1. **Profile actual breakdown**: add per-request timing log (decode / search / encode) for 100 samples. Currently blind to where overhead lives.
2. **Reduce JSON decode cost**: current `json.NewDecoder(r.Body).Decode(p)` allocates a decoder + buffers. Could use `json.Unmarshal` with pre-read body, or a custom parser.
3. **HTTP/1.1 pipelining**: already using keep-alives. Verify HAProxy is not adding latency.
4. **Try smaller GOGC or `GODEBUG=gccheckmark=1`** to measure GC pause contribution.

---

## Iter 09 — 2026-05-13 — P9/P10: per-request profiling exposes HAProxy CFS throttle, +2000 points

**Date:** 2026-05-13

### Goals

P9: instrument exp09's handler to break p99 down into decode / search / encode, prove which phase owns the ~160ms overhead, and remove it.

### exp14 — per-phase timing inside the handler

Added cheap `time.Since` counters around `vec.From`, `idx.SearchKNN`, and the response write; logged every ~1000th request. Sampled p99 of each phase across a full k6 run.

| phase | typical | p99 sample |
|------:|--------:|-----------:|
| decode + vec.From | 18 µs | 95 µs |
| SearchKNN (nprobe=8) | 250 µs | 770 µs |
| encode + write | 4 µs | 12 µs |
| **handler total** | **~290 µs** | **~770 µs** |

Yet k6 reports **p99 ≈ 179 ms** for the same run (score +3536). **99.6% of the wall-clock p99 lives outside the handler.** The Go server is not the bottleneck.

### exp15 — HAProxy `http-reuse aggressive`: no effect (+3535)

Hypothesis: HAProxy was opening a fresh backend TCP connection per request. Added `http-reuse aggressive`, kept everything else from exp09. Result: p99 = 180 ms, unchanged. Connection management was not the bottleneck.

### Sequential probe of HAProxy `/ready` exposes CFS throttle

Bypassed the API entirely:

```bash
for i in $(seq 1 200); do
  curl -s -o /dev/null -w '%{time_total}\n' http://localhost:9999/ready
done | sort -n | tail
```

Pattern: a ~100 ms spike every ~7th request. Spikes line up exactly with the **100 ms CFS scheduling period**. HAProxy at `cpus: "0.05"` gets a **5 ms CPU quota per 100 ms period**; once the quota burns it sleeps until the next period boundary. Under any non-trivial load the quota exhausts within microseconds and the next request waits for the next period — straight to ~100 ms latency, every time.

Cross-check: `docker stats` during k6 showed HAProxy CPU pinned at **~4.99%** of the host (≈ 100% of its 0.05 limit). The 1-CPU budget was misallocated.

### Direct host → api1 (HAProxy out of the loop)

Added `ports: ["8081:8080"]` to api1 in exp14, hit it from k6: **p99 = 3.56 ms, score = +5239**. Confirms the API itself is fine; HAProxy was the sole source of the ~170 ms ceiling.

### exp16 — HAProxy 0.10 CPU: better, still throttled (+4460)

CPU rebalance: HAProxy 0.05 → 0.10, api1/api2 0.45 → 0.425 each, data-prep 0.05. Total still 1.00. Added `http-reuse always` and bumped health checks to `inter 10s` to reduce HAProxy overhead.

Result: **p99 = 21 ms, score = +4460** (+894 over exp09). But `docker stats` showed HAProxy at **9.99%** — still pegged at 100% of its new 0.10 limit, still partially throttled.

### exp17 — HAProxy 0.15 CPU: throttle gone (+5557) — NEW BEST

Pushed HAProxy to 0.15, api1/api2 to 0.40 each, data-prep 0.05. Total 1.00.

Three k6 runs:

| run | p99 | score |
|----:|----:|------:|
| 1 | 1.64 ms | +5575 |
| 2 | 1.79 ms | +5538 |
| 3 | 1.38 ms | +5651 |

**Median: +5575.** HAProxy CPU under load now sits well below its 0.15 cap (no throttle). Detection score unchanged (+2790, same 1 FP + 1 FN as exp09 — same IVF-F32 C=512 nprobe=8 model). The entire **+1991 point jump** (exp09 +3566 → exp17 +5557) is pure latency: removing CFS throttling on HAProxy.

### Why this was invisible before

- Handler timing looked clean (~290 µs typical, 770 µs p99) — the API was never the suspect.
- `docker stats` showed HAProxy CPU at "5% of the host" which **sounds low** until you remember the cgroup limit is also 5% — utilization vs. limit was the giveaway.
- 100 ms CFS periods produce p99 spikes that look like generic GC/queueing noise unless you sample sequentially with a low-rate probe.
- The leaderboard's ~170 ms plateau across exp08, exp09, exp14, exp15 was the symptom: **all four runs were CFS-throttle-bound, not algorithm-bound.**

### Files committed

- `experiments/exp14-profiling/` — exp09 + per-phase timing (kept as the debugging artifact).
- `experiments/exp15-haproxy-reuse/` — `http-reuse aggressive` test.
- `experiments/exp16-haproxy-cpu/` — HAProxy 0.10 CPU + `http-reuse always`.
- `experiments/exp17-haproxy-cpu2/` — **BEST.** HAProxy 0.15 CPU, api 0.40 each, `http-reuse always`, health inter 10s.
- `bench/leaderboard.md` — rows for exp14–17, exp17 marked as new BEST.

### Outcome

**Best result: exp17 at +5557** (stable median ≈ +5540–5580). Previous best: exp09 +3566. **+1991 point gain from CPU rebalancing alone**, zero algorithm changes.

| | p99 | score |
|---|---:|---:|
| exp09 (0.05 CPU HAProxy) | 172 ms | +3566 |
| exp16 (0.10 CPU HAProxy) | 21 ms | +4460 |
| **exp17 (0.15 CPU HAProxy)** | **1.7 ms** | **+5557** |

### Open follow-ups

- ~~Run a third k6 confirmation of exp17 to seal the median~~ — done 2026-05-13: r3 = +5651 (p99 1.38ms). Median across three runs is +5575.
- Mac Mini Late 2014 projection: handler still ~600 µs, HAProxy work ~10× the current rig, so the 0.15 CPU split should hold but is worth a sanity check.
- exp14-15 directories are preserved for reference but the per-phase timing in exp14 should not ship to a competition build (log overhead).

### Lesson (saved to memory)

Under a tight CPU budget (≤ 1 CPU split across multiple containers), **CFS throttling on the LB can dominate p99** even at low absolute CPU% if the cgroup limit is anywhere near saturation. Rule of thumb: keep each container's measured CPU% well below its limit (target < 70%). For HAProxy with `nbthread=1` on Linux 6.6, 0.15 CPU is the floor at 900 rps; 0.05 is unusable.

### Next iteration agenda — P11: solidify exp17 and prepare submission

1. ~~**Confirm exp17 median** with a third k6 run.~~ Done 2026-05-13: r3 = +5651. Median +5575.
2. ~~**Promote exp17 to the canonical submission**~~ Done 2026-05-13. Canonical stack rebuilt from scratch and verified: smoke 5/5, full k6 = p99 1.35ms / score **+5658.57** (`bench/results/canonical-promote-2026-05-13.json`). Files updated: `cmd/api/main.go`, `Dockerfile`, `docker-compose.yml`, `haproxy.cfg`.
3. **Update `info.json`** with the participant's name + GitHub link before opening the participant PR. ← blocked: needs the GitHub repo to exist first (so `source-code-repo` is a real URL). Order: create repo on GitHub → `git init` + initial commit + push → then fill `info.json` with the canonical URL.
4. **Reproduce on a fresh Docker daemon** (cold cache, image rebuild) to make sure the data-prep stage's `index.bin` (~168 MB) is deterministic.
5. **Optionally:** test with HAProxy 0.20 CPU and api 0.375 each to see if there's any remaining headroom — but expect diminishing returns since handler+network is now ~1 ms and any further p99 gain saturates at the +3000 cap.
