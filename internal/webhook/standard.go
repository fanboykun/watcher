package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderWebhookID        = "webhook-id"
	HeaderWebhookSignature = "webhook-signature"
	HeaderWebhookTimestamp = "webhook-timestamp"

	WebhookSecretPrefix = "whsec_"
)

var (
	base64enc = base64.StdEncoding
	tolerance = 5 * time.Minute

	ErrInvalidSigningSecret = errors.New("invalid signing secret")
	ErrMissingSigningPrefix = errors.New("missing whsec_ prefix")
	ErrRequiredHeaders      = errors.New("missing required headers")
	ErrInvalidHeaders       = errors.New("invalid signature headers")
	ErrNoMatchingSignature  = errors.New("no matching signature found")
	ErrMessageTooOld        = errors.New("message timestamp too old")
	ErrMessageTooNew        = errors.New("message timestamp too new")
)

type StandardWebhook struct {
	key []byte
}

func NewStandardWebhook(secret string) (*StandardWebhook, error) {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return nil, fmt.Errorf("unable to create webhook, err: %w", ErrInvalidSigningSecret)
	}
	if !strings.HasPrefix(trimmed, WebhookSecretPrefix) {
		return nil, fmt.Errorf("unable to create webhook, err: %w", errors.Join(ErrInvalidSigningSecret, ErrMissingSigningPrefix))
	}
	key, err := base64enc.DecodeString(strings.TrimPrefix(trimmed, WebhookSecretPrefix))
	if err != nil {
		return nil, fmt.Errorf("unable to create webhook, err: %w", errors.Join(ErrInvalidSigningSecret, err))
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("unable to create webhook, err: %w", ErrInvalidSigningSecret)
	}
	return &StandardWebhook{key: key}, nil
}

func GenerateSigningSecret() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate signing secret: %w", err)
	}
	return WebhookSecretPrefix + base64enc.EncodeToString(key), nil
}

func (wh *StandardWebhook) Verify(payload []byte, headers http.Header) error {
	return wh.verify(payload, headers, true)
}

func (wh *StandardWebhook) VerifyIgnoringTimestamp(payload []byte, headers http.Header) error {
	return wh.verify(payload, headers, false)
}

func (wh *StandardWebhook) verify(payload []byte, headers http.Header, enforceTolerance bool) error {
	msgID := headers.Get(HeaderWebhookID)
	msgSignature := headers.Get(HeaderWebhookSignature)
	msgTimestamp := headers.Get(HeaderWebhookTimestamp)
	if msgID == "" || msgSignature == "" || msgTimestamp == "" {
		return fmt.Errorf("unable to verify payload, err: %w", ErrRequiredHeaders)
	}

	timestamp, err := parseTimestampHeader(msgTimestamp)
	if err != nil {
		return fmt.Errorf("unable to verify payload, err: %w", err)
	}
	if enforceTolerance {
		if err := verifyTimestamp(timestamp); err != nil {
			return fmt.Errorf("unable to verify payload, err: %w", err)
		}
	}

	_, expectedSignature, err := wh.sign(msgID, timestamp, payload)
	if err != nil {
		return fmt.Errorf("unable to verify payload, err: %w", err)
	}

	for _, versionedSignature := range strings.Split(msgSignature, " ") {
		parts := strings.Split(versionedSignature, ",")
		if len(parts) < 2 || parts[0] != "v1" {
			continue
		}
		if hmac.Equal([]byte(parts[1]), expectedSignature) {
			return nil
		}
	}

	return fmt.Errorf("unable to verify payload, err: %w", ErrNoMatchingSignature)
}

func (wh *StandardWebhook) Sign(msgID string, timestamp time.Time, payload []byte) (string, error) {
	version, signature, err := wh.sign(msgID, timestamp, payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s,%s", version, signature), nil
}

func (wh *StandardWebhook) sign(msgID string, timestamp time.Time, payload []byte) (string, []byte, error) {
	toSign := fmt.Sprintf("%s.%d.%s", msgID, timestamp.Unix(), payload)

	h := hmac.New(sha256.New, wh.key)
	if _, err := h.Write([]byte(toSign)); err != nil {
		return "", nil, err
	}

	sig := make([]byte, base64enc.EncodedLen(h.Size()))
	base64enc.Encode(sig, h.Sum(nil))
	return "v1", sig, nil
}

func parseTimestampHeader(timestampHeader string) (time.Time, error) {
	timeInt, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse timestamp header, err: %w", errors.Join(err, ErrInvalidHeaders))
	}
	return time.Unix(timeInt, 0), nil
}

func verifyTimestamp(timestamp time.Time) error {
	now := time.Now()
	if now.Sub(timestamp) > tolerance {
		return ErrMessageTooOld
	}
	if timestamp.After(now.Add(tolerance)) {
		return ErrMessageTooNew
	}
	return nil
}
