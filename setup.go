package ansermail

import (
	"context"
	"iter"
	"net/http"
	"net/url"
	"strconv"
)

// DomainsService manages sending domains.
type DomainsService struct{ c *Client }

// Create adds a domain and its SES identity; add the DNS records it returns.
func (s *DomainsService) Create(ctx context.Context, name string, options ...RequestOption) (*Domain, error) {
	var out Domain
	body := map[string]string{"name": name}
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/domains", body: body}, &out, options)
}

// List returns every domain.
func (s *DomainsService) List(ctx context.Context, options ...RequestOption) ([]Domain, error) {
	var out List[Domain]
	return out.Data, s.c.do(ctx, &call{method: http.MethodGet, path: "/domains"}, &out, options)
}

// Get returns a domain with its DNS records.
func (s *DomainsService) Get(ctx context.Context, id string, options ...RequestOption) (*Domain, error) {
	var out Domain
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/domains/" + escape(id)}, &out, options)
}

// Update turns tracking or receiving on or off.
func (s *DomainsService) Update(ctx context.Context, id string, params *UpdateDomainParams, options ...RequestOption) (*Domain, error) {
	var out Domain
	return &out, s.c.do(ctx, &call{method: http.MethodPatch, path: "/domains/" + escape(id), body: params}, &out, options)
}

// Verify checks SES and the DNS records again now.
func (s *DomainsService) Verify(ctx context.Context, id string, options ...RequestOption) (*Domain, error) {
	var out Domain
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/domains/" + escape(id) + "/verify"}, &out, options)
}

// AddToCloudflare adds the records that are not in effect yet at Cloudflare, with an API token that can edit the
// zone's DNS. The token is used for this call only and never stored.
func (s *DomainsService) AddToCloudflare(ctx context.Context, id, token string, options ...RequestOption) (*DNSSetup, error) {
	var out DNSSetup
	body := map[string]string{"token": token}
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/domains/" + escape(id) + "/dns/cloudflare", body: body}, &out, options)
}

// Delete deletes a domain.
func (s *DomainsService) Delete(ctx context.Context, id string, options ...RequestOption) (*Deleted, error) {
	var out Deleted
	return &out, s.c.do(ctx, &call{method: http.MethodDelete, path: "/domains/" + escape(id)}, &out, options)
}

// APIKeysService manages API keys.
type APIKeysService struct{ c *Client }

// Create makes a key; its token comes only in this answer.
func (s *APIKeysService) Create(ctx context.Context, params *CreateAPIKeyParams, options ...RequestOption) (*CreatedAPIKey, error) {
	var out CreatedAPIKey
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/api-keys", body: params}, &out, options)
}

// List returns every key, without tokens.
func (s *APIKeysService) List(ctx context.Context, options ...RequestOption) ([]APIKey, error) {
	var out List[APIKey]
	return out.Data, s.c.do(ctx, &call{method: http.MethodGet, path: "/api-keys"}, &out, options)
}

// Delete deletes a key.
func (s *APIKeysService) Delete(ctx context.Context, id string, options ...RequestOption) (*Deleted, error) {
	var out Deleted
	return &out, s.c.do(ctx, &call{method: http.MethodDelete, path: "/api-keys/" + escape(id)}, &out, options)
}

// WebhooksService manages webhooks. Check the pushes you receive with VerifyWebhook.
type WebhooksService struct{ c *Client }

// Create adds a webhook; the answer has the Secret for checking signatures.
func (s *WebhooksService) Create(ctx context.Context, params *CreateWebhookParams, options ...RequestOption) (*Webhook, error) {
	var out Webhook
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/webhooks", body: params}, &out, options)
}

// List returns every webhook, without secrets.
func (s *WebhooksService) List(ctx context.Context, options ...RequestOption) ([]Webhook, error) {
	var out List[Webhook]
	return out.Data, s.c.do(ctx, &call{method: http.MethodGet, path: "/webhooks"}, &out, options)
}

// Get returns a webhook with its secret.
func (s *WebhooksService) Get(ctx context.Context, id string, options ...RequestOption) (*Webhook, error) {
	var out Webhook
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/webhooks/" + escape(id)}, &out, options)
}

// Update changes a webhook.
func (s *WebhooksService) Update(ctx context.Context, id string, params *UpdateWebhookParams, options ...RequestOption) (*Webhook, error) {
	var out Webhook
	return &out, s.c.do(ctx, &call{method: http.MethodPatch, path: "/webhooks/" + escape(id), body: params}, &out, options)
}

// Delete deletes a webhook.
func (s *WebhooksService) Delete(ctx context.Context, id string, options ...RequestOption) (*Deleted, error) {
	var out Deleted
	return &out, s.c.do(ctx, &call{method: http.MethodDelete, path: "/webhooks/" + escape(id)}, &out, options)
}

