package ansermail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// uuidKey is the idempotency key the SDK makes up: anser-<a version 4 UUID>.
var uuidKey = regexp.MustCompile(`^anser-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type recorded struct {
	Method string
	Path   string
	Header http.Header
	Body   map[string]any
}

// stub answers what answer says and records every request.
type stub struct {
	mu       sync.Mutex
	requests []recorded
	answer   func(r recorded, n int) (int, http.Header, string)
	server   *httptest.Server
}

func newStub(t *testing.T) *stub {
	s := &stub{answer: func(recorded, int) (int, http.Header, string) { return 201, nil, `{"object":"email","id":"e1"}` }}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec := recorded{Method: r.Method, Path: r.URL.RequestURI(), Header: r.Header}
		_ = json.Unmarshal(raw, &rec.Body)
		s.mu.Lock()
		s.requests = append(s.requests, rec)
		n := len(s.requests)
		s.mu.Unlock()
		status, header, body := s.answer(rec, n)
		for name, values := range header {
			w.Header()[name] = values
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func apiError(code string) string {
	return fmt.Sprintf(`{"error":{"code":%q,"message":"%s happened","param":null}}`, code, code)
}

func TestSendsWithAKeyBytesAndATime(t *testing.T) {
	s := newStub(t)
	client := NewClient("ms_test", WithBaseURL(s.server.URL+"/"))
	at := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	sent, err := client.Emails.Send(context.Background(), &SendEmailParams{
		From:        Address{Email: "hello@acme.com", Name: "Acme"},
		To:          []Address{{Email: "amy@example.com"}},
		Subject:     "Hi",
		Text:        "Hi",
		Attachments: []Attachment{{Filename: "a.txt", Content: []byte("你好")}},
		Tags:        Tags{"category": "welcome"},
		ScheduledAt: &at,
	})
	if err != nil || sent.ID != "e1" {
		t.Fatalf("sent %+v, %v", sent, err)
	}
	r := s.requests[0]
	if r.Method != "POST" || r.Path != "/v1/emails" {
		t.Fatalf("request %s %s", r.Method, r.Path)
	}
	from, _ := json.Marshal(r.Body["from"])
	to, _ := json.Marshal(r.Body["to"])
	tags, _ := json.Marshal(r.Body["tags"])
	if string(from) != `{"email":"hello@acme.com","name":"Acme"}` || string(to) != `["amy@example.com"]` ||
		string(tags) != `{"category":"welcome"}` {
		t.Fatalf("from %s, to %s, tags %s", from, to, tags)
	}
	if r.Header.Get("Authorization") != "Bearer ms_test" || !uuidKey.MatchString(r.Header.Get("Idempotency-Key")) ||
		!strings.HasPrefix(r.Header.Get("User-Agent"), "ansermail-go/") {
		t.Fatalf("headers %v", r.Header)
	}
	attachment := r.Body["attachments"].([]any)[0].(map[string]any)
	if attachment["content"] != base64.StdEncoding.EncodeToString([]byte("你好")) {
		t.Fatalf("attachment %v", attachment)
	}
	if r.Body["scheduled_at"] != "2026-12-01T09:00:00Z" {
		t.Fatalf("scheduled_at %v", r.Body["scheduled_at"])
	}
	if _, ok := r.Body["html"]; ok {
		t.Fatal("empty html should be left out")
	}
}

func TestRetriesASendWithTheSameKey(t *testing.T) {
	s := newStub(t)
	s.answer = func(_ recorded, n int) (int, http.Header, string) {
		if n == 1 {
			return 503, nil, apiError("internal_error")
		}
		return 201, nil, `{"id":"e1"}`
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL))
	if _, err := client.Emails.Send(context.Background(), &SendEmailParams{From: Address{Email: "a@acme.com"}, To: []Address{{Email: "b@x.com"}}, Subject: "s", Text: "t"}); err != nil {
		t.Fatal(err)
	}
	if len(s.requests) != 2 || s.requests[0].Header.Get("Idempotency-Key") != s.requests[1].Header.Get("Idempotency-Key") {
		t.Fatalf("requests %+v", s.requests)
	}
}

func TestNeverRepeatsACreateWithoutAKey(t *testing.T) {
	s := newStub(t)
	s.answer = func(recorded, int) (int, http.Header, string) {
		return 503, nil, apiError("internal_error")
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL))
	_, err := client.Domains.Create(context.Background(), "acme.com")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 503 || len(s.requests) != 1 {
		t.Fatalf("err %v, %d requests", err, len(s.requests))
	}
}

func TestWaitsOutARateLimitButNotAQuota(t *testing.T) {
	s := newStub(t)
	s.answer = func(_ recorded, n int) (int, http.Header, string) {
		if n == 1 {
			return 429, http.Header{"Retry-After": {"1"}}, apiError("rate_limit_exceeded")
		}
		return 200, nil, `{"object":"usage"}`
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL))
	started := time.Now()
	usage, err := client.Usage.Get(context.Background())
	if err != nil || usage.Object != "usage" || time.Since(started) < 900*time.Millisecond {
		t.Fatalf("usage %+v, %v after %v", usage, err, time.Since(started))
	}

	s.requests = nil
	s.answer = func(recorded, int) (int, http.Header, string) { return 429, nil, apiError("daily_quota_exceeded") }
	_, err = client.Emails.Send(context.Background(), &SendEmailParams{From: Address{Email: "a@acme.com"}, To: []Address{{Email: "b@x.com"}}, Subject: "s", Text: "t"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "daily_quota_exceeded" || len(s.requests) != 1 {
		t.Fatalf("err %v, %d requests", err, len(s.requests))
	}
}

func TestErrors(t *testing.T) {
	s := newStub(t)
	s.answer = func(recorded, int) (int, http.Header, string) {
		return 422, http.Header{"X-Request-Id": {"r1"}},
			`{"error":{"code":"validation_error","message":"Not an email address.","param":"to[1]","request_id":"r1"}}`
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL), WithMaxRetries(0))
	_, err := client.Emails.Get(context.Background(), "nope")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 422 || apiErr.Code != "validation_error" ||
		apiErr.Param != "to[1]" || apiErr.RequestID != "r1" || apiErr.Message != "Not an email address." {
		t.Fatalf("err %+v", apiErr)
	}
	s.answer = func(recorded, int) (int, http.Header, string) { return 502, nil, "<h1>Bad Gateway</h1>" }
	_, err = client.Usage.Get(context.Background())
	if !errors.As(err, &apiErr) || apiErr.Code != "http_error" {
		t.Fatalf("err %v", err)
	}
}

func TestSendsABatch(t *testing.T) {
	s := newStub(t)
	s.answer = func(recorded, int) (int, http.Header, string) {
		return 200, nil, `{"object":"batch","data":[null],"errors":[{"index":0,"code":"domain_not_verified","message":"x","param":"emails[0].from"}]}`
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL))
	atomic := false
	result, err := client.Emails.SendBatch(context.Background(), &SendBatchParams{
		Emails: []SendEmailParams{{From: Address{Email: "a@other.com"}, To: []Address{{Email: "b@x.com"}}, Subject: "s", Text: "t"}},
		Atomic: &atomic,
	})
	if err != nil || result.Data[0] != nil || result.Errors[0].Code != "domain_not_verified" || *result.Errors[0].Param != "emails[0].from" {
		t.Fatalf("result %+v, %v", result, err)
	}
	r := s.requests[0]
	if r.Path != "/v1/emails/batch" || r.Body["atomic"] != false || len(r.Body["emails"].([]any)) != 1 {
		t.Fatalf("request %s %v", r.Path, r.Body)
	}
}

func TestTemplates(t *testing.T) {
	s := newStub(t)
	s.answer = func(r recorded, _ int) (int, http.Header, string) {
		if strings.HasSuffix(r.Path, "/versions") {
			return 200, nil, `{"object":"list","has_more":false,"data":[{"version":2,"published_by":null}]}`
		}
		if strings.HasPrefix(r.Path, "/v1/templates?") {
			return 200, nil, `{"object":"list","has_more":false,"data":[{"id":"t1","preview":{"html":null,"text":"Hi name","unavailable":"error"}}]}`
		}
		return 200, nil, `{"object":"template","id":"t1","alias":"welcome","draft":{"subject":"Hi {{name}}","variables":["name"]},"published":{"version":2,"html":"<p>Hi</p>","published_at":"2026-11-27T10:00:00Z"}}`
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL))
	ctx := context.Background()

	if _, err := client.Emails.Send(ctx, &SendEmailParams{
		To: []Address{{Email: "amy@example.com"}},
		Template: &TemplateRef{ID: "welcome", Variables: struct {
			Name string `json:"name"`
		}{"Amy"}},
	}); err != nil {
		t.Fatal(err)
	}
	created, err := client.Templates.Create(ctx, &CreateTemplateParams{Name: "Welcome", Alias: "welcome", HTML: "<p>Hi {{name}}</p>"})
	if err != nil || *created.Alias != "welcome" || created.Published.Version != 2 || *created.Published.HTML != "<p>Hi</p>" {
		t.Fatalf("created %+v, %v", created, err)
	}
	if _, err := client.Templates.Update(ctx, "welcome", &UpdateTemplateParams{Subject: Ptr("Hi {{name}}"), From: &Address{}, ReplyTo: Ptr([]Address{})}); err != nil {
		t.Fatal(err)
	}
	_, _ = client.Templates.Publish(ctx, "welcome")
	_, _ = client.Templates.Restore(ctx, "welcome", 2)
	_, _ = client.Templates.Duplicate(ctx, "a/b", "")
	versions, err := client.Templates.Versions(ctx, "welcome")
	if err != nil || len(versions) != 1 || versions[0].Version != 2 || versions[0].PublishedBy != nil {
		t.Fatalf("versions %+v, %v", versions, err)
	}
	listed, err := client.Templates.List(ctx, &ListTemplatesParams{Q: "wel", Preview: true})
	if err != nil || len(listed.Data) != 1 {
		t.Fatalf("listed %+v, %v", listed, err)
	}
	if p := listed.Data[0].Preview; p == nil || p.HTML != nil || *p.Text != "Hi name" || *p.Unavailable != "error" {
		t.Fatalf("preview %+v", p)
	}

	got := []string{}
	for _, r := range s.requests {
		body, _ := json.Marshal(r.Body)
		got = append(got, r.Method+" "+r.Path+" "+string(body))
	}
	want := []string{
		`POST /v1/emails {"template":{"id":"welcome","variables":{"name":"Amy"}},"to":["amy@example.com"]}`,
		`POST /v1/templates {"alias":"welcome","html":"\u003cp\u003eHi {{name}}\u003c/p\u003e","name":"Welcome"}`,
		`PATCH /v1/templates/welcome {"from":"","reply_to":[],"subject":"Hi {{name}}"}`,
		`POST /v1/templates/welcome/publish null`,
		`POST /v1/templates/welcome/versions/2/restore null`,
		`POST /v1/templates/a%2Fb/duplicate {}`,
		`GET /v1/templates/welcome/versions null`,
		`GET /v1/templates?preview=true&q=wel null`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestWalksEveryPage(t *testing.T) {
	s := newStub(t)
	s.answer = func(r recorded, _ int) (int, http.Header, string) {
		if strings.Contains(r.Path, "after=2") {
			return 200, nil, `{"object":"list","has_more":false,"data":[{"id":"1"}]}`
		}
		return 200, nil, `{"object":"list","has_more":true,"data":[{"id":"3"},{"id":"2"}]}`
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL))
	var ids []string
	for email, err := range client.Emails.All(context.Background(), &ListEmailsParams{ListParams: ListParams{Limit: 2}, Status: "sent"}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, email.ID)
	}
	if strings.Join(ids, ",") != "3,2,1" || s.requests[0].Path != "/v1/emails?limit=2&status=sent" {
		t.Fatalf("ids %v, first %s", ids, s.requests[0].Path)
	}
	// stopping early fetches no more pages
	s.requests = nil
	for range client.Emails.All(context.Background(), nil) {
		break
	}
	if len(s.requests) != 1 {
		t.Fatalf("%d requests", len(s.requests))
	}
}

func TestInboxFiltersAndBytes(t *testing.T) {
	s := newStub(t)
	s.answer = func(r recorded, _ int) (int, http.Header, string) {
		if strings.Contains(r.Path, "/attachments/") {
			return 200, nil, "%PDF"
		}
		return 200, nil, `{"object":"list","has_more":false,"data":[]}`
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL))
	_, err := client.Inbox.List(context.Background(), &ListReceivedParams{To: []string{"a@acme.com", "b@acme.com"}, From: "amy@x.com"})
	if err != nil || s.requests[0].Path != "/v1/inbox?from=amy%40x.com&to=a%40acme.com%2Cb%40acme.com" {
		t.Fatalf("%v %s", err, s.requests[0].Path)
	}
	data, err := client.Emails.Attachment(context.Background(), "e1", "a/1")
	if err != nil || string(data) != "%PDF" || s.requests[1].Path != "/v1/emails/e1/attachments/a%2F1" {
		t.Fatalf("%v %q %s", err, data, s.requests[1].Path)
	}
}

func TestInboxMarksAndDeletesInBatches(t *testing.T) {
	s := newStub(t)
	s.answer = func(recorded, int) (int, http.Header, string) {
		return 200, nil, `{"object":"list","has_more":false,"data":[{"id":"r1"}]}`
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL))
	ctx := context.Background()
	marked, err := client.Inbox.Mark(ctx, []string{"r1", "r2"}, true)
	if err != nil || len(marked) != 1 || marked[0].ID != "r1" {
		t.Fatalf("%v %v", err, marked)
	}
	deleted, err := client.Inbox.DeleteMany(ctx, []string{"r1"})
	if err != nil || len(deleted) != 1 || deleted[0].ID != "r1" {
		t.Fatalf("%v %v", err, deleted)
	}
	if _, err := client.Inbox.List(ctx, &ListReceivedParams{Domain: "acme.com", Read: Ptr(false)}); err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range s.requests {
		got = append(got, r.Method+" "+r.Path)
	}
	want := []string{"POST /v1/inbox/mark", "POST /v1/inbox/delete", "GET /v1/inbox?domain=acme.com&read=false"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%v", got)
	}
	if ids, _ := s.requests[0].Body["ids"].([]any); len(ids) != 2 || s.requests[0].Body["read"] != true {
		t.Fatalf("%v", s.requests[0].Body)
	}
}

func TestTimesOutAndFailsToConnect(t *testing.T) {
	s := newStub(t)
	s.answer = func(recorded, int) (int, http.Header, string) {
		time.Sleep(300 * time.Millisecond)
		return 200, nil, `{}`
	}
	client := NewClient("ms_test", WithBaseURL(s.server.URL), WithMaxRetries(0),
		WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
	_, err := client.Usage.Get(context.Background())
	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatalf("err %v", err)
	}
	_, err = NewClient("ms_test", WithBaseURL("http://127.0.0.1:9"), WithMaxRetries(0)).Usage.Get(context.Background())
	if !errors.As(err, &connErr) {
		t.Fatalf("err %v", err)
	}
	// a canceled context is the caller's, not a connection error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = client.Usage.Get(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

type fixture struct {
	Secret  string            `json:"secret"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Now     int64             `json:"now"`
}

