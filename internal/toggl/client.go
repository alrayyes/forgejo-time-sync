// Package toggl is a minimal, deliberately narrow client for Toggl's 2.0
// (Focus) API — not the older Track v9 API, which is a separate product
// with a different auth scheme and doesn't see entries created here. There
// is no method that could start, stop, or modify an existing time entry,
// which is what guarantees a manually started running timer is never
// touched by this tool: there's no code path that reaches it. Client and
// project creation are the one other thing it can do, for auto-
// provisioning — neither comes near a time entry.
package toggl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/ratelimit"
)

const (
	defaultBaseURL = "https://focus.toggl.com/api"
	maxRetries     = 5
	// defaultTagColor is sent on tag creation, since Focus requires one —
	// it has no effect on this tool's behavior, only on how the tag looks
	// in Toggl's UI.
	defaultTagColor = "#4A90D9"
)

// waiter is the one method this package needs from ratelimit.Pacer, defined
// here (the consumer) rather than there so tests can substitute a fake
// without pulling in real timing.
type waiter interface {
	Wait()
}

// Entry is a Toggl time entry, or the subset of its fields this tool cares
// about.
type Entry struct {
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspace_id"`
	ProjectID   int64  `json:"project_id"`
	Description string `json:"description"`
	Duration    int64  `json:"duration"`
}

// Client talks to the Toggl Focus API, throttled to stay under a configured
// requests-per-hour budget.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client

	apiToken       string
	organizationID int64
	pacer          waiter
}

// NewClient builds a Client throttled to maxRequestsPerHour (Toggl's Free
// tier is 30; raise it if the token is on a higher plan).
func NewClient(apiToken string, organizationID int64, maxRequestsPerHour int) *Client {
	if maxRequestsPerHour <= 0 {
		maxRequestsPerHour = 30
	}
	return NewClientWithPacer(apiToken, organizationID, ratelimit.New(time.Hour/time.Duration(maxRequestsPerHour)))
}

// NewClientWithPacer builds a Client against an explicit waiter, for tests
// that need deterministic control over throttling.
func NewClientWithPacer(apiToken string, organizationID int64, pacer waiter) *Client {
	return &Client{
		BaseURL:        defaultBaseURL,
		apiToken:       apiToken,
		organizationID: organizationID,
		pacer:          pacer,
	}
}

// NewTimeEntry describes a time entry to create. Start and Duration are
// both required and explicit, so a created entry can never register as (or
// disturb) a running timer.
type NewTimeEntry struct {
	WorkspaceID int64
	ProjectID   int64
	Start       time.Time
	Duration    time.Duration
	Description string
	// Tags attaches structured metadata beyond the free-text Description —
	// e.g. an issue reference — so it's queryable/filterable in Toggl
	// rather than only readable. Unlike Track, Focus addresses tags by id,
	// not name, so each one here is resolved (creating it if it doesn't
	// exist yet) before the entry itself is created.
	Tags []string
}