// Deliveries returns a page of a webhook's pushes, newest first.
func (s *WebhooksService) Deliveries(ctx context.Context, id string, params *ListParams, options ...RequestOption) (*List[WebhookDelivery], error) {
	var out List[WebhookDelivery]
	path := "/webhooks/" + escape(id) + "/deliveries"
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: path, query: params.values()}, &out, options)
}

// Test queues a webhook.test push.
func (s *WebhooksService) Test(ctx context.Context, id string, options ...RequestOption) (*WebhookDelivery, error) {
	var out WebhookDelivery
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/webhooks/" + escape(id) + "/test"}, &out, options)
}

// SuppressionsService manages the addresses that are never sent to.
type SuppressionsService struct{ c *Client }

// ListSuppressionsParams: Q matches the address; Reason keeps one reason (hard_bounce, complaint or manual).
type ListSuppressionsParams struct {
	ListParams
	Q      string
	Reason string
}

func (p *ListSuppressionsParams) values() url.Values {
	if p == nil {
		return url.Values{}
	}
	v := p.ListParams.values()
	set(v, "q", p.Q)
	set(v, "reason", p.Reason)
	return v
}

// List returns one page, newest first.
func (s *SuppressionsService) List(ctx context.Context, params *ListSuppressionsParams, options ...RequestOption) (*List[Suppression], error) {
	var out List[Suppression]
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/suppressions", query: params.values()}, &out, options)
}

// All goes through the whole list.
func (s *SuppressionsService) All(ctx context.Context, params *ListSuppressionsParams, options ...RequestOption) iter.Seq2[Suppression, error] {
	return all(ctx, s.c, "/suppressions", params.values(), func(e Suppression) string { return e.ID }, options)
}

// Add puts an address on the list.
func (s *SuppressionsService) Add(ctx context.Context, address string, options ...RequestOption) (*Suppression, error) {
	var out Suppression
	body := map[string]string{"address": address}
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/suppressions", body: body}, &out, options)
}

// Remove takes an address off the list.
func (s *SuppressionsService) Remove(ctx context.Context, address string, options ...RequestOption) error {
	return s.c.do(ctx, &call{method: http.MethodDelete, path: "/suppressions/" + escape(address)}, nil, options)
}

// StatsService reads the sending statistics.
type StatsService struct{ c *Client }

func days(n int) url.Values {
	v := url.Values{}
	if n > 0 {
		v.Set("days", strconv.Itoa(n))
	}
	return v
}

// Get returns the last n UTC days (1–90; 0 means 14) with totals and rates.
func (s *StatsService) Get(ctx context.Context, n int, options ...RequestOption) (*Stats, error) {
	var out Stats
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/stats", query: days(n)}, &out, options)
}

// Providers returns the same period per mailbox provider (Gmail, QQ, Outlook…), busiest first.
func (s *StatsService) Providers(ctx context.Context, n int, options ...RequestOption) ([]ProviderStats, error) {
	var out List[ProviderStats]
	return out.Data, s.c.do(ctx, &call{method: http.MethodGet, path: "/stats/providers", query: days(n)}, &out, options)
}

// RecipientsService looks up one address.
type RecipientsService struct{ c *Client }

// Get tells why someone may not be getting your emails: the suppression list, bounces, the last emails to them.
func (s *RecipientsService) Get(ctx context.Context, address string, options ...RequestOption) (*Recipient, error) {
	var out Recipient
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/recipients/" + escape(address)}, &out, options)
}

// UsageService reads the workspace's quotas.
type UsageService struct{ c *Client }

// Get returns how much of the workspace's quotas is used.
func (s *UsageService) Get(ctx context.Context, options ...RequestOption) (*Usage, error) {
	var out Usage
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/usage"}, &out, options)
}

// LogsService reads your API requests of the last 7 days.
type LogsService struct{ c *Client }

// ListLogsParams: Status is "succeeded" or "failed".
type ListLogsParams struct {
	ListParams
	Status string
}

func (p *ListLogsParams) values() url.Values {
	if p == nil {
		return url.Values{}
	}
	v := p.ListParams.values()
	set(v, "status", p.Status)
	return v
}

// List returns one page, newest first, without bodies.
func (s *LogsService) List(ctx context.Context, params *ListLogsParams, options ...RequestOption) (*List[RequestLog], error) {
	var out List[RequestLog]
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/logs", query: params.values()}, &out, options)
}

// All goes through every request logged.
func (s *LogsService) All(ctx context.Context, params *ListLogsParams, options ...RequestOption) iter.Seq2[RequestLog, error] {
	return all(ctx, s.c, "/logs", params.values(), func(e RequestLog) string { return e.ID }, options)
}

// Get returns one request with its bodies.
func (s *LogsService) Get(ctx context.Context, id string, options ...RequestOption) (*RequestLog, error) {
	var out RequestLog
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/logs/" + escape(id)}, &out, options)
}
