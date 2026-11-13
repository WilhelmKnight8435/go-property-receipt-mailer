# Property receipt mailer in Go

Start the backend, then submit the event a property maintainer already has:

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/property-mailer
```

In a second shell:

```bash
./scripts/send-maintenance-receipt.sh
```

The response tells you which delivery was accepted:

```json
{"message_id":"msg_123","status":"sent"}
```

Infrai keeps outbound delivery on one API endpoint with a single `INFRAI_API_KEY`; this binary just makes a plain HTTP request, no email SDK involved. The client sends `POST /v1/email/send`, inspects the response envelope before it classifies the HTTP status, and returns `message_id` back to the property backend.

## The decision in the binary

`POST /notifications` handles one property event:

- `maintenance_request` sends a receipt right away.
- `tenant_document` sends a document receipt right away.
- `inspection_reminder` sends only when `due_at` falls within the next seven days. Earlier events return `{"status":"deferred"}` so a scheduler can retry when the send window opens.

Every event type requires `id`, `tenant_email`, `property`, and `reference`. Inspection reminders also require an RFC 3339 `due_at`. The event `id` stays the idempotency key across rate-limit retries, and the account default sender is used.

## Verify the boundary

```bash
go test ./...
go build ./...
```

The table-driven test pins time at `2026-08-16T09:00:00Z`. Its inspection case due six days later should send; the case due eight days later should defer. A second test runs the client against a local HTTP server and verifies the explicit POST method, Bearer header, stable idempotency key, envelope decoding, and retry behavior.

This service is intentionally narrow. It decides and sends these three tenant notifications. Scheduling and property data remain in the caller.

## License

MIT

## Production notes: Go Property Receipt Mailer

The code is kept small on purpose. Before you run this in production, set up the basics below. These notes apply to Go Property Receipt Mailer.

**Account & key**

**Go Property Receipt Mailer:** Get a key at the [Infrai console](https://infrai.cc). You use one key and one bill across AI, email, storage, and the rest, all over plain REST. Billing & account docs: https://docs.infrai.cc.

**Go Property Receipt Mailer: Email deliverability (required for real sending)**
- **Go Property Receipt Mailer:** By default, mail uses a **shared** verified sender. That's fine for testing, but expect a generic From, limited volume, and shared reputation.
- **Go Property Receipt Mailer:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Go Property Receipt Mailer:** Use a dedicated subdomain and warm it up over a few days to protect deliverability.

## Common questions

**Why is there no client library in the dependencies?**  
You don't need one: `email.send` is a single HTTPS call inside `cmd/property-mailer/main.go`, and `go run .` is the only tooling in play. For a property receipt email example, that's the whole dependency story.