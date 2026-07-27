# Rent Collector

A small, correctness-focused billing engine for monthly rent collection, built with Go and [Fiber](https://gofiber.io/). It manages leases, generates monthly invoices idempotently, applies one-time late fees, and records payments with idempotency keys.

The point of this MVP is not feature count — it is **billing correctness**: an explicit invoice state machine, idempotent operations that are safe to retry, and integer money handling.

## Architecture

```
cmd/api/            entrypoint (Fiber server)
internal/billing/   domain core: Lease, Invoice, Payment, state machine, billing service
internal/store/     in-memory repositories behind interfaces (Postgres is next)
internal/httpapi/   thin HTTP layer: JSON <-> domain, typed errors -> status codes
```

Repository interfaces are defined in `internal/billing` (the consumer), so the domain has zero knowledge of storage.

## Invoice state machine

```
            issue                    full payment
  draft ----------> issued ----------------------> paid
                       |                             ^
                       | past due date               |
                       | (+ one-time late fee)       | full payment
                       v                             |
                    overdue -------------------------+
```

Any other transition (e.g. `paid -> issued`, `draft -> paid`) returns a typed `ErrIllegalTransition`, which the API maps to HTTP 409.

## API

| Method | Path                 | Body / Query                                    | Description |
|--------|----------------------|--------------------------------------------------|-------------|
| GET    | `/health`            | —                                                | Liveness check |
| POST   | `/leases`            | `{tenant, property, rent_thb, due_day}`          | Create an active lease |
| GET    | `/leases`            | —                                                | List leases |
| POST   | `/invoices/generate` | `{month: "2026-08"}`                             | Generate issued invoices for all active leases (idempotent) |
| POST   | `/invoices/late-fees`| `{as_of: "2026-08-10"}` (optional, defaults now) | Mark past-due issued invoices overdue + one-time late fee |
| GET    | `/invoices`          | `?month=2026-08` (optional)                      | List invoices |
| GET    | `/invoices/:id`      | —                                                | Get one invoice |
| POST   | `/payments`          | `{invoice_id, amount_thb, idempotency_key}`      | Record a (partial) payment, idempotent per key |

### curl examples

```bash
# Create a lease (9,000 THB/month, due on the 5th)
curl -s -X POST localhost:8080/leases \
  -H 'Content-Type: application/json' \
  -d '{"tenant":"Somchai","property":"Room 101","rent_thb":9000,"due_day":5}'

# Generate August invoices — run it twice, the second run skips everything
curl -s -X POST localhost:8080/invoices/generate \
  -H 'Content-Type: application/json' -d '{"month":"2026-08"}'
# -> {"month":"2026-08","created":1,"skipped":0}
# -> {"month":"2026-08","created":0,"skipped":1}   (second run)

# Apply late fees as of a date (500 THB flat, applied at most once)
curl -s -X POST localhost:8080/invoices/late-fees \
  -H 'Content-Type: application/json' -d '{"as_of":"2026-08-10"}'

# Pay an invoice (safe to retry with the same idempotency key)
curl -s -X POST localhost:8080/payments \
  -H 'Content-Type: application/json' \
  -d '{"invoice_id":"<id>","amount_thb":9500,"idempotency_key":"pay-aug-101"}'

curl -s 'localhost:8080/invoices?month=2026-08'
```

## Correctness decisions

- **Money is `int64` satang.** Never float: binary floats cannot represent 0.1 THB exactly and rounding drift compounds across invoices. THB amounts are converted once at the JSON boundary; all arithmetic is integer.
- **Idempotent invoice generation.** `(lease_id, month)` is a unique key. Re-running generation for a month creates nothing new and reports `created` vs `skipped`, so a crashed or retried cron run is harmless.
- **One-time late fee.** Overdue marking transitions `issued -> overdue` (so already-overdue invoices are not reprocessed) and a `late_fee_applied` flag guarantees the fee is charged at most once even if the job re-runs.
- **Payment idempotency keys.** Replaying the same key returns the original payment and leaves the invoice untouched — no double-recording on client retries. Partial payments accumulate; the invoice transitions to `paid` only when rent + late fee is fully covered. Overpayment is rejected.
- **Explicit state machine.** A single transition table is the source of truth; every state change goes through `TransitionTo`, and illegal moves are typed errors surfaced as HTTP 409.
- **Concurrency.** A service-level mutex serializes billing commands so check-then-insert idempotency is race-free in memory. With Postgres this responsibility moves to unique constraints and transactions.

## Run

```bash
go test ./...
go run ./cmd/api          # PORT=8080 by default

# Docker
docker build -t rent-collector .
docker run -p 8080:8080 rent-collector
```

## Roadmap

- **Postgres** behind the existing repo interfaces, with `UNIQUE (lease_id, month)` and `UNIQUE (idempotency_key)` constraints enforcing idempotency at the DB level.
- **Scheduled generation** — cron worker that runs monthly generation and daily late-fee sweeps (both already safe to re-run).
- **PromptPay QR** on invoices for tenant payment.
- **Reminders** via LINE Notify before and after the due date.
- **Next.js dashboard** for landlords: occupancy, aging report, collection rate.
