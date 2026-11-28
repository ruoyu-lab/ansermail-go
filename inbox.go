package anser

import (
	"context"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// InboxService reads the emails sent to your domains (with receiving on) or to the workspace's test address.
type InboxService struct{ c *Client }

// ListReceivedParams filters received emails. To takes up to 20 addresses; StartDate and EndDate take a day
// (2026-11-01) or a time; Kind is message, reply, auto_reply or bounce; Status is received, blocked or
// over_quota. Domain is the domain of the receiving address; Read set to Ptr(false) lists unread emails only.
type ListReceivedParams struct {
	ListParams
	To               []string
	From             string
	StartDate        string
	EndDate          string
	Q                string
	Kind             string
	Status           string
	InReplyToEmailID string
	Domain           string
	Read             *bool
}

func (p *ListReceivedParams) values() url.Values {
	if p == nil {
		return url.Values{}
	}
	v := p.ListParams.values()
	set(v, "to", strings.Join(p.To, ","))
	set(v, "from", p.From)
	set(v, "start_date", p.StartDate)
	set(v, "end_date", p.EndDate)
	set(v, "q", p.Q)
	set(v, "kind", p.Kind)
	set(v, "status", p.Status)
	set(v, "in_reply_to_email_id", p.InReplyToEmailID)
	set(v, "domain", p.Domain)
	if p.Read != nil {
		v.Set("read", strconv.FormatBool(*p.Read))
	}
	return v
}

// List returns one page, newest first.
func (s *InboxService) List(ctx context.Context, params *ListReceivedParams, options ...RequestOption) (*List[ReceivedSummary], error) {
	var out List[ReceivedSummary]
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/inbox", query: params.values()}, &out, options)
}

// All goes through every received email matching.
func (s *InboxService) All(ctx context.Context, params *ListReceivedParams, options ...RequestOption) iter.Seq2[ReceivedSummary, error] {
	return all(ctx, s.c, "/inbox", params.values(), func(e ReceivedSummary) string { return e.ID }, options)
}

// Get returns a received email. With cidImages, inline images stay cid: references instead of data: URIs.
func (s *InboxService) Get(ctx context.Context, id string, cidImages bool, options ...RequestOption) (*ReceivedEmail, error) {
	var out ReceivedEmail
	query := url.Values{}
	if cidImages {
		query.Set("html_format", "cid")
	}
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/inbox/" + escape(id), query: query}, &out, options)
}

// SetRead marks a received email read (keeping the first time) or unread; shared by the whole workspace.
func (s *InboxService) SetRead(ctx context.Context, id string, read bool, options ...RequestOption) (*ReceivedSummary, error) {
	var out ReceivedSummary
	body := map[string]bool{"read": read}
	return &out, s.c.do(ctx, &call{method: http.MethodPatch, path: "/inbox/" + escape(id), body: body}, &out, options)
}

// Mark marks up to 100 received emails read or unread and returns those marked (ids not found are skipped).
func (s *InboxService) Mark(ctx context.Context, ids []string, read bool, options ...RequestOption) ([]ReceivedSummary, error) {
	var out List[ReceivedSummary]
	body := map[string]any{"ids": ids, "read": read}
	return out.Data, s.c.do(ctx, &call{method: http.MethodPost, path: "/inbox/mark", body: body}, &out, options)
}

// Delete deletes a received email with its attachments and raw message.
func (s *InboxService) Delete(ctx context.Context, id string, options ...RequestOption) (*Deleted, error) {
	var out Deleted
	return &out, s.c.do(ctx, &call{method: http.MethodDelete, path: "/inbox/" + escape(id)}, &out, options)
}

// DeleteMany deletes up to 100 received emails and returns those deleted (ids not found are skipped).
func (s *InboxService) DeleteMany(ctx context.Context, ids []string, options ...RequestOption) ([]Deleted, error) {
	var out List[Deleted]
	body := map[string][]string{"ids": ids}
	return out.Data, s.c.do(ctx, &call{method: http.MethodPost, path: "/inbox/delete", body: body}, &out, options)
}

// Summary counts the emails, and unread ones, of each receiving domain.
func (s *InboxService) Summary(ctx context.Context, options ...RequestOption) (*InboxSummary, error) {
	var out InboxSummary
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/inbox/summary"}, &out, options)
}

// Attachments lists a received email's attachments with download links, in the order of the email.
func (s *InboxService) Attachments(ctx context.Context, id string, params *ListParams, options ...RequestOption) (*List[ReceivedAttachment], error) {
	var out List[ReceivedAttachment]
	path := "/inbox/" + escape(id) + "/attachments"
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: path, query: params.values()}, &out, options)
}

