package anser

import (
	"encoding/json"
	"time"
)

// List is one page of a list, newest first.
type List[T any] struct {
	Object  string `json:"object"`
	HasMore bool   `json:"has_more"`
	Data    []T    `json:"data"`
}

// Deleted is the answer to a delete.
type Deleted struct {
	Object  string `json:"object"`
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// Address is an email address and, optionally, the name shown with it. Without a name it goes on the wire as
// a plain string, so Email can also be written "Amy <amy@example.com>".
type Address struct {
	Email string
	Name  string
}

// MarshalJSON writes "amy@example.com", or {"email": …, "name": …} when there is a name.
func (a Address) MarshalJSON() ([]byte, error) {
	if a.Name == "" {
		return json.Marshal(a.Email)
	}
	return json.Marshal(struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}{a.Email, a.Name})
}

// Tags are up to 50 names and values: letters, digits, _ and -, up to 256 each.
type Tags map[string]string

// ---------------------------------------------------------------- sending

// Attachment to send. Content is the raw bytes (base64 on the wire); set ContentID to show it inline with
// <img src="cid:…">.
type Attachment struct {
	Filename    string `json:"filename"`
	Content     []byte `json:"content"`
	ContentType string `json:"content_type,omitempty"`
	ContentID   string `json:"content_id,omitempty"`
}

// SendEmailParams is one email. From's domain must be verified; HTML or Text, at least one; To, Cc and Bcc
// up to 50 together; attachments up to 30 MB together.
//
// With Template set, the content comes from the template instead: leave HTML and Text empty; From, Subject and
// ReplyTo may stay empty too, and when set they win over the template's.
type SendEmailParams struct {
	From        Address           `json:"from"`
	To          []Address         `json:"to"`
	Subject     string            `json:"subject,omitempty"`
	HTML        string            `json:"html,omitempty"`
	Text        string            `json:"text,omitempty"`
	Template    *TemplateRef      `json:"template,omitempty"`
	Cc          []Address         `json:"cc,omitempty"`
	Bcc         []Address         `json:"bcc,omitempty"`
	ReplyTo     []Address         `json:"reply_to,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
	Tags        Tags              `json:"tags,omitempty"`
	// ScheduledAt sends it later (up to 30 days).
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

// MarshalJSON leaves out an empty From, so a template's sender is used.
func (p SendEmailParams) MarshalJSON() ([]byte, error) {
	type params SendEmailParams
	var from *Address
	if p.From != (Address{}) {
		from = &p.From
	}
	return json.Marshal(struct {
		params
		From *Address `json:"from,omitempty"`
	}{params(p), from})
}

// SendBatchParams is up to 100 emails. Atomic (true when nil): if one is refused, none is sent and SendBatch
// returns that error (its Param says which, "emails[3].from"); false: each is taken or refused on its own.
type SendBatchParams struct {
	Emails []SendEmailParams `json:"emails"`
	Atomic *bool             `json:"atomic,omitempty"`
}

// BatchError is why one email of a batch was refused; Index counts from 0.
type BatchError struct {
	Index   int     `json:"index"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Param   *string `json:"param"`
}

// BatchResult has one entry per email, in order, nil for one that was refused; Errors says why (always empty
// when atomic).
type BatchResult struct {
	Object string          `json:"object"`
	Data   []*EmailSummary `json:"data"`
	Errors []BatchError    `json:"errors"`
}

// EmailStatus is where an email is: scheduled, queued, sending, sent, delivered, delivery_delayed, bounced,
// complained, failed, canceled or suppressed.
type EmailStatus string

// EmailSummary is an email in a list.
type EmailSummary struct {
	Object      string      `json:"object"`
	ID          string      `json:"id"`
	From        string      `json:"from"`
	To          []string    `json:"to"`
	Subject     string      `json:"subject"`
	Status      EmailStatus `json:"status"`
	Tags        Tags        `json:"tags"`
	CreatedAt   time.Time   `json:"created_at"`
	ScheduledAt *time.Time  `json:"scheduled_at"`
	SentAt      *time.Time  `json:"sent_at"`
}

