# Leaderboard

Median of three k6 `test.js` runs against each experiment's `docker-compose.yml`. All experiments run under the same budget (1 CPU / 350 MB total). Run on this WSL i7-14700HX rig; Mac-Mini-Late-2014 projections in parentheses where useful.

> **One row per experiment.** Adding/changing an experiment ⇒ overwrite that row + bump the `notes` cell. The history is in `git log` and in `experiments/expXX-*/results/`.

| exp | p99 | score_p99 | FP | FN | Err | failure_rate | score_det | **final_score** | peak mem (api1+api2) | notes |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| exp01-brute-float32 | 2002 ms | −3000 | 0 | 0 | 13 572 | 98.1 % | −3000 | **−6000** | 68 + 90 MiB | both cutoffs active. 263 successful responses (123 TP / 140 TN) — when it does respond, brute force is **exactly** correct. Throughput cap ≈ 60 rps at 0.9 CPU; tested target was 900 rps. |
| exp02-brute-int8 | 2002 ms | −3000 | 0 | 0 | 13 108 | 94.7 % | −3000 | **−6000** | ~52 + ~52 MiB | both cutoffs active. 738 successful responses (328 TP / 410 TN) — 0 FP/0 FN, int8 quantization is lossless for KNN decisions. 4× smaller index (42 MB) reduces page-fault pressure; throughput marginally better (~6 rps/replica). Need ANN for real gains. |
| exp03-parallel-int8 | 2002 ms | −3000 | 0 | 0 | 13 124 | 94.8 % | −3000 | **−6000** | ~52 + ~52 MiB | GOMAXPROCS=2 vs exp02 (GOMAXPROCS=1). 726 successful vs 738 — statistically identical. Confirmed: CPU budget (0.45 CPU / 15 ms per query = 30 rps/replica) is the hard ceiling. Concurrency cannot bypass it. ANN required. |
| exp04-ivf-int8 | 176 ms | +753 | 26 | 44 | 0 | 0.13 % | +1873 | **+2627** | ~10 + ~10 MiB | **First positive score.** IVF C=256, nprobe=8. 117 μs/query (128× vs brute force). 54 011/54 100 completed. Recall 99.87%: 26 FP + 44 FN from ANN approximation. nprobe=16 tested: worse overall (+2063) because latency penalty outweighs recall gain. |
| exp05-ivf-512 | 177 ms | +751 | 26 | 44 | 0 | 0.13 % | +1873 | **+2624** | ~10 + ~10 MiB | IVF C=512, nprobe=16. C=512 vs C=256 no improvement — same nprobe=16 searches ~93k vecs both ways. All 70 errors persist: root cause is int8 quantization distortion, not ANN miss (confirmed by nprobe=256 test). |
| exp06-ivf-adaptive | 431 ms | +365 | 26 | 43 | 0 | 0.13 % | +1881 | **+2246** | ~10 + ~10 MiB | IVF C=512 adaptive: int8 fast pass, BruteForce int8 fallback for nFraud=2 or 3. Saves only 1 FN. 69/70 errors are quantization-caused (int8 BruteForce still wrong). p99 penalty kills the score. |
| exp07-ivf-f32 | 887 ms | +52 | 0 | 1 | 0 | 0 % | +2819 | **+2871** | ~0 + ~0 MiB | IVF C=512, FormatIVF_F32 (float32 cluster vectors), nprobe=16. Eliminates quantization error: 69 errors drop to 1 FN. But p99=887ms because inner loop uses slow byte-by-byte bit manipulation. |
| exp08-ivf-f32-fast | 163 ms | +789 | 0 | 1 | 0 | 0 % | +2819 | **+3608** | ~0 + ~0 MiB | Same as exp07 but searchAndCountF32 uses unsafe.Pointer cast + loop unrolling. 5.5× speedup (887ms→163ms). **Best detection: 0 FP + 1 FN.** |
| exp09-ivf-f32-np8 | 172 ms | +776 | 1 | 1 | 0 | 0 % | +2790 | +3566 | ~0 + ~0 MiB | IVF C=512 F32, nprobe=8. Typical p99 165–180 ms; HAProxy bottleneck (0.05 CPU, CFS-throttled every 100ms). Best observed 94 ms (+3816) P-core. |
| exp10-ivf-f32-np4 | 76 ms | +1117 | 5 | 5 | 0 | 0.02 % | +2603 | +3720 | ~0 + ~0 MiB | nprobe=4. Accuracy degradation (5 FP + 5 FN) outweighs latency gain. |
| exp11-ivf-f32-np6 | 84 ms | +1077 | 1 | 2 | 0 | 0.01 % | +2729 | +3806 | ~0 + ~0 MiB | nprobe=6. Close to exp09 but slightly worse accuracy at similar latency. |
| exp12-ivf-f32-c1024 | 143 ms | +845 | 3 | 1 | 0 | 0.01 % | +2746 | +3592 | ~0 + ~0 MiB | C=1024, nprobe=8. Centroid scan (57 KB, spills L1 cache) adds overhead; non-uniform clusters (max=13908) hurt p99. |
| exp13-ivf-f32-c1024-np16 | 181 ms | +743 | 0 | 1 | 0 | 0 % | +2819 | +3563 | ~0 + ~0 MiB | C=1024, nprobe=16. Centroid opt (unsafe.Pointer unroll) REGRESSED — Go compiler auto-vectorizes []float32 slice loop better. |
| exp14-profiling | 179 ms | +745 | 1 | 1 | 0 | 0 % | +2790 | +3536 | ~0 + ~0 MiB | exp09 + per-phase timing. Revealed: handler=120–770µs total; 99.7% of p99 is outside the handler. HAProxy CFS throttle was the root cause. |
| exp15-haproxy-reuse | 180 ms | +745 | 1 | 1 | 0 | 0 % | +2790 | +3535 | ~0 + ~0 MiB | Added `http-reuse aggressive` to HAProxy. No improvement — HAProxy CPU was the bottleneck, not connection reuse. |
| exp16-haproxy-cpu | 21 ms | +1670 | 1 | 1 | 0 | 0 % | +2790 | +4460 | ~0 + ~0 MiB | HAProxy CPU 0.05→0.10, api 0.45→0.425. HAProxy still at 9.99% (100% of limit) under load — still throttled. |
| **exp17-haproxy-cpu2** | **1.64 ms** | **+2785** | **1** | **1** | **0** | **0 %** | **+2790** | **+5575** | **~0 + ~0 MiB** | **BEST.** HAProxy CPU 0.10→0.15, api 0.425→0.40. HAProxy throttle resolved. Three k6 runs: 5575 / 5538 / 5651 → median 5575. p99 1.38–1.79ms. Root cause of 170ms was CFS throttle on HAProxy (5ms quota per 100ms period). |

## Columns

- **p99** — `http_req_duration` p(99), ms. Feeds `score_p99 = 1000 · log10(1000 / max(p99, 1))` (saturates at +3000 below 1 ms, falls to −3000 above 2000 ms).
- **score_p99 / score_det / final_score** — taken directly from `test/results.json` after a successful run.
- **failure_rate** — `(FP + FN + Err) / N`. > 15 % triggers the detection cutoff (`score_det = −3000`).
- **peak mem** — max `MemUsage` from `docker stats` during the run.
- **notes** — anything load-bearing (e.g. "memory cutoff triggered", "ANN nprobe=8").

## How to reproduce a row

```bash
cd experiments/<expXX-name>
docker compose up -d --build
until [ "$(curl -s -o /dev/null -w '%{http_code}' http://localhost:9999/ready)" = "200" ]; do sleep 1; done
docker run --rm -i --network host \
  -v $PWD/../../../rinha-de-backend-2026/test:/test \
  grafana/k6:latest run /test/test.js
cp ../../../rinha-de-backend-2026/test/results.json results/run-$(date -u +%F-%H%M).json
docker compose down
```