// Attachment returns one attachment with a fresh download link.
func (s *InboxService) Attachment(ctx context.Context, id, attachmentID string, options ...RequestOption) (*ReceivedAttachment, error) {
	var out ReceivedAttachment
	path := "/inbox/" + escape(id) + "/attachments/" + escape(attachmentID)
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: path}, &out, options)
}

// Address returns the workspace's test address ("" when receiving is off); <name>+anything@… lands here too.
func (s *InboxService) Address(ctx context.Context, options ...RequestOption) (string, error) {
	var out struct {
		Address *string `json:"address"`
	}
	if err := s.c.do(ctx, &call{method: http.MethodGet, path: "/inbox/address"}, &out, options); err != nil {
		return "", err
	}
	if out.Address == nil {
		return "", nil
	}
	return *out.Address, nil
}

// Reply answers a received email in the same thread (In-Reply-To, References, Re:) and returns the email sent.
func (s *InboxService) Reply(ctx context.Context, id string, params *ReplyParams, options ...RequestOption) (*EmailSummary, error) {
	var out EmailSummary
	path := "/inbox/" + escape(id) + "/reply"
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: path, body: params}, &out, sending(options))
}

// Forward forwards a received email with its attachments and returns the email sent.
func (s *InboxService) Forward(ctx context.Context, id string, params *ForwardParams, options ...RequestOption) (*EmailSummary, error) {
	var out EmailSummary
	path := "/inbox/" + escape(id) + "/forward"
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: path, body: params}, &out, sending(options))
}

// ForwardingRulesService manages the rules that forward received emails to other mailboxes.
type ForwardingRulesService struct{ c *Client }

// List returns every rule.
func (s *ForwardingRulesService) List(ctx context.Context, options ...RequestOption) ([]ForwardingRule, error) {
	var out List[ForwardingRule]
	return out.Data, s.c.do(ctx, &call{method: http.MethodGet, path: "/forwarding-rules"}, &out, options)
}

// Create forwards address (support@acme.com, or *@acme.com for the whole domain) to forwardTo, which
// confirms by a link first.
func (s *ForwardingRulesService) Create(ctx context.Context, address, forwardTo string, options ...RequestOption) (*ForwardingRule, error) {
	var out ForwardingRule
	body := map[string]string{"address": address, "forward_to": forwardTo}
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/forwarding-rules", body: body}, &out, options)
}

// SetEnabled pauses or resumes a rule.
func (s *ForwardingRulesService) SetEnabled(ctx context.Context, id string, enabled bool, options ...RequestOption) (*ForwardingRule, error) {
	var out ForwardingRule
	body := map[string]bool{"enabled": enabled}
	return &out, s.c.do(ctx, &call{method: http.MethodPatch, path: "/forwarding-rules/" + escape(id), body: body}, &out, options)
}

// Delete deletes a rule.
func (s *ForwardingRulesService) Delete(ctx context.Context, id string, options ...RequestOption) (*Deleted, error) {
	var out Deleted
	return &out, s.c.do(ctx, &call{method: http.MethodDelete, path: "/forwarding-rules/" + escape(id)}, &out, options)
}

// ResendConfirmation sends the confirmation email again (the old link stops working).
func (s *ForwardingRulesService) ResendConfirmation(ctx context.Context, id string, options ...RequestOption) (*ForwardingRule, error) {
	var out ForwardingRule
	path := "/forwarding-rules/" + escape(id) + "/confirmation"
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: path}, &out, options)
}
