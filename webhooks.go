package anser

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// WebhookEvent is a verified push. Data depends on Type: use EmailData or ReceivedData to decode it.
type WebhookEvent struct {
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

// EmailData decodes the data of an email event (email.sent, email.delivered, email.bounced, …).
func (e *WebhookEvent) EmailData() (*EmailEventData, error) {
	var data EmailEventData
	return &data, json.Unmarshal(e.Data, &data)
}

// ReceivedData decodes the data of an email.received event.
func (e *WebhookEvent) ReceivedData() (*ReceivedEventData, error) {
	var data ReceivedEventData
	return &data, json.Unmarshal(e.Data, &data)
}

// VerifyOptions changes how VerifyWebhook checks the timestamp.
type VerifyOptions struct {
	// Tolerance is how old (or how far ahead) a push may be; the default is 5 minutes.
	Tolerance time.Duration
	// Now is for tests.
	Now time.Time
}

func verifyFail(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrWebhookVerification}, args...)...)
}

// VerifyWebhook checks a webhook push (Standard Webhooks) and returns its event. payload must be the raw
// request body. Every error it returns wraps ErrWebhookVerification.
func VerifyWebhook(payload []byte, header http.Header, secret string, options ...VerifyOptions) (*WebhookEvent, error) {
	var opts VerifyOptions
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.Tolerance == 0 {
		opts.Tolerance = 5 * time.Minute
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	id := header.Get("webhook-id")
	timestamp := header.Get("webhook-timestamp")
	signatures := header.Get("webhook-signature")
	if id == "" || timestamp == "" || signatures == "" {
		return nil, verifyFail("missing webhook-id, webhook-timestamp or webhook-signature")
	}
	sentAt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, verifyFail("the timestamp is not a number")
	}
	if math.Abs(float64(opts.Now.Unix()-sentAt)) > opts.Tolerance.Seconds() {
		return nil, verifyFail("the timestamp is too old or too far ahead")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return nil, verifyFail("the secret is not valid")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(payload)
	expected := []byte(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	for _, signature := range strings.Split(signatures, " ") {
		value, ok := strings.CutPrefix(signature, "v1,")
		if ok && hmac.Equal([]byte(value), expected) {
			var event WebhookEvent
			if err := json.Unmarshal(payload, &event); err != nil {
				return nil, verifyFail("the body is not JSON")
			}
			return &event, nil
		}
	}
	return nil, verifyFail("the signature does not match")
}
