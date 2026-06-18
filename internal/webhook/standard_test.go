package webhook

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/database"
)

func TestStandardWebhookSignAndVerify(t *testing.T) {
	secret, err := GenerateSigningSecret()
	if err != nil {
		t.Fatalf("GenerateSigningSecret() error = %v", err)
	}

	wh, err := NewStandardWebhook(secret)
	if err != nil {
		t.Fatalf("NewStandardWebhook() error = %v", err)
	}

	payload := []byte(`{"type":"watcher.webhook_test","timestamp":"2026-06-18T00:00:00Z","data":{"summary":"test"}}`)
	timestamp := time.Now().UTC()
	signature, err := wh.Sign("evt_test_123", timestamp, payload)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	headers := http.Header{}
	headers.Set(HeaderWebhookID, "evt_test_123")
	headers.Set(HeaderWebhookTimestamp, "123")
	headers.Set(HeaderWebhookSignature, signature)
	headers.Set(HeaderWebhookTimestamp, timeString(timestamp))

	if err := wh.Verify(payload, headers); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestDispatcherSendUsesStandardWebhookHeaders(t *testing.T) {
	secret, err := GenerateSigningSecret()
	if err != nil {
		t.Fatalf("GenerateSigningSecret() error = %v", err)
	}

	var gotHeader http.Header
	var gotBody string
	originalClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotHeader = r.Header.Clone()
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(strings.NewReader("accepted")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	defer func() {
		http.DefaultClient = originalClient
	}()

	d := &Dispatcher{}
	event := &database.WebhookEvent{
		EventID:   "evt_watch_123",
		EventType: EventWebhookTest,
		Payload:   `{"type":"watcher.webhook_test","timestamp":"2026-06-18T00:00:00Z","data":{"summary":"Webhook test"}}`,
	}
	delivery := &database.WebhookDelivery{
		DeliveryID: "delv_test_123",
	}
	attemptTime := time.Unix(1718668800, 0).UTC()
	statusCode, responseBody, signature, err := d.send(context.Background(), ResolvedConfig{
		URL:           "https://example.invalid/webhook",
		SigningSecret: secret,
	}, event, delivery, attemptTime)
	if err != nil {
		t.Fatalf("send() error = %v", err)
	}
	if statusCode != http.StatusAccepted {
		t.Fatalf("statusCode = %d, want %d", statusCode, http.StatusAccepted)
	}
	if responseBody != "accepted" {
		t.Fatalf("responseBody = %q, want %q", responseBody, "accepted")
	}
	if gotBody != event.Payload {
		t.Fatalf("payload mismatch: got %q want %q", gotBody, event.Payload)
	}
	if gotHeader.Get(HeaderWebhookID) != event.EventID {
		t.Fatalf("%s = %q, want %q", HeaderWebhookID, gotHeader.Get(HeaderWebhookID), event.EventID)
	}
	if gotHeader.Get(HeaderWebhookTimestamp) != timeString(attemptTime) {
		t.Fatalf("%s = %q, want %q", HeaderWebhookTimestamp, gotHeader.Get(HeaderWebhookTimestamp), timeString(attemptTime))
	}
	if gotHeader.Get(HeaderWebhookSignature) != signature {
		t.Fatalf("%s = %q, want %q", HeaderWebhookSignature, gotHeader.Get(HeaderWebhookSignature), signature)
	}
	if gotHeader.Get("Authorization") != "" {
		t.Fatalf("Authorization header should be empty, got %q", gotHeader.Get("Authorization"))
	}
}

func TestInvalidSigningSecretIsNotRetryable(t *testing.T) {
	if isRetryable(0, ErrInvalidSigningSecret) {
		t.Fatal("invalid signing secret should not be retryable")
	}
}

func timeString(ts time.Time) string {
	return strconv.FormatInt(ts.UTC().Unix(), 10)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
