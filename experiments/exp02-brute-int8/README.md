# exp02 — brute-force int8

Brute-force k=5 KNN over a FormatInt8 index. Vectors are quantized from float64 to uint8:

- `[0, 1] → [0, 254]` (linear, rounded to nearest)
- `-1` (null-last-transaction sentinel) → `255`

Index shrinks from 168 MB (FLAT32) to **42 MB** (4× smaller), reducing page-fault pressure under cgroup memory limits.

## Stack

- `haproxy:2.9-alpine` round-robin on port 9999.
- 2 × Go API binaries (distroless), each mmaps `/data/index.bin`.
- `data-prep` init service copies the pre-built int8 index into the shared volume.

## Hypothesis

| Metric | Predicted |
|---|---|
| p99 | Similar to exp01 in warm-cache benchmark (15 ms/query), but in production the smaller index means fewer page evictions → lower p99 tail |
| score_p99 | Still likely negative (need ~3 ms/query to break even) |
| FP + FN | Slight increase vs exp01 due to uint8 quantization error; expected < 500 out of 54 100 |
| score_det | ~+2000 (if quantization doesn't push failure_rate above 15%) |
| final_score | Negative but better than exp01 |

## Memory budget

| Service | CPU | Memory |
|---|---|---|
| haproxy | 0.05 | 15 MB |
| data-prep | 0.05 | 25 MB |
| api1 | 0.45 | 155 MB |
| api2 | 0.45 | 155 MB |
| **total** | **1.00** | **350 MB** |

42 MB index + ~10 MB Go runtime = ~52 MB per api at idle. Headroom is substantial compared to exp01.

## Run

```bash
cd experiments/exp02-brute-int8
docker compose up -d --build
until [ "$(curl -s -o /dev/null -w '%{http_code}' http://localhost:9999/ready)" = "200" ]; do sleep 1; done

# k6 smoke
rm -rf /tmp/k6-stage && mkdir -p /tmp/k6-stage/test
cp /home/italo/rinha-2026/rinha-de-backend-2026/test/*.{js,json} /tmp/k6-stage/test/
chmod -R 777 /tmp/k6-stage
docker run --rm -i --network host \
  -v /tmp/k6-stage:/work -w /work --user "$(id -u):$(id -g)" \
  grafana/k6:latest run test/smoke.js

# k6 full load
docker run --rm -i --network host \
  -v /tmp/k6-stage:/work -w /work --user "$(id -u):$(id -g)" \
  grafana/k6:latest run test/test.js
cat /tmp/k6-stage/test/results.json | jq

docker compose -p rinha-2026-exp02 down
```

## Results

See `bench/leaderboard.md` and `experiments/exp02-brute-int8/results/`.
