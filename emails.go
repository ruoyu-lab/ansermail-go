package anser

import (
	"context"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ListParams pages through a list: After fetches older items than that id, Before newer ones.
type ListParams struct {
	// Limit is 1–100; 0 means the default, 20.
	Limit  int
	After  string
	Before string
}

func (p *ListParams) values() url.Values {
	v := url.Values{}
	if p == nil {
		return v
	}
	if p.Limit > 0 {
		v.Set("limit", strconv.Itoa(p.Limit))
	}
	set(v, "after", p.After)
	set(v, "before", p.Before)
	return v
}

func set(v url.Values, name, value string) {
	if value != "" {
		v.Set(name, value)
	}
}

// all walks every page of a list, one item at a time.
func all[T any](ctx context.Context, c *Client, path string, query url.Values, id func(T) string,
	options []RequestOption) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for {
			var page List[T]
			if err := c.do(ctx, &call{method: http.MethodGet, path: path, query: query}, &page, options); err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range page.Data {
				if !yield(item, nil) {
					return
				}
			}
			if !page.HasMore || len(page.Data) == 0 {
				return
			}
			query.Set("after", id(page.Data[len(page.Data)-1]))
		}
	}
}

// sending makes up an idempotency key when there is none, so a retry never sends twice.
func sending(options []RequestOption) []RequestOption {
	return append([]RequestOption{WithIdempotencyKey(newIdempotencyKey())}, options...)
}

// EmailsService sends emails and looks them up.
type EmailsService struct{ c *Client }

// Send queues an email; it leaves within seconds, or at ScheduledAt.
func (s *EmailsService) Send(ctx context.Context, params *SendEmailParams, options ...RequestOption) (*EmailSummary, error) {
	var out EmailSummary
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/emails", body: params}, &out, sending(options))
}

// SendBatch queues up to 100 emails: all or none, or with Atomic set to false each taken or refused on its own.
func (s *EmailsService) SendBatch(ctx context.Context, params *SendBatchParams, options ...RequestOption) (*BatchResult, error) {
	var out BatchResult
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/emails/batch", body: params}, &out, sending(options))
}

// Get returns an email in full.
func (s *EmailsService) Get(ctx context.Context, id string, options ...RequestOption) (*Email, error) {
	var out Email
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/emails/" + escape(id)}, &out, options)
}

// ListEmailsParams filters emails: Status is an EmailStatus; Q matches recipients, the sender and the subject.
type ListEmailsParams struct {
	ListParams
	Status EmailStatus
	Q      string
}

func (p *ListEmailsParams) values() url.Values {
	if p == nil {
		return url.Values{}
	}
	v := p.ListParams.values()
	set(v, "status", string(p.Status))
	set(v, "q", p.Q)
	return v
}

// List returns one page, newest first.
func (s *EmailsService) List(ctx context.Context, params *ListEmailsParams, options ...RequestOption) (*List[EmailSummary], error) {
	var out List[EmailSummary]
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/emails", query: params.values()}, &out, options)
}

// All goes through every email matching, page after page.
func (s *EmailsService) All(ctx context.Context, params *ListEmailsParams, options ...RequestOption) iter.Seq2[EmailSummary, error] {
	return all(ctx, s.c, "/emails", params.values(), func(e EmailSummary) string { return e.ID }, options)
}

// Events returns what happened to an email, oldest first.
func (s *EmailsService) Events(ctx context.Context, id string, options ...RequestOption) ([]EmailEvent, error) {
	var out List[EmailEvent]
	err := s.c.do(ctx, &call{method: http.MethodGet, path: "/emails/" + escape(id) + "/events"}, &out, options)
	return out.Data, err
}

// Attachment returns an attachment's bytes, only while the email has not left yet.
func (s *EmailsService) Attachment(ctx context.Context, id, attachmentID string, options ...RequestOption) ([]byte, error) {
	path := "/emails/" + escape(id) + "/attachments/" + escape(attachmentID)
	return s.c.send(ctx, &call{method: http.MethodGet, path: path}, options)
}

// Update moves a scheduled email.
func (s *EmailsService) Update(ctx context.Context, id string, scheduledAt time.Time, options ...RequestOption) (*EmailSummary, error) {
	var out EmailSummary
	body := map[string]time.Time{"scheduled_at": scheduledAt}
	return &out, s.c.do(ctx, &call{method: http.MethodPatch, path: "/emails/" + escape(id), body: body}, &out, options)
}

// Cancel cancels a scheduled email.
func (s *EmailsService) Cancel(ctx context.Context, id string, options ...RequestOption) (*EmailSummary, error) {
	var out EmailSummary
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/emails/" + escape(id) + "/cancel"}, &out, options)
}
