# Rinha de Backend 2026 — pure Go submission

Fraud-detection API for the [Rinha de Backend 2026](https://github.com/zanfranceschi/rinha-de-backend-2026) challenge, built in pure Go (stdlib `net/http`, no web frameworks).

- **Plan:** [`PLAN.md`](./PLAN.md)
- **Development log:** [`docs.md`](./docs.md)

## Quick start (once Docker is set up)

```bash
docker compose up --build
# in another shell
curl -i http://localhost:9999/ready
```

## Layout

```
cmd/api          # HTTP server
internal/        # vec, quantize, ivf, mmap, httpx, bench (added in later phases)
experiments/     # exp01..exp06 — one folder per benchmarked search strategy
bench/           # run-all.sh, compare.go, leaderboard.md
```

## Status

P1 skeleton only. Always-approve response. See [`docs.md`](./docs.md) for the latest iteration.