// EmailAttachment is an attachment of a sent email. Downloadable turns false once the email is sent,
// failed or canceled: only the name and size are kept.
type EmailAttachment struct {
	ID           string  `json:"id"`
	Filename     string  `json:"filename"`
	ContentType  string  `json:"content_type"`
	ContentID    *string `json:"content_id"`
	Size         int64   `json:"size"`
	Downloadable bool    `json:"downloadable"`
}

// Email is a sent email in full.
type Email struct {
	EmailSummary
	Cc          []string          `json:"cc"`
	Bcc         []string          `json:"bcc"`
	ReplyTo     []string          `json:"reply_to"`
	HTML        *string           `json:"html"`
	Text        *string           `json:"text"`
	Headers     map[string]string `json:"headers"`
	Attachments []EmailAttachment `json:"attachments"`
	// Template is the template and version it was sent with; nil when it had its own content or the template
	// was deleted.
	Template          *EmailTemplate `json:"template"`
	Attempts          int            `json:"attempts"`
	LastError         *string        `json:"last_error"`
	ProviderMessageID *string        `json:"provider_message_id"`
}

// EmailEvent is something that happened to an email.
type EmailEvent struct {
	Object     string         `json:"object"`
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Data       map[string]any `json:"data"`
	OccurredAt time.Time      `json:"occurred_at"`
}

// ---------------------------------------------------------------- the inbox

// ReceivedAttachmentMeta describes an attachment of a received email.
type ReceivedAttachmentMeta struct {
	ID                 string  `json:"id"`
	Filename           string  `json:"filename"`
	ContentType        string  `json:"content_type"`
	ContentDisposition string  `json:"content_disposition"`
	ContentID          *string `json:"content_id"`
	Size               int64   `json:"size"`
}

