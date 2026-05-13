# exp04 — IVF int8

Approximate k=5 KNN using an Inverted File Index (IVF) with int8-quantized cluster vectors.

## How it works

**Offline (build time):**
1. Build FLAT32 index from `references.json.gz`.
2. Run parallel k-means++ on the 3M float32 vectors → C=256 centroids.
3. Assign each vector to its nearest centroid.
4. Write FormatIVF index: centroids + per-cluster int8-quantized vectors.

**Online (query time):**
1. Compute float32 squared distance to all 256 centroids (~3μs).
2. Pick the nearest nprobe=8 centroids.
3. Search those 8 clusters' int8 vectors for top-5 (8 × ~11,700 = ~93,600 vectors).
4. Return `nFraud / 5`.

Expected speedup: 3M / 93,600 ≈ **32× fewer distance computations** than brute force.

## Hypothesis

| Metric | Predicted |
|---|---|
| Query time | ~0.5–1.0 ms (32× less work than 15 ms brute force) |
| p99 | < 200 ms at 900 rps (server capacity ~300–600 rps/replica) |
| score_p99 | +700 to +2400 |
| FP + FN rate | < 5 % (IVF recall ~95–98 % at nprobe=8) |
| score_det | ~+2000 |
| **final_score** | **+2700 to +4400 (first positive score)** |

## Memory budget

| Service | CPU | Memory |
|---|---|---|
| haproxy | 0.05 | 15 MB |
| data-prep | 0.05 | 25 MB |
| api1 | 0.45 | 155 MB |
| api2 | 0.45 | 155 MB |
| **total** | **1.00** | **350 MB** |

IVF index: ~42 MB (same size as int8 flat). Comfortably fits in the 155 MB budget per replica.

## Tuning knobs

- `NPROBE` env var controls nprobe at runtime (default 8). Higher = better recall, slower query.
- `--centroids=N` in `build-ivf` controls cluster count (default 256).

## Run

```bash
cd experiments/exp04-ivf-int8
docker compose up -d --build
until [ "$(curl -s -o /dev/null -w '%{http_code}' http://localhost:9999/ready)" = "200" ]; do sleep 1; done

rm -rf /tmp/k6-stage && mkdir -p /tmp/k6-stage/test
cp /home/italo/rinha-2026/rinha-de-backend-2026/test/*.{js,json} /tmp/k6-stage/test/
chmod -R 777 /tmp/k6-stage

# smoke
docker run --rm -i --network host \
  -v /tmp/k6-stage:/work -w /work --user "$(id -u):$(id -g)" \
  grafana/k6:latest run test/smoke.js

# full
docker run --rm -i --network host \
  -v /tmp/k6-stage:/work -w /work --user "$(id -u):$(id -g)" \
  grafana/k6:latest run test/test.js

docker compose -p rinha-2026-exp04 down
```

## Results

See `bench/leaderboard.md` and `experiments/exp04-ivf-int8/results/`.
