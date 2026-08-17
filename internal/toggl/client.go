// Package toggl is a minimal, deliberately narrow client for Toggl Track's
// API v9. It can create completed time entries and list recent ones — that
// is the entire surface. There is no method that could start, stop, or
// modify an existing entry, which is what guarantees a manually started
// running timer is never touched by this tool: there's no code path that
// reaches it.
package toggl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/ratelimit"
)

const (
	defaultBaseURL  = "https://api.track.toggl.com"
	apiTokenBasicPW = "api_token"
	maxRetries      = 5
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

// Client talks to the Toggl Track API, throttled to stay under a configured
// requests-per-hour budget.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client

	apiToken string
	pacer    waiter
}

// NewClient builds a Client throttled to maxRequestsPerHour (Toggl's Free
// tier is 30; raise it if the token is on a higher plan).
func NewClient(apiToken string, maxRequestsPerHour int) *Client {
	if maxRequestsPerHour <= 0 {
		maxRequestsPerHour = 30
	}
	return NewClientWithPacer(apiToken, ratelimit.New(time.Hour/time.Duration(maxRequestsPerHour)))
}

// NewClientWithPacer builds a Client against an explicit waiter, for tests
// that need deterministic control over throttling.
func NewClientWithPacer(apiToken string, pacer waiter) *Client {
	return &Client{
		BaseURL:  defaultBaseURL,
		apiToken: apiToken,
		pacer:    pacer,
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
	// rather than only readable. Toggl creates any tag that doesn't
	// already exist in the workspace.
	Tags []string
}

// CreateTimeEntry creates a single, already-completed time entry.
func (c *Client) CreateTimeEntry(ctx context.Context, e NewTimeEntry) (Entry, error) {
	body := map[string]any{
		"workspace_id": e.WorkspaceID,
		"project_id":   e.ProjectID,
		"start":        e.Start.UTC().Format(time.RFC3339),
		"duration":     int64(e.Duration.Seconds()),
		"description":  e.Description,
		"created_with": "forgejo-time-sync",
	}
	if len(e.Tags) > 0 {
		body["tags"] = e.Tags
	}

	path := fmt.Sprintf("/api/v9/workspaces/%d/time_entries", e.WorkspaceID)
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

// ListRecentEntries returns the authenticated user's time entries since the
// given time. Used only for one-time cold-start reconciliation, never on
// the steady-state poll path.
func (c *Client) ListRecentEntries(ctx context.Context, since time.Time) ([]Entry, error) {
	path := fmt.Sprintf("/api/v9/me/time_entries?since=%d", since.Unix())
	resp, err := c.getJSON(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var entries []Entry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("toggl: decoding time entries response: %w", err)
	}
	return entries, nil
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
		req.SetBasicAuth(c.apiToken, apiTokenBasicPW)
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
