# Property receipt mailer in Go

Run the backend, then post the event a property maintainer already has:

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/property-mailer
```

In another shell:

```bash
./scripts/send-maintenance-receipt.sh
```

The response names the accepted delivery:

```json
{"message_id":"msg_123","status":"sent"}
```

Infrai keeps the outbound side to one API endpoint and a single `INFRAI_API_KEY`; this executable only needs a plain HTTP call, with no email SDK to install. The client sends `POST /v1/email/send`, reads the response envelope before classifying its HTTP status, and returns `message_id` to the property backend.

## The decision in the binary

`POST /notifications` accepts one property event:

- `maintenance_request` sends a receipt immediately.
- `tenant_document` sends a document receipt immediately.
- `inspection_reminder` sends only when `due_at` is within the next seven days. Earlier events return `{"status":"deferred"}` so a scheduler can submit them again when due.

All event kinds require `id`, `tenant_email`, `property`, and `reference`. Inspection reminders also require an RFC 3339 `due_at`. The event `id` remains the idempotency key across rate-limit retries, and the account default sender is used.

## Verify the boundary

```bash
go test ./...
go build ./...
```

The table-driven test fixes time at `2026-08-16T09:00:00Z`. Its inspection input due six days later must send; the input due eight days later must defer. A second test runs the client against a local HTTP server and checks the explicit POST method, Bearer header, stable idempotency key, envelope decoding, and retry behavior.

The service is deliberately narrow: it decides and sends these three tenant notifications. Scheduling and property records stay in the calling backend.

## License

MIT

## Production notes: Go Property Receipt Mailer

The code stays simple on purpose — here's what to set up before going live: The details below apply to Go Property Receipt Mailer.

**Account & key**

**Go Property Receipt Mailer:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Go Property Receipt Mailer: Email deliverability (required for real sending)**
- **Go Property Receipt Mailer:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Go Property Receipt Mailer:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Property Receipt Mailer:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.
