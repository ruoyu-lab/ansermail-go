// Package anser is the SDK for 鸿雁 / Anser Mail: send and receive email through its HTTP API.
//
//	client := anser.NewClient("ms_…")
//	email, err := client.Emails.Send(ctx, &anser.SendEmailParams{
//		From:    anser.Address{Email: "hello@acme.com", Name: "Acme"},
//		To:      []anser.Address{{Email: "amy@example.com", Name: "Amy"}},
//		Subject: "Welcome",
//		HTML:    "<p>Hi Amy</p>",
//		Tags:    anser.Tags{"category": "welcome"},
//	})
//
// Or with a published template, by its alias:
//
//	email, err := client.Emails.Send(ctx, &anser.SendEmailParams{
//		To:       []anser.Address{{Email: "amy@example.com"}},
//		Template: &anser.TemplateRef{ID: "welcome", Variables: map[string]any{"name": "Amy"}},
//	})
package anser

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	mathrand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version of this SDK.
const Version = "0.2.0"

// DefaultBaseURL is the hosted service.
const DefaultBaseURL = "https://mail.ruoyulab.dev"

// Client talks to the API. Make one with NewClient and share it.
type Client struct {
	apiKey     string
	baseURL    string
	maxRetries int
	httpClient *http.Client
	headers    http.Header

	Emails          *EmailsService
	Templates       *TemplatesService
	Inbox           *InboxService
	ForwardingRules *ForwardingRulesService
	Domains         *DomainsService
	APIKeys         *APIKeysService
	Webhooks        *WebhooksService
	Suppressions    *SuppressionsService
	Stats           *StatsService
	Recipients      *RecipientsService
	Usage           *UsageService
	Logs            *LogsService
}

// Option changes a new Client.
type Option func(*Client)

// WithBaseURL points the client somewhere else (it defaults to ANSER_BASE_URL, then DefaultBaseURL).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithHTTPClient uses your own http.Client (its Timeout applies to each attempt; the default is 60 s).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithMaxRetries sets how often a request is tried again after a network error, a rate limit or a server
// error; the default is 2.
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = n } }

// WithHeader sends a header with every request.
func WithHeader(name, value string) Option { return func(c *Client) { c.headers.Set(name, value) } }

// NewClient makes a client. An empty apiKey means the ANSER_API_KEY environment variable.
func NewClient(apiKey string, options ...Option) *Client {
	if apiKey == "" {
		apiKey = os.Getenv("ANSER_API_KEY")
	}
	baseURL := os.Getenv("ANSER_BASE_URL")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	c := &Client{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		maxRetries: 2,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		headers:    http.Header{},
	}
	for _, option := range options {
		option(c)
	}
	c.Emails = &EmailsService{c}
	c.Templates = &TemplatesService{c}
	c.Inbox = &InboxService{c}
	c.ForwardingRules = &ForwardingRulesService{c}
	c.Domains = &DomainsService{c}
	c.APIKeys = &APIKeysService{c}
	c.Webhooks = &WebhooksService{c}
	c.Suppressions = &SuppressionsService{c}
	c.Stats = &StatsService{c}
	c.Recipients = &RecipientsService{c}
	c.Usage = &UsageService{c}
	c.Logs = &LogsService{c}
	return c
}

// Ptr returns a pointer to v, for the fields of update params that are left alone when nil.
func Ptr[T any](v T) *T { return &v }

// RequestOption changes one request.
type RequestOption func(*call)

// WithIdempotencyKey makes retries of this request safe: the server runs it once. Sending calls make one up
// when you don't.
func WithIdempotencyKey(key string) RequestOption { return func(c *call) { c.idempotencyKey = key } }

type call struct {
	method         string
	path           string
	query          url.Values
	body           any
	idempotencyKey string
}

func newIdempotencyKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40 // a version 4 UUID, as the other SDKs make
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("anser-%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// do runs a call and decodes the JSON answer into out (when out is not nil).
func (c *Client) do(ctx context.Context, cl *call, out any, options []RequestOption) error {
	body, err := c.send(ctx, cl, options)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("anser: decoding the answer to %s %s: %w", cl.method, cl.path, err)
	}
	return nil
}

func (c *Client) send(ctx context.Context, cl *call, options []RequestOption) ([]byte, error) {
	for _, option := range options {
		option(cl)
	}
	if c.apiKey == "" {
		return nil, errors.New("anser: missing API key: pass one to NewClient or set ANSER_API_KEY")
	}
	target := c.baseURL + "/v1" + cl.path
	if len(cl.query) > 0 {
		target += "?" + cl.query.Encode()
	}
	var payload []byte
	if cl.body != nil {
		var err error
		if payload, err = json.Marshal(cl.body); err != nil {
			return nil, fmt.Errorf("anser: encoding the request: %w", err)
		}
	}
	// A POST runs again only when the server can tell it is the same request
	safe := cl.method != http.MethodPost || cl.idempotencyKey != ""

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, cl.method, target, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		for name, values := range c.headers {
			req.Header[name] = values
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("User-Agent", "anser-mail-go/"+Version)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if cl.idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", cl.idempotencyKey)
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !safe || attempt >= c.maxRetries {
				return nil, &ConnectionError{Err: err}
			}
			if err := sleep(ctx, backoff(attempt)); err != nil {
				return nil, err
			}
			continue
		}
		content, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, &ConnectionError{Err: err}
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			return content, nil
		}
		apiErr := toError(res, content)
		if attempt < c.maxRetries && apiErr.retryable(safe) {
			wait := retryAfter(res.Header)
			if wait == 0 {
				wait = backoff(attempt)
			}
			if err := sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}
		return nil, apiErr
	}
}

func toError(res *http.Response, content []byte) *Error {
	var body struct {
		Error *struct {
			Code    *string `json:"code"`
			Message *string `json:"message"`
			Param   *string `json:"param"`
		} `json:"error"`
	}
	requestID := res.Header.Get("X-Request-Id")
	if json.Unmarshal(content, &body) == nil && body.Error != nil && body.Error.Code != nil &&
		body.Error.Message != nil {
		e := &Error{
			StatusCode: res.StatusCode,
			Code:       *body.Error.Code,
			Message:    *body.Error.Message,
			RequestID:  requestID,
			Header:     res.Header,
		}
		if body.Error.Param != nil {
			e.Param = *body.Error.Param
		}
		return e
	}
	// not JSON: a proxy in between answered
	text := string(content)
	if len(text) > 500 {
		text = text[:500]
	}
	if text == "" {
		text = res.Status
	}
	return &Error{StatusCode: res.StatusCode, Code: "http_error", Message: text, RequestID: requestID, Header: res.Header}
}

func retryAfter(h http.Header) time.Duration {
	seconds, err := strconv.ParseFloat(h.Get("Retry-After"), 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(math.Min(seconds, 60) * float64(time.Second))
}

// backoff: 0.5 s, 1 s, 2 s … up to 8 s, give or take a quarter.
func backoff(attempt int) time.Duration {
	base := math.Min(0.5*math.Pow(2, float64(attempt)), 8)
	return time.Duration(base * (0.75 + mathrand.Float64()/2) * float64(time.Second))
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func escape(segment string) string { return url.PathEscape(segment) }