// ReceivedAttachment has a download link, valid for an hour.
type ReceivedAttachment struct {
	ReceivedAttachmentMeta
	Object      string    `json:"object"`
	DownloadURL string    `json:"download_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Authentication is what SPF, DKIM and DMARC said: pass, fail, gray, processing_failed or unknown.
type Authentication struct {
	SPF   string `json:"spf"`
	DKIM  string `json:"dkim"`
	DMARC string `json:"dmarc"`
}

// ReceivedSummary is a received email in a list.
type ReceivedSummary struct {
	Object      string                   `json:"object"`
	ID          string                   `json:"id"`
	To          []string                 `json:"to"`
	From        string                   `json:"from"`
	CreatedAt   time.Time                `json:"created_at"`
	Subject     string                   `json:"subject"`
	Bcc         []string                 `json:"bcc"`
	Cc          []string                 `json:"cc"`
	ReplyTo     []string                 `json:"reply_to"`
	MessageID   *string                  `json:"message_id"`
	Attachments []ReceivedAttachmentMeta `json:"attachments"`
	// Status: received, blocked (a virus) or over_quota.
	Status string `json:"status"`
	// Kind: message, reply, auto_reply or bounce.
	Kind string `json:"kind"`
	// ReceivedFor are the workspace's addresses that actually received it (a Bcc shows up only here).
	ReceivedFor      []string       `json:"received_for"`
	FromHeader       *string        `json:"from_header"`
	InReplyToEmailID *string        `json:"in_reply_to_email_id"`
	SpamVerdict      string         `json:"spam_verdict"`
	VirusVerdict     string         `json:"virus_verdict"`
	Authentication   Authentication `json:"authentication"`
	Preview          *string        `json:"preview"`
	Size             int64          `json:"size"`
	// ReadAt is when someone in the workspace read it; nil while unread. Shared by the whole workspace.
	ReadAt *time.Time `json:"read_at"`
}

// InboxDomain is one receiving domain on the inbox's front page.
type InboxDomain struct {
	Object string `json:"object"`
	Domain string `json:"domain"`
	// TestAddress is true for the domain of the workspace's test address.
	TestAddress bool `json:"test_address"`
	// Receiving is whether mail to it is still received.
	Receiving      bool       `json:"receiving"`
	Total          int        `json:"total"`
	Unread         int        `json:"unread"`
	LastReceivedAt *time.Time `json:"last_received_at"`
}

// InboxSummary counts the inbox by receiving domain; the totals count each email once.
type InboxSummary struct {
	Object         string        `json:"object"`
	Total          int           `json:"total"`
	Unread         int           `json:"unread"`
	LastReceivedAt *time.Time    `json:"last_received_at"`
	Domains        []InboxDomain `json:"domains"`
}

// Forward is a copy sent by a forwarding rule.
type Forward struct {
	To      string  `json:"to"`
	EmailID *string `json:"email_id"`
	Error   *string `json:"error"`
}

// ReceivedEmail is a received email in full.
type ReceivedEmail struct {
	ReceivedSummary
	HTML       *string           `json:"html"`
	HTMLFormat string            `json:"html_format"`
	Text       *string           `json:"text"`
	Headers    map[string]string `json:"headers"`
	Raw        *struct {
		DownloadURL string    `json:"download_url"`
		ExpiresAt   time.Time `json:"expires_at"`
	} `json:"raw"`
	// StrippedText is what the sender wrote, without the quoted email and the signature.
	StrippedText *string    `json:"stripped_text"`
	InReplyTo    *string    `json:"in_reply_to"`
	References   []string   `json:"references"`
	Forwards     []Forward  `json:"forwards"`
	SentAt       *time.Time `json:"sent_at"`
}

// ReplyParams answers a received email; every field is optional but HTML or Text. From defaults to the
// address that received it, To to the sender (or its Reply-To).
type ReplyParams struct {
	From     *Address  `json:"from,omitempty"`
	To       []Address `json:"to,omitempty"`
	Cc       []Address `json:"cc,omitempty"`
	Bcc      []Address `json:"bcc,omitempty"`
	ReplyAll bool      `json:"reply_all,omitempty"`
	Subject  string    `json:"subject,omitempty"`
	HTML     string    `json:"html,omitempty"`
	Text     string    `json:"text,omitempty"`
	Tags     Tags      `json:"tags,omitempty"`
}

// ForwardParams forwards a received email. Passthrough (the default when nil) forwards it as it is; false
// sends your HTML or Text with the original below.
type ForwardParams struct {
	From        Address   `json:"from"`
	To          []Address `json:"to"`
	Passthrough *bool     `json:"passthrough,omitempty"`
	HTML        string    `json:"html,omitempty"`
	Text        string    `json:"text,omitempty"`
	Tags        Tags      `json:"tags,omitempty"`
}

// ForwardingRule forwards received emails to another mailbox.
type ForwardingRule struct {
	Object   string `json:"object"`
	ID       string `json:"id"`
	Address  string `json:"address"`
	DomainID string `json:"domain_id"`
	// ForwardTo has to confirm by a link first.
	ForwardTo string `json:"forward_to"`
	Enabled   bool   `json:"enabled"`
	// Status: active, pending_confirmation or confirmation_expired.
	Status             string     `json:"status"`
	ConfirmedAt        *time.Time `json:"confirmed_at"`
	ConfirmationSentAt time.Time  `json:"confirmation_sent_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

// ---------------------------------------------------------------- domains, keys, webhooks, suppressions

// DNSRecord is a record to add at your DNS host, with what the service sees there now. Purpose is ownership (a TXT at
// _anser.<domain> proving the domain belongs to this workspace), dkim, mx or spf (the bounce subdomain), dmarc, or
// receiving (the MX record that brings the domain's email in);
// Conflicts, for receiving only, are other MX servers on the domain that may get the email first.
type DNSRecord struct {
	Purpose   string   `json:"purpose"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Value     string   `json:"value"`
	Priority  *int     `json:"priority"`
	Required  bool     `json:"required"`
	Status    string   `json:"status"`
	Conflicts []string `json:"conflicts,omitempty"`
}

// Domain is a sending (and maybe receiving) domain. Region is the server's SES region, the same for every domain.
type Domain struct {
	Object        string      `json:"object"`
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Region        string      `json:"region"`
	Status        string      `json:"status"`
	CreatedAt     time.Time   `json:"created_at"`
	CheckedAt     *time.Time  `json:"checked_at"`
	VerifiedAt    *time.Time  `json:"verified_at"`
	OpenTracking  bool        `json:"open_tracking"`
	ClickTracking bool        `json:"click_tracking"`
	Receiving     bool        `json:"receiving"`
	Records       []DNSRecord `json:"records"`
	// DNS is where the domain's DNS is hosted, from its NS records; nil until found.
	DNS *DNSHost `json:"dns"`
}

// DNSHost is where a domain's DNS is hosted. Zone is where the records go (acme.com for mail.acme.com); Provider is
// cloudflare, aliyun, dnspod, godaddy, namecheap or route53, or "" when not recognized.
type DNSHost struct {
	Zone     string `json:"zone"`
	Provider string `json:"provider"`
}

// DNSSetupResult is what happened to one record. Outcome is created; exists (already there, left alone); updated
// (already there but proxied, now DNS only); conflict (another value at that name, left alone: see Existing); or
// failed (Cloudflare refused: see Message).
type DNSSetupResult struct {
	Purpose  string   `json:"purpose"`
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Outcome  string   `json:"outcome"`
	Existing []string `json:"existing,omitempty"`
	Message  string   `json:"message,omitempty"`
}

// DNSSetup is the answer of DomainsService.AddToCloudflare: the Cloudflare zone the records went into, each record,
// and the domain checked again.
type DNSSetup struct {
	Object   string           `json:"object"`
	Provider string           `json:"provider"`
	Zone     string           `json:"zone"`
	Results  []DNSSetupResult `json:"results"`
	Domain   Domain           `json:"domain"`
}

// UpdateDomainParams changes only what is set.
type UpdateDomainParams struct {
	OpenTracking  *bool `json:"open_tracking,omitempty"`
	ClickTracking *bool `json:"click_tracking,omitempty"`
	Receiving     *bool `json:"receiving,omitempty"`
}

// APIKey is a key without its token. Permission is full (everything) or send (sending only, from DomainID's
// domain when set).
type APIKey struct {
	Object      string     `json:"object"`
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Permission  string     `json:"permission"`
	DomainID    *string    `json:"domain_id"`
	TokenPrefix string     `json:"token_prefix"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

// CreateAPIKeyParams: Permission is full (the default) or send; DomainID limits a send key to that domain.
type CreateAPIKeyParams struct {
	Name       string `json:"name"`
	Permission string `json:"permission,omitempty"`
	DomainID   string `json:"domain_id,omitempty"`
}

// CreatedAPIKey is the new key with its token, shown only this once.
type CreatedAPIKey struct {
	APIKey
	Token string `json:"token"`
}

// Webhook pushes email events to your endpoint. Secret (whsec_…) comes with create and get, not list.
type Webhook struct {
	Object      string    `json:"object"`
	ID          string    `json:"id"`
	Endpoint    string    `json:"endpoint"`
	Events      []string  `json:"events"`
	Description *string   `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	Secret      string    `json:"secret,omitempty"`
}

// CreateWebhookParams: Endpoint must be a public https URL.
type CreateWebhookParams struct {
	Endpoint    string   `json:"endpoint"`
	Events      []string `json:"events"`
	Description string   `json:"description,omitempty"`
}

// UpdateWebhookParams changes only what is set.
type UpdateWebhookParams struct {
	Endpoint    *string  `json:"endpoint,omitempty"`
	Events      []string `json:"events,omitempty"`
	Description *string  `json:"description,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

// WebhookDelivery is one push and its attempts.
type WebhookDelivery struct {
	Object         string          `json:"object"`
	ID             string          `json:"id"`
	EventType      string          `json:"event_type"`
	Status         string          `json:"status"`
	Attempts       int             `json:"attempts"`
	ResponseStatus *int            `json:"response_status"`
	LastError      *string         `json:"last_error"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      time.Time       `json:"created_at"`
	DeliveredAt    *time.Time      `json:"delivered_at"`
	NextAttemptAt  *time.Time      `json:"next_attempt_at"`
}

// EventEmail is the sent email an event is about.
type EventEmail struct {
	Object    string    `json:"object"`
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        []string  `json:"to"`
	Subject   string    `json:"subject"`
	Tags      Tags      `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

// EmailEventData is what an email event push carries: the email, and in Details everything the push carried,
// with what depends on the event: bounce, complaint, delay, reason, the clicked link…
type EmailEventData struct {
	Email   EventEmail     `json:"email"`
	Details map[string]any `json:"-"`
}

// UnmarshalJSON fills the fields and Details.
func (d *EmailEventData) UnmarshalJSON(b []byte) error {
	type fields EmailEventData
	if err := json.Unmarshal(b, (*fields)(d)); err != nil {
		return err
	}
	return json.Unmarshal(b, &d.Details)
}

// ReceivedEventData is what an email.received push carries: the email as the inbox lists it; fetch it for
// the body and headers.
type ReceivedEventData struct {
	Email ReceivedSummary `json:"email"`
}

// Suppression is an address that is never sent to. Reason: hard_bounce, complaint or manual.
type Suppression struct {
	Object    string    `json:"object"`
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	Reason    string    `json:"reason"`
	EmailID   *string   `json:"email_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ---------------------------------------------------------------- statistics, usage, logs

// Counts of a day or a period.
type Counts struct {
	Sent       int `json:"sent"`
	Delivered  int `json:"delivered"`
	Bounced    int `json:"bounced"`
	Complained int `json:"complained"`
	Failed     int `json:"failed"`
	Opened     int `json:"opened"`
	Clicked    int `json:"clicked"`
}

// Stats per UTC day; rates are 0–1 of Sent, nil with nothing sent.
type Stats struct {
	Object string `json:"object"`
	Days   []struct {
		Date string `json:"date"`
		Counts
	} `json:"days"`
	Totals Counts              `json:"totals"`
	Rates  map[string]*float64 `json:"rates"`
}

// ProviderStats is a period's sending to one mailbox provider (gmail, qq, outlook…; other when unknown).
type ProviderStats struct {
	Object        string              `json:"object"`
	Provider      string              `json:"provider"`
	Name          string              `json:"name"`
	Sent          int                 `json:"sent"`
	Delivered     int                 `json:"delivered"`
	Delayed       int                 `json:"delayed"`
	Bounced       int                 `json:"bounced"`
	Complained    int                 `json:"complained"`
	Rates         map[string]*float64 `json:"rates"`
	DeliveryTimes []struct {
		UpToMs *int `json:"up_to_ms"`
		Count  int  `json:"count"`
	} `json:"delivery_times"`
}

// Recipient is everything about one address in this workspace: why they may not be getting your emails.
type Recipient struct {
	Object           string           `json:"object"`
	Address          string           `json:"address"`
	Domain           string           `json:"domain"`
	Provider         *string          `json:"provider"`
	ProviderName     *string          `json:"provider_name"`
	AcceptsMail      *bool            `json:"accepts_mail"`
	DidYouMean       *string          `json:"did_you_mean"`
	Suppression      map[string]any   `json:"suppression"`
	RecentlyBounced  bool             `json:"recently_bounced"`
	Emails           []map[string]any `json:"emails"`
	RejectedRequests []map[string]any `json:"rejected_requests"`
}

// Quota used and its limit (nil: no limit).
type Quota struct {
	Used     int64     `json:"used"`
	Limit    *int64    `json:"limit"`
	ResetsAt time.Time `json:"resets_at"`
}

// ReceivingQuota also counts what was turned away.
type ReceivingQuota struct {
	Quota
	Blocked   int64 `json:"blocked"`
	OverQuota int64 `json:"over_quota"`
}

// Usage is how much of the workspace's quotas is used. Attachments are bytes this month. RampUntil is until when a new
// workspace has a lower daily quota (already in Today.Limit); nil when not.
type Usage struct {
	Object  string `json:"object"`
	Today   Quota  `json:"today"`
	Month   Quota  `json:"month"`
	Domains struct {
		Used  int64  `json:"used"`
		Limit *int64 `json:"limit"`
	} `json:"domains"`
	Attachments Quota `json:"attachments"`
	Receiving   struct {
		Today ReceivingQuota `json:"today"`
		Month ReceivingQuota `json:"month"`
	} `json:"receiving"`
	RampUntil        *time.Time `json:"ramp_until"`
	RetentionDays    *int       `json:"retention_days"`
	MaxRetentionDays *int       `json:"max_retention_days"`
	PausedAt         *time.Time `json:"paused_at"`
	PauseReason      *string    `json:"pause_reason"`
}

// RequestLog is one API request; the bodies come only with LogsService.Get.
type RequestLog struct {
	Object     string `json:"object"`
	ID         string `json:"id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	DurationMs int    `json:"duration_ms"`
	APIKey     *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"api_key"`
	UserAgent    *string   `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
	RequestBody  *string   `json:"request_body"`
	ResponseBody *string   `json:"response_body"`
}
