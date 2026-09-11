package propertymail

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMessageFor(t *testing.T) {
	now := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		event    Event
		wantSend bool
		wantErr  bool
	}{
		{"maintenance receipt", Event{ID: "evt-1", Kind: "maintenance_request", TenantEmail: "tenant@example.com", Property: "17 Pine St", Reference: "MR-204"}, true, false},
		{"document receipt", Event{ID: "evt-2", Kind: "tenant_document", TenantEmail: "tenant@example.com", Property: "17 Pine St", Reference: "LEASE-9"}, true, false},
		{"inspection within window", Event{ID: "evt-3", Kind: "inspection_reminder", TenantEmail: "tenant@example.com", Property: "17 Pine St", Reference: "INS-8", DueAt: now.Add(6 * 24 * time.Hour)}, true, false},
		{"inspection outside window", Event{ID: "evt-4", Kind: "inspection_reminder", TenantEmail: "tenant@example.com", Property: "17 Pine St", Reference: "INS-9", DueAt: now.Add(8 * 24 * time.Hour)}, false, false},
		{"missing inspection date", Event{ID: "evt-5", Kind: "inspection_reminder", TenantEmail: "tenant@example.com", Property: "17 Pine St", Reference: "INS-10"}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, send, err := MessageFor(tt.event, now)
			if send != tt.wantSend || (err != nil) != tt.wantErr {
				t.Fatalf("send=%v err=%v, want send=%v err=%v", send, err, tt.wantSend, tt.wantErr)
			}
		})
	}
}

func TestClientRetriesWithStableIdempotencyKey(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Idempotency-Key") != "evt-42" {
			t.Fatalf("unexpected request method or headers")
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "from") {
			t.Fatalf("request must use the account default sender")
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"data":null,"error":{"code":"RATE_LIMITED","message":"retry later"},"metadata":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{"message_id":"msg-123"},"error":null,"metadata":{}}`))
	}))
	defer server.Close()

	client := Client{APIKey: "test-key", Endpoint: server.URL, MaxRetries: 1, Sleep: func(context.Context, time.Duration) error { return nil }}
	id, err := client.Send(context.Background(), Event{ID: "evt-42"}, Message{To: "tenant@example.com", Subject: "Receipt", HTML: "<p>Received</p>"})
	if err != nil || id != "msg-123" || requests != 2 {
		t.Fatalf("id=%q requests=%d err=%v", id, requests, err)
	}
}
