# ansermail-go

The Go SDK for [Anser](https://mail.ruoyulab.dev): send and receive email through its HTTP API. Go 1.23+, the standard
library only.

## Install

```bash
go get github.com/ruoyu-lab/ansermail-go
```

## Sending

```go
import "github.com/ruoyu-lab/ansermail-go"

client := ansermail.NewClient("ms_…") // or NewClient("") with ANSER_API_KEY set

at := time.Now().Add(time.Hour)
sent, err := client.Emails.Send(ctx, &ansermail.SendEmailParams{
	From:        ansermail.Address{Email: "hello@acme.com", Name: "Acme"},
	To:          []ansermail.Address{{Email: "amy@example.com", Name: "Amy"}},
	Subject:     "Welcome",
	HTML:        "<p>Hi Amy</p>",
	Attachments: []ansermail.Attachment{{Filename: "invoice.pdf", Content: pdf}}, // raw bytes
	Tags:        ansermail.Tags{"category": "welcome"},
	ScheduledAt: &at, // optional, up to 30 days
})
// sent has the ID, status, recipients and tags

email, err := client.Emails.Get(ctx, sent.ID) // with the body, attachments and delivery details
_, err = client.Emails.Update(ctx, sent.ID, at.Add(time.Hour))
_, err = client.Emails.Cancel(ctx, sent.ID)

// up to 100; all or none, unless Atomic is false
atomic := false
batch, err := client.Emails.SendBatch(ctx, &ansermail.SendBatchParams{
	Emails: []ansermail.SendEmailParams{
		{From: ansermail.Address{Email: "hello@acme.com"}, To: []ansermail.Address{{Email: "amy@example.com"}}, Subject: "Hi", Text: "Hi Amy"},
		{From: ansermail.Address{Email: "hello@acme.com"}, To: []ansermail.Address{{Email: "bob@example.com"}}, Subject: "Hi", Text: "Hi Bob"},
	},
	Atomic: &atomic,
})
// batch.Data: one per email, nil for a refused one; batch.Errors: why
```

Every send, batch, reply and forward carries an idempotency key, so a retry never sends twice. Pass your own to make a
repeated call (a job that runs again) safe too: `client.Emails.Send(ctx, params, ansermail.WithIdempotencyKey("welcome-"+userID))`.

## Lists

```go
page, err := client.Emails.List(ctx, &ansermail.ListEmailsParams{ListParams: ansermail.ListParams{Limit: 50}, Status: "bounced"})

for email, err := range client.Emails.All(ctx, &ansermail.ListEmailsParams{Status: "bounced"}) {
	if err != nil {
		return err
	}
	fmt.Println(email.To)
}
```

## The inbox

```go
address, err := client.Inbox.Address(ctx) // the workspace's test address

for received, err := range client.Inbox.All(ctx, &ansermail.ListReceivedParams{To: []string{"support@acme.com"}}) {
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
	_, err = client.Inbox.Reply(ctx, received.ID, &ansermail.ReplyParams{Text: "Thanks, we are on it."})
}

_, err = client.ForwardingRules.Create(ctx, "support@acme.com", "me@example.com")
```

## Webhooks

```go
http.HandleFunc("POST /anser", func(w http.ResponseWriter, r *http.Request) {
	payload, _ := io.ReadAll(r.Body) // the body exactly as it arrived
	event, err := ansermail.VerifyWebhook(payload, r.Header, os.Getenv("ANSER_WEBHOOK_SECRET"))
	if err != nil { // errors.Is(err, ansermail.ErrWebhookVerification)
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
var apiErr *ansermail.Error
var connErr *ansermail.ConnectionError
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
client := ansermail.NewClient("ms_…",
	ansermail.WithBaseURL("https://mail.ruoyulab.dev"), // default: ANSER_BASE_URL, then the hosted service
	ansermail.WithMaxRetries(2), // rate limits always; server errors and lost connections only when safe to repeat
	ansermail.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}), // its Timeout is per attempt
	ansermail.WithHeader("X-Team", "growth"), // sent with every request
)
```

A call stops when its `ctx` is done, retries and their waits included. The other services follow the
[API](https://mail.ruoyulab.dev/docs/api) (reference in Chinese): `Domains`, `APIKeys`, `Webhooks`, `Suppressions`,
`Stats`, `Recipients`, `Usage` and `Logs`.

## License

[MIT](LICENSE)
