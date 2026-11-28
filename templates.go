package ansermail

import (
	"context"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// TemplateRef sends with a template: its latest published version, rendered with Variables. ID is the template's
// id or alias. Variables is a map or a struct that marshals to a JSON object, up to 1 MB; every variable the
// template prints must be there, or the email is refused.
type TemplateRef struct {
	ID        string `json:"id"`
	Variables any    `json:"variables,omitempty"`
}

// EmailTemplate is the template and version an email was sent with.
type EmailTemplate struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// TemplateBody is a draft or a published version. Subject, HTML and Text are Handlebars; From and ReplyTo are
// used when the email does not give its own; Variables are the top-level ones it uses (user for {{user.name}}).
type TemplateBody struct {
	Subject   *string  `json:"subject"`
	HTML      *string  `json:"html"`
	Text      *string  `json:"text"`
	From      *string  `json:"from"`
	ReplyTo   []string `json:"reply_to"`
	Variables []string `json:"variables"`
}

// PublishedTemplate is the latest published version of a template, the one emails use.
type PublishedTemplate struct {
	TemplateBody
	Version     int       `json:"version"`
	PublishedAt time.Time `json:"published_at"`
}

// Template is a template in full. Edits change Draft, which is never sent; Published is nil until first
// published. HasUnpublishedChanges says the draft differs from it.
type Template struct {
	Object                string             `json:"object"`
	ID                    string             `json:"id"`
	Name                  string             `json:"name"`
	Alias                 *string            `json:"alias"`
	Draft                 TemplateBody       `json:"draft"`
	Published             *PublishedTemplate `json:"published"`
	HasUnpublishedChanges bool               `json:"has_unpublished_changes"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

// TemplateSummary is a template in a list, without the content; Subject is the draft's.
type TemplateSummary struct {
	Object                string     `json:"object"`
	ID                    string     `json:"id"`
	Name                  string     `json:"name"`
	Alias                 *string    `json:"alias"`
	Subject               *string    `json:"subject"`
	PublishedVersion      *int       `json:"published_version"`
	PublishedAt           *time.Time `json:"published_at"`
	HasUnpublishedChanges bool       `json:"has_unpublished_changes"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	// Preview is set only when listed with Preview: true.
	Preview *TemplateSummaryPreview `json:"preview,omitempty"`
}

// TemplateSummaryPreview is the draft rendered with suggested_variables, for thumbnails. A field with a syntax error
// is nil and Unavailable is "error"; when html or text is over 128 KB nothing is rendered ("too_large").
type TemplateSummaryPreview struct {
	HTML        *string `json:"html"`
	Text        *string `json:"text"`
	Unavailable *string `json:"unavailable"`
}

// CreateTemplateParams is a new template. Alias is 1–100 letters, digits, '.', '_' and '-', starting with a letter
// or digit, unique in the workspace. HTML and Text are up to 1 MB each; publishing needs at least one.
type CreateTemplateParams struct {
	Name    string    `json:"name"`
	Alias   string    `json:"alias,omitempty"`
	Subject string    `json:"subject,omitempty"`
	HTML    string    `json:"html,omitempty"`
	Text    string    `json:"text,omitempty"`
	From    *Address  `json:"from,omitempty"`
	ReplyTo []Address `json:"reply_to,omitempty"`
}

// UpdateTemplateParams changes only what is set, and only the draft. Ptr("") clears a field (From: &Address{}
// clears the sender, ReplyTo: Ptr([]Address{}) the reply addresses).
type UpdateTemplateParams struct {
	Name    *string    `json:"name,omitempty"`
	Alias   *string    `json:"alias,omitempty"`
	Subject *string    `json:"subject,omitempty"`
	HTML    *string    `json:"html,omitempty"`
	Text    *string    `json:"text,omitempty"`
	From    *Address   `json:"from,omitempty"`
	ReplyTo *[]Address `json:"reply_to,omitempty"`
}

// ListTemplatesParams filters templates: Q matches the name and the alias. Preview adds Preview to each template.
type ListTemplatesParams struct {
	ListParams
	Q       string
	Preview bool
}

func (p *ListTemplatesParams) values() url.Values {
	if p == nil {
		return url.Values{}
	}
	v := p.ListParams.values()
	set(v, "q", p.Q)
	if p.Preview {
		v.Set("preview", "true")
	}
	return v
}

// TemplateVersionSummary is a published version in a list. PublishedBy is the email of whoever published it
// from the dashboard; nil through the API.
type TemplateVersionSummary struct {
	Object      string    `json:"object"`
	TemplateID  string    `json:"template_id"`
	Version     int       `json:"version"`
	Subject     *string   `json:"subject"`
	PublishedAt time.Time `json:"published_at"`
	PublishedBy *string   `json:"published_by"`
}

// TemplateVersion is a published version in full.
type TemplateVersion struct {
	TemplateVersionSummary
	HTML    *string  `json:"html"`
	Text    *string  `json:"text"`
	From    *string  `json:"from"`
	ReplyTo []string `json:"reply_to"`
}

// RenderTemplateParams is content to try out; nothing is stored.
type RenderTemplateParams struct {
	Subject   string `json:"subject,omitempty"`
	HTML      string `json:"html,omitempty"`
	Text      string `json:"text,omitempty"`
	Variables any    `json:"variables,omitempty"`
}

// TemplateRenderError is one field's problem. Code is syntax (it does not parse), missing_variable (printed but
// not given) or render (a helper failed or it grew too large); Line counts from 1 and is nil when not known.
type TemplateRenderError struct {
	Field   string `json:"field"`
	Line    *int   `json:"line"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TemplateRender is the rendered content; a field not given or with an error is nil. SuggestedVariables is a
// sample shaped like the template uses its variables ({{user.name}} is an object, {{#each items}} a one-item
// array): a starting point to edit, rendering with it misses nothing.
type TemplateRender struct {
	Object             string                `json:"object"`
	Subject            *string               `json:"subject"`
	HTML               *string               `json:"html"`
	Text               *string               `json:"text"`
	Variables          []string              `json:"variables"`
	SuggestedVariables map[string]any        `json:"suggested_variables"`
	Errors             []TemplateRenderError `json:"errors"`
}

// TemplatesService manages templates. Every call takes the template's id or its alias; send with one through
// SendEmailParams.Template.
type TemplatesService struct{ c *Client }

func templatePath(idOrAlias string) string { return "/templates/" + escape(idOrAlias) }

// Create makes a template with only a draft; publish it before sending with it.
func (s *TemplatesService) Create(ctx context.Context, params *CreateTemplateParams, options ...RequestOption) (*Template, error) {
	var out Template
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/templates", body: params}, &out, options)
}

// Get returns a template with its draft and published version.
func (s *TemplatesService) Get(ctx context.Context, idOrAlias string, options ...RequestOption) (*Template, error) {
	var out Template
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: templatePath(idOrAlias)}, &out, options)
}

// List returns one page, newest first.
func (s *TemplatesService) List(ctx context.Context, params *ListTemplatesParams, options ...RequestOption) (*List[TemplateSummary], error) {
	var out List[TemplateSummary]
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: "/templates", query: params.values()}, &out, options)
}

// All goes through every template matching, page after page.
func (s *TemplatesService) All(ctx context.Context, params *ListTemplatesParams, options ...RequestOption) iter.Seq2[TemplateSummary, error] {
	return all(ctx, s.c, "/templates", params.values(), func(t TemplateSummary) string { return t.ID }, options)
}

// Update changes the name, the alias or the draft; emails keep using the published version until you publish.
func (s *TemplatesService) Update(ctx context.Context, idOrAlias string, params *UpdateTemplateParams, options ...RequestOption) (*Template, error) {
	var out Template
	return &out, s.c.do(ctx, &call{method: http.MethodPatch, path: templatePath(idOrAlias), body: params}, &out, options)
}

// Delete deletes a template; emails already sent with it stay, with a nil Template.
func (s *TemplatesService) Delete(ctx context.Context, idOrAlias string, options ...RequestOption) (*Deleted, error) {
	var out Deleted
	return &out, s.c.do(ctx, &call{method: http.MethodDelete, path: templatePath(idOrAlias)}, &out, options)
}

// Publish saves the draft as the next version; emails use it from now on. It fails with the code no_changes
// when there is nothing new.
func (s *TemplatesService) Publish(ctx context.Context, idOrAlias string, options ...RequestOption) (*Template, error) {
	var out Template
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: templatePath(idOrAlias) + "/publish"}, &out, options)
}

// Duplicate copies the draft, without the alias and unpublished; an empty name means "Copy of …".
func (s *TemplatesService) Duplicate(ctx context.Context, idOrAlias, name string, options ...RequestOption) (*Template, error) {
	var out Template
	body := map[string]string{}
	if name != "" {
		body["name"] = name
	}
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: templatePath(idOrAlias) + "/duplicate", body: body}, &out, options)
}

// Versions returns the published versions, newest first.
func (s *TemplatesService) Versions(ctx context.Context, idOrAlias string, options ...RequestOption) ([]TemplateVersionSummary, error) {
	var out List[TemplateVersionSummary]
	err := s.c.do(ctx, &call{method: http.MethodGet, path: templatePath(idOrAlias) + "/versions"}, &out, options)
	return out.Data, err
}

// Version returns one published version in full.
func (s *TemplatesService) Version(ctx context.Context, idOrAlias string, version int, options ...RequestOption) (*TemplateVersion, error) {
	var out TemplateVersion
	path := templatePath(idOrAlias) + "/versions/" + strconv.Itoa(version)
	return &out, s.c.do(ctx, &call{method: http.MethodGet, path: path}, &out, options)
}

// Restore puts a version's content back into the draft; publish to send it again.
func (s *TemplatesService) Restore(ctx context.Context, idOrAlias string, version int, options ...RequestOption) (*Template, error) {
	var out Template
	path := templatePath(idOrAlias) + "/versions/" + strconv.Itoa(version) + "/restore"
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: path}, &out, options)
}

// Render renders content with variables without storing anything; every field's problems come back together.
func (s *TemplatesService) Render(ctx context.Context, params *RenderTemplateParams, options ...RequestOption) (*TemplateRender, error) {
	var out TemplateRender
	return &out, s.c.do(ctx, &call{method: http.MethodPost, path: "/templates/render", body: params}, &out, options)
}
