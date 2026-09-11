package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	propertymail "github.com/example/property-receipt-mailer/internal"
)

type server struct {
	client *propertymail.Client
	now    func() time.Time
}

func (s server) notify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var event propertymail.Event
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	message, send, err := propertymail.MessageFor(event, s.now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !send {
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "deferred"})
		return
	}
	messageID, err := s.client.Send(r.Context(), event, message)
	if err != nil {
		var apiErr *propertymail.APIError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			writeJSON(w, apiErr.HTTPStatus, map[string]any{"error": apiErr.Code, "message": apiErr.Message})
			return
		}
		log.Printf("send notification: %v", err)
		http.Error(w, "email delivery unavailable", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "sent", "message_id": messageID})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	s := server{client: &propertymail.Client{APIKey: key, MaxRetries: 3}, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("/notifications", s.notify)
	log.Printf("property mailer listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
