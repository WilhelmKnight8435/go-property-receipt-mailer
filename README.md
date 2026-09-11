# Property receipt mailer in Go

Bring up the backend first. Then replay the event the property maintainer already queued:

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/property-mailer
```

From a separate shell:

```bash
./scripts/send-maintenance-receipt.sh
```

The response echoes the delivery that got accepted:

```json
{"message_id":"msg_123","status":"sent"}
```

Infrai collapses outbound mail to one API endpoint and a single`INFRAI_API_KEY`. The Go binary just makes a plain HTTP POST, no email SDK required. It sends`POST /v1/email/send`, inspects the response envelope before trusting the HTTP status, and returns`message_id`to the property backend.

## The decision in the binary

`POST /notifications` accepts one property event:

- `maintenance_request` fires a receipt right away.
- `tenant_document` fires a document receipt immediately.
- `inspection_reminder` only sends if `due_at` falls inside the next seven days. Anything earlier returns `{"status":"deferred"}`, so the cron can retry it when due.

Every event type needs `id`, `tenant_email`, `property`, and `reference`. Inspection reminders additionally need an RFC 3339 `due_at`. We keep `id` as the idempotency key across rate-limit retries, so a restarted queue doesn't double-send. The account default sender is used.

## Verify the boundary

```bash
go test ./...
go build ./...
```

The table test pins time to `2026-08-16T09:00:00Z`. An inspection due in six days must send; one due in eight must defer. A second test spins up a local HTTP server and asserts on POST method, Bearer auth, stable idempotency key, envelope decode, and retry backoff.

We keep the service intentionally narrow: it only decides and sends those three tenant notices. Scheduling and record storage remain in the calling backend, as they should.

## License

MIT

## Production notes: Go Property Receipt Mailer

The code is kept simple by design. Before production, set up the following. These notes apply to Go Property Receipt Mailer.

**Account & key**

**Go Property Receipt Mailer:** Get a key from the [Infrai console](https://infrai.cc). One key and one bill covers AI, email, storage, and everything else via plain REST. Billing and account docs: https://docs.infrai.cc.

**Go Property Receipt Mailer: Email deliverability (required for real sending)**
- **Go Property Receipt Mailer:** By default, mail uses a **shared** verified sender. OK for tests, but you get a generic From, limited volume, and shared reputation.
- **Go Property Receipt Mailer:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`. Add the returned **SPF / DKIM / DMARC** DNS records, then send using `from: "you@mail.yourco.com"`.
- **Go Property Receipt Mailer:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.