// CreateTimeEntry creates a single, already-completed, taskless time entry.
func (c *Client) CreateTimeEntry(ctx context.Context, e NewTimeEntry) (Entry, error) {
	tagIDs, err := c.resolveTagIDs(ctx, e.WorkspaceID, e.Tags)
	if err != nil {
		return Entry{}, err
	}

	body := map[string]any{
		"type":        "activity",
		"project_id":  e.ProjectID,
		"start":       e.Start.UTC().Format(time.RFC3339),
		"duration":    int64(e.Duration.Seconds()),
		"description": e.Description,
	}
	if len(tagIDs) > 0 {
		body["tag_ids"] = tagIDs
	}

	path := fmt.Sprintf("/organizations/%d/workspaces/%d/time-entries", c.organizationID, e.WorkspaceID)
	resp, err := c.postJSON(ctx, path, body)
	if err != nil {
		return Entry{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var created Entry
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return Entry{}, fmt.Errorf("toggl: decoding create-time-entry response: %w", err)
	}
	return created, nil
}

// resolveTagIDs finds or creates a tag for each name in workspaceID,
// returning their ids in the same order.
func (c *Client) resolveTagIDs(ctx context.Context, workspaceID int64, names []string) ([]int64, error) {
	if len(names) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(names))
	for _, name := range names {
		id, err := c.findOrCreateTagID(ctx, workspaceID, name)
		if err != nil {
			return nil, fmt.Errorf("toggl: resolving tag %q: %w", name, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (c *Client) findOrCreateTagID(ctx context.Context, workspaceID int64, name string) (int64, error) {
	path := fmt.Sprintf("/workspaces/%d/tags", workspaceID)
	query := url.Values{"name": {name}}

	tags, err := c.listNamedResources(ctx, path+"?"+query.Encode())
	if err != nil {
		return 0, fmt.Errorf("listing tags: %w", err)
	}
	for _, t := range tags {
		if t.Name == name {
			return t.ID, nil
		}
	}

	created, err := c.createNamedResource(ctx, path, map[string]any{"name": name, "color": defaultTagColor})
	if err != nil {
		return 0, fmt.Errorf("creating tag: %w", err)
	}
	return created.ID, nil
}

// namedResource is the subset of a Toggl client, project, or tag this
// package cares about: enough to find one by name (and, for a project, by
// the client it belongs to) without pulling in every field Toggl returns.
type namedResource struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ClientID int64  `json:"client_id"`
}

// namedResourcePage is the envelope Focus wraps every list response in.
type namedResourcePage struct {
	Data []namedResource `json:"data"`
}

// FindOrCreateClient returns the ID of the Toggl client named name in
// workspaceID, creating one if none exists yet.
func (c *Client) FindOrCreateClient(ctx context.Context, workspaceID int64, name string) (int64, error) {
	path := fmt.Sprintf("/workspaces/%d/clients", workspaceID)
	query := url.Values{"name": {name}}

	clients, err := c.listNamedResources(ctx, path+"?"+query.Encode())
	if err != nil {
		return 0, fmt.Errorf("toggl: listing clients: %w", err)
	}
	for _, cl := range clients {
		if cl.Name == name {
			return cl.ID, nil
		}
	}

	created, err := c.createNamedResource(ctx, path, map[string]any{"name": name})
	if err != nil {
		return 0, fmt.Errorf("toggl: creating client %q: %w", name, err)
	}
	return created.ID, nil
}

// FindOrCreateProject returns the ID of the Toggl project named name under
// clientID in workspaceID, creating one if none exists yet.
func (c *Client) FindOrCreateProject(ctx context.Context, workspaceID, clientID int64, name string) (int64, error) {
	query := url.Values{"name": {name}}
	path := fmt.Sprintf("/organizations/%d/workspaces/%d/projects", c.organizationID, workspaceID)

	projects, err := c.listNamedResources(ctx, path+"?"+query.Encode())
	if err != nil {
		return 0, fmt.Errorf("toggl: listing projects: %w", err)
	}
	for _, p := range projects {
		if p.Name == name && p.ClientID == clientID {
			return p.ID, nil
		}
	}

	created, err := c.createNamedResource(ctx, path, map[string]any{"name": name, "client_id": clientID})
	if err != nil {
		return 0, fmt.Errorf("toggl: creating project %q: %w", name, err)
	}
	return created.ID, nil
}

func (c *Client) listNamedResources(ctx context.Context, path string) ([]namedResource, error) {
	resp, err := c.getJSON(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var page namedResourcePage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return page.Data, nil
}

func (c *Client) createNamedResource(ctx context.Context, path string, body map[string]any) (namedResource, error) {
	resp, err := c.postJSON(ctx, path, body)
	if err != nil {
		return namedResource{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var created namedResource
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return namedResource{}, fmt.Errorf("decoding response: %w", err)
	}
	return created, nil
}

// ListRecentEntries returns workspaceID's time entries since the given
// time, up to a single generously-sized page. Used only for one-time
// cold-start reconciliation, never on the steady-state poll path — a
// single-repo sync tool's 90-day lookback isn't expected to exceed one
// page, so there's no pagination loop here.
func (c *Client) ListRecentEntries(ctx context.Context, workspaceID int64, since time.Time) ([]Entry, error) {
	path := fmt.Sprintf("/organizations/%d/workspaces/%d/time-entries", c.organizationID, workspaceID)
	query := url.Values{
		"date_from":        {since.UTC().Format(time.RFC3339)},
		"date_to":          {time.Now().UTC().Format(time.RFC3339)},
		"include_taskless": {"true"},
		"per_page":         {"200"},
	}

	resp, err := c.getJSON(ctx, path+"?"+query.Encode())
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var page struct {
		Data []Entry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("toggl: decoding time entries response: %w", err)
	}
	return page.Data, nil
}

func (c *Client) postJSON(ctx context.Context, path string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.doWithRetry(ctx, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	})
}

func (c *Client) getJSON(ctx context.Context, path string) (*http.Response, error) {
	return c.doWithRetry(ctx, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	})
}

// doWithRetry paces and sends a request built fresh on every attempt
// (bodies can only be read once), retrying on Toggl's rate-limit response
// (402, not the usual 429) up to maxRetries times.
func (c *Client) doWithRetry(ctx context.Context, buildRequest func(context.Context) (*http.Request, error)) (*http.Response, error) {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	var lastStatus string
	for attempt := 0; attempt < maxRetries; attempt++ {
		c.pacer.Wait()

		req, err := buildRequest(ctx)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusPaymentRequired {
			lastStatus = resp.Status
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer func() { _ = resp.Body.Close() }()
			return nil, fmt.Errorf("toggl: %s %s: unexpected status %s", req.Method, req.URL.Path, resp.Status)
		}
		return resp, nil
	}
	return nil, fmt.Errorf("toggl: still rate limited after %d attempts (last status %s)", maxRetries, lastStatus)
}