func loadFixture(t *testing.T) fixture {
	raw, err := os.ReadFile("testdata/webhook.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestVerifiesWebhooks(t *testing.T) {
	f := loadFixture(t)
	header := http.Header{}
	for name, value := range f.Headers {
		header.Set(name, value)
	}
	now := VerifyOptions{Now: time.Unix(f.Now, 0)}
	event, err := VerifyWebhook([]byte(f.Body), header, f.Secret, now)
	if err != nil || event.Type != "email.delivered" {
		t.Fatalf("event %+v, %v", event, err)
	}
	data, err := event.EmailData()
	if err != nil || data.Email.Subject != "你好 👋" || data.Email.Tags["category"] != "welcome" ||
		data.Details["smtp_response"] != "250 2.0.0 OK" {
		t.Fatalf("data %+v, %v", data, err)
	}

	cases := []struct {
		body, secret string
		header       http.Header
		now          VerifyOptions
	}{
		{strings.Replace(f.Body, "你好", "hi", 1), f.Secret, header, now},
		{f.Body, "whsec_" + base64.StdEncoding.EncodeToString([]byte("other")), header, now},
		{f.Body, f.Secret, header, VerifyOptions{Now: time.Unix(f.Now+600, 0)}},
		{f.Body, f.Secret, http.Header{"Webhook-Id": {"x"}}, now},
	}
	for i, c := range cases {
		if _, err := VerifyWebhook([]byte(c.body), c.header, c.secret, c.now); !errors.Is(err, ErrWebhookVerification) {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}
