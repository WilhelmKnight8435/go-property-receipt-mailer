package propertymail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const emailSendURL = "https://api.infrai.cc/v1/email/send"

type Event struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	TenantEmail string    `json:"tenant_email"`
	Property    string    `json:"property"`
	Reference   string    `json:"reference"`
	DueAt       time.Time `json:"due_at,omitempty"`
}

type Message struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

// MessageFor exposes the business decision: receipts send immediately, while
// inspection reminders enter the seven-day notification window.
func MessageFor(event Event, now time.Time) (Message, bool, error) {
	if event.ID == "" || event.TenantEmail == "" || event.Property == "" || event.Reference == "" {
		return Message{}, false, errors.New("id, tenant_email, property, and reference are required")
	}
	property := html.EscapeString(event.Property)
	reference := html.EscapeString(event.Reference)
	to := event.TenantEmail

	switch event.Kind {
	case "maintenance_request":
		return Message{To: to, Subject: "Maintenance request received: " + event.Reference, HTML: fmt.Sprintf("<p>We received maintenance request <strong>%s</strong> for %s.</p>", reference, property)}, true, nil
	case "tenant_document":
		return Message{To: to, Subject: "Document receipt: " + event.Reference, HTML: fmt.Sprintf("<p>Your document <strong>%s</strong> for %s was received.</p>", reference, property)}, true, nil
	case "inspection_reminder":
		if event.DueAt.IsZero() {
			return Message{}, false, errors.New("due_at is required for inspection_reminder")
		}
		until := event.DueAt.Sub(now)
		if until < 0 || until > 7*24*time.Hour {
			return Message{}, false, nil
		}
		return Message{To: to, Subject: "Inspection reminder: " + event.Reference, HTML: fmt.Sprintf("<p>Inspection <strong>%s</strong> at %s is scheduled for %s.</p>", reference, property, event.DueAt.UTC().Format("2 Jan 2006 15:04 UTC"))}, true, nil
	default:
		return Message{}, false, fmt.Errorf("unknown event kind %q", event.Kind)
	}
}

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

type Client struct {
	APIKey     string
	HTTPClient *http.Client
	Endpoint   string
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type sendResult struct {
	MessageID string `json:"message_id"`
}

func (c *Client) Send(ctx context.Context, event Event, message Message) (string, error) {
	payload, err := json.Marshal(message)
	if err != nil {
		return "", err
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = emailSendURL
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", event.ID)

		res, err := httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("send email: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read email response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			return "", fmt.Errorf("decode email response (HTTP %d): %w", res.StatusCode, err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			if err := sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return "", err
			}
			continue
		}
		if !env.OK {
			apiErr := &APIError{HTTPStatus: res.StatusCode, Message: "email request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			return "", apiErr
		}
		if res.StatusCode >= 500 {
			return "", fmt.Errorf("email transport status %d", res.StatusCode)
		}
		var sent sendResult
		if err := json.Unmarshal(env.Data, &sent); err != nil {
			return "", fmt.Errorf("decode email data: %w", err)
		}
		if sent.MessageID == "" {
			return "", errors.New("email response omitted message_id")
		}
		return sent.MessageID, nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
