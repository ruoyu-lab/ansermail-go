package ansermail

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// The live tests run against a server when ANSER_LIVE_BASE_URL, ANSER_LIVE_API_KEY and ANSER_LIVE_FROM are set.
// ANSER_LIVE_FROM is an address on a verified domain of that workspace. Mail goes to the SES simulator, which
// does not count towards the quota.
const liveTo = "success@simulator.amazonses.com"

func live(t *testing.T) (*Client, string) {
	t.Helper()
	base, key, from := os.Getenv("ANSER_LIVE_BASE_URL"), os.Getenv("ANSER_LIVE_API_KEY"), os.Getenv("ANSER_LIVE_FROM")
	if base == "" || key == "" || from == "" {
		t.Skip("set ANSER_LIVE_BASE_URL, ANSER_LIVE_API_KEY and ANSER_LIVE_FROM")
	}
	return NewClient(key, WithBaseURL(base)), from
}

// must stops the test (a panic fails it) when a call errs.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func TestLiveSchedulesReadsMovesAndCancels(t *testing.T) {
	client, from := live(t)
	ctx := context.Background()
	at := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()
	params := &SendEmailParams{
		From:        Address{Email: from},
		To:          []Address{{Email: liveTo}},
		Subject:     "SDK 联调 👋",
		Text:        "go",
		Attachments: []Attachment{{Filename: "hi.txt", Content: []byte("你好")}},
		Tags:        Tags{"sdk": "go"},
		ScheduledAt: &at,
	}
	key := WithIdempotencyKey(fmt.Sprintf("live-%d", time.Now().UnixNano()))
	sent := must(client.Emails.Send(ctx, params, key))
	// the same key answers with the same email
	if again := must(client.Emails.Send(ctx, params, key)); again.ID != sent.ID {
		t.Fatalf("same key, new email: %s vs %s", again.ID, sent.ID)
	}

	email := must(client.Emails.Get(ctx, sent.ID))
	if email.Status != "scheduled" || !slices.Equal(email.To, []string{liveTo}) || email.Subject != params.Subject {
		t.Fatalf("got %+v", email.EmailSummary)
	}
	if !email.ScheduledAt.Equal(at) || len(email.Tags) != 1 || email.Tags["sdk"] != "go" {
		t.Fatalf("got %v %v", email.ScheduledAt, email.Tags)
	}
	if bytes := must(client.Emails.Attachment(ctx, sent.ID, email.Attachments[0].ID)); string(bytes) != "你好" {
		t.Fatalf("attachment %q", bytes)
	}

	if moved := must(client.Emails.Update(ctx, sent.ID, at.Add(time.Hour))); !moved.ScheduledAt.Equal(at.Add(time.Hour)) {
		t.Fatalf("moved to %v", moved.ScheduledAt)
	}
	if canceled := must(client.Emails.Cancel(ctx, sent.ID)); canceled.Status != "canceled" {
		t.Fatalf("status %s", canceled.Status)
	}
	var types []string
	for _, event := range must(client.Emails.Events(ctx, sent.ID)) {
		types = append(types, event.Type)
	}
	if strings.Join(types, " ") != "email.scheduled email.scheduled email.canceled" {
		t.Fatalf("events %v", types)
	}
	page := must(client.Emails.List(ctx, &ListEmailsParams{ListParams: ListParams{Limit: 5}}))
	if !slices.ContainsFunc(page.Data, func(e EmailSummary) bool { return e.ID == sent.ID }) {
		t.Fatal("not in the list")
	}
}

func TestLiveSendsABatch(t *testing.T) {
	client, from := live(t)
	email := func(subject, to string) SendEmailParams {
		return SendEmailParams{From: Address{Email: from}, To: []Address{{Email: to}}, Subject: subject, Text: "go"}
	}
	batch := must(client.Emails.SendBatch(context.Background(), &SendBatchParams{
		Emails: []SendEmailParams{email("SDK 联调 1", liveTo), email("SDK 联调 2", liveTo)},
	}))
	if len(batch.Data) != 2 || batch.Data[1].Subject != "SDK 联调 2" {
		t.Fatalf("got %+v", batch)
	}

	atomic := false
	partial := must(client.Emails.SendBatch(context.Background(), &SendBatchParams{
		Emails: []SendEmailParams{email("SDK 联调 3", liveTo), email("SDK 联调 4", "nope")},
		Atomic: &atomic,
	}))
	if partial.Data[0] == nil || partial.Data[1] != nil || *partial.Errors[0].Param != "emails[1].to[0]" {
		t.Fatalf("got %+v", partial)
	}
}

func TestLiveTheRestOfTheWorkspace(t *testing.T) {
	client, from := live(t)
	ctx := context.Background()
	domain := strings.TrimSuffix(from[strings.LastIndex(from, "@")+1:], ">")
	domains := must(client.Domains.List(ctx))
	if !slices.ContainsFunc(domains, func(d Domain) bool { return d.Name == domain }) {
		t.Fatalf("%s is not a domain of this workspace", domain)
	}
	if usage := must(client.Usage.Get(ctx)); usage.Object != "usage" {
		t.Fatalf("usage %+v", usage)
	}
	if stats := must(client.Stats.Get(ctx, 7)); len(stats.Days) != 7 {
		t.Fatalf("%d days", len(stats.Days))
	}
	address := fmt.Sprintf("sdk-live-%d@example.com", time.Now().UnixNano())
	if added := must(client.Suppressions.Add(ctx, address)); added.Address != address {
		t.Fatalf("added %s", added.Address)
	}
	if err := client.Suppressions.Remove(ctx, address); err != nil {
		t.Fatal(err)
	}
	if left := must(client.Suppressions.List(ctx, &ListSuppressionsParams{Q: address})); len(left.Data) != 0 {
		t.Fatal("still suppressed")
	}
	_, err := client.Emails.Get(ctx, "01938a1c-0000-7000-8000-000000000000")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 || apiErr.Code != "not_found" {
		t.Fatalf("got %v", err)
	}
}
