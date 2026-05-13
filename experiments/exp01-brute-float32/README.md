# exp01 — brute-force float32

The correctness oracle of the experiment matrix. Brute-force k=5 Euclidean KNN over the full 3M-entry FLAT32 index. Slow on purpose — every other experiment's recall is validated by comparing against this one.

## Stack

- `haproxy:2.9-alpine` round-robin on port 9999.
- 2 × Go API binaries (distroless), each mmaps `/data/index.bin` shared via a named volume.
- `data-prep` init service builds the index from `resources/references.json.gz` at image-build time, copies it into the volume at compose-up time, exits.

## Hypothesis

| Metric | Predicted (Mac Mini Late 2014) |
|---|---|
| p99    | 30–200 ms (single-threaded brute force ~30 ms; under-budget pressure makes this worse) |
| score_p99 | 700 to 1500, possibly cutoff −3000 |
| Recall vs. expected | ≥ 99 % (same KNN that generated `test-data.json` labels) |
| FP + FN | < 200 out of 54 100 |
| score_det | ~+2000 |
| final_score | low positive or low negative; not a contender |

## Memory budget

| Service | CPU | Memory |
|---|---|---|
| haproxy | 0.05 | 15 MB |
| data-prep | 0.05 | 25 MB |
| api1 | 0.45 | 155 MB |
| api2 | 0.45 | 155 MB |
| **total** | **1.00** | **350 MB** |

⚠ Tight: the index is 168 MB and a brute-force scan touches every page. Under cgroup v2, file-backed pages are charged to the cgroup that faults them. With 155 MB per api, the kernel will reclaim file pages aggressively → repeated page faults → terrible p99. This is expected behavior for exp01.

## Run

```bash
cd experiments/exp01-brute-float32
docker compose up -d --build
# wait for /ready
curl -i http://localhost:9999/ready
# k6 smoke
cd ../../../rinha-de-backend-2026 && k6 run test/smoke.js
# k6 full load
k6 run test/test.js
cat test/results.json | jq
```

## Results

See `bench/leaderboard.md` and `experiments/exp01-brute-float32/results/`.
