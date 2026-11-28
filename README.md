# anser-mail-go

The Go SDK for [Anser Mail](https://mail.ruoyulab.dev): send and receive email through its HTTP API. Go 1.23+, the standard
library only.

## Install

```bash
go get github.com/ruoyu-lab/anser-mail-go
```

## Sending

```go
import "github.com/ruoyu-lab/anser-mail-go"

client := anser.NewClient("ms_…") // or NewClient("") with ANSER_API_KEY set

at := time.Now().Add(time.Hour)
sent, err := client.Emails.Send(ctx, &anser.SendEmailParams{
	From:        anser.Address{Email: "hello@acme.com", Name: "Acme"},
	To:          []anser.Address{{Email: "amy@example.com", Name: "Amy"}},
	Subject:     "Welcome",
	HTML:        "<p>Hi Amy</p>",
	Attachments: []anser.Attachment{{Filename: "invoice.pdf", Content: pdf}}, // raw bytes
	Tags:        anser.Tags{"category": "welcome"},
	ScheduledAt: &at, // optional, up to 30 days
})
// sent has the ID, status, recipients and tags

email, err := client.Emails.Get(ctx, sent.ID) // with the body, attachments and delivery details
_, err = client.Emails.Update(ctx, sent.ID, at.Add(time.Hour))
_, err = client.Emails.Cancel(ctx, sent.ID)

// up to 100; all or none, unless Atomic is false
atomic := false
batch, err := client.Emails.SendBatch(ctx, &anser.SendBatchParams{
	Emails: []anser.SendEmailParams{
		{From: anser.Address{Email: "hello@acme.com"}, To: []anser.Address{{Email: "amy@example.com"}}, Subject: "Hi", Text: "Hi Amy"},
		{From: anser.Address{Email: "hello@acme.com"}, To: []anser.Address{{Email: "bob@example.com"}}, Subject: "Hi", Text: "Hi Bob"},
	},
	Atomic: &atomic,
})
// batch.Data: one per email, nil for a refused one; batch.Errors: why
```

Every send, batch, reply and forward carries an idempotency key, so a retry never sends twice. Pass your own to make a
repeated call (a job that runs again) safe too: `client.Emails.Send(ctx, params, anser.WithIdempotencyKey("welcome-"+userID))`.

## Templates

Templates are Handlebars (`{{name}}`, `{{#if}}`, `{{#each}}`, helpers like `formatDate`). Edits change the draft; publishing
saves it as the next version, and emails always use the latest published version. A variable the template prints but the
call does not give refuses the email (`template_variable_missing`).

```go
_, err = client.Templates.Create(ctx, &anser.CreateTemplateParams{
	Name:    "Welcome",
	Alias:   "welcome", // send and look it up by this instead of the ID
	Subject: "Welcome, {{name}}",
	HTML:    "<p>Hi {{name}}</p>{{#if coupon}}<p>Your code: {{coupon}}</p>{{/if}}",
	From:    &anser.Address{Email: "hello@acme.com", Name: "Acme"}, // used when the email gives none
})
_, err = client.Templates.Publish(ctx, "welcome") // version 1

// Variables: a map, or a struct with json tags. From, Subject and ReplyTo set here win over the template's.
_, err = client.Emails.Send(ctx, &anser.SendEmailParams{
	To:       []anser.Address{{Email: "amy@example.com"}},
	Template: &anser.TemplateRef{ID: "welcome", Variables: map[string]any{"name": "Amy", "coupon": "HELLO10"}},
})

// try a change out without storing it; every field's problems come back together in Errors
result, err := client.Templates.Render(ctx, &anser.RenderTemplateParams{HTML: "<p>{{name}}</p>", Variables: map[string]any{"name": "Amy"}})

_, err = client.Templates.Update(ctx, "welcome", &anser.UpdateTemplateParams{Subject: anser.Ptr("Hello, {{name}}")}) // the draft only
versions, err := client.Templates.Versions(ctx, "welcome")
_, err = client.Templates.Restore(ctx, "welcome", 1) // back into the draft; publish to send it again
```

## Lists

```go
page, err := client.Emails.List(ctx, &anser.ListEmailsParams{ListParams: anser.ListParams{Limit: 50}, Status: "bounced"})

for email, err := range client.Emails.All(ctx, &anser.ListEmailsParams{Status: "bounced"}) {
	if err != nil {
		return err
	}
	fmt.Println(email.To)
}
```

## The inbox

```go
address, err := client.Inbox.Address(ctx) // the workspace's test address

unread := &anser.ListReceivedParams{To: []string{"support@acme.com"}, Read: anser.Ptr(false)}
for received, err := range client.Inbox.All(ctx, unread) {
	if err != nil {
		return err
	}
	email, err := client.Inbox.Get(ctx, received.ID, false)
	if err != nil {
		return err
	}
	if email.StrippedText != nil {
		fmt.Println(*email.StrippedText) // what the sender wrote, without the quoted history
	}
	_, err = client.Inbox.Reply(ctx, received.ID, &anser.ReplyParams{Text: "Thanks, we are on it."})
	_, err = client.Inbox.SetRead(ctx, received.ID, true) // read state is shared by the workspace
}

summary, err := client.Inbox.Summary(ctx) // unread counts, per receiving domain too
_, err = client.Inbox.Mark(ctx, ids, true) // or DeleteMany(ctx, ids), up to 100 at a time

_, err = client.ForwardingRules.Create(ctx, "support@acme.com", "me@example.com")
```

## Webhooks

```go
http.HandleFunc("POST /anser", func(w http.ResponseWriter, r *http.Request) {
	payload, _ := io.ReadAll(r.Body) // the body exactly as it arrived
	event, err := anser.VerifyWebhook(payload, r.Header, os.Getenv("ANSER_WEBHOOK_SECRET"))
	if err != nil { // errors.Is(err, anser.ErrWebhookVerification)
		http.Error(w, "bad signature", http.StatusBadRequest)
		return
	}
	if event.Type == "email.bounced" {
		data, _ := event.EmailData() // email.received: event.ReceivedData()
		_ = data.Email.ID             // the email, with its tags
		_ = data.Details["bounce"]
	}
	w.WriteHeader(http.StatusNoContent)
})
```

## Errors

```go
_, err := client.Emails.Send(ctx, params)
var apiErr *anser.Error
var connErr *anser.ConnectionError
switch {
case errors.As(err, &apiErr):
	_ = apiErr.StatusCode // 422
	_ = apiErr.Code       // "validation_error", "domain_not_verified", "daily_quota_exceeded" …
	_ = apiErr.Param      // the field it is about, e.g. "to[1]", or ""
	_ = apiErr.RequestID  // look it up in the request logs
case errors.As(err, &connErr):
	// no answer: a timeout or the network
}
```

## Options

```go
client := anser.NewClient("ms_…",
	anser.WithBaseURL("https://mail.ruoyulab.dev"), // default: ANSER_BASE_URL, then the hosted service
	anser.WithMaxRetries(2), // rate limits always; server errors and lost connections only when safe to repeat
	anser.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}), // its Timeout is per attempt
	anser.WithHeader("X-Team", "growth"), // sent with every request
)
```

A call stops when its `ctx` is done, retries and their waits included. The other services follow the
[API](https://mail.ruoyulab.dev/docs/api) (reference in Chinese): `Domains`, `APIKeys`, `Webhooks`, `Suppressions`,
`Stats`, `Recipients`, `Usage` and `Logs`.

## License

[MIT](LICENSE)
