// Package forgejo is a minimal client for the one Forgejo endpoint this
// tool needs: a repo's tracked time entries.
package forgejo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client talks to a Forgejo instance's API.
type Client struct {
	BaseURL string
	Token   string

	HTTPClient *http.Client
}

// TimeEntry is one tracked-time record on a Forgejo issue.
type TimeEntry struct {
	ID          int64
	Created     time.Time
	Seconds     int64
	IssueNumber int64
}

type timeEntryJSON struct {
	ID      int64     `json:"id"`
	Created time.Time `json:"created"`
	Time    int64     `json:"time"`
	Issue   struct {
		Number int64 `json:"number"`
	} `json:"issue"`
}

// ListRepoTimes returns every tracked time entry on owner/repo.
func (c Client) ListRepoTimes(ctx context.Context, owner, repo string) ([]TimeEntry, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/times", c.BaseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.Token)

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("forgejo: GET %s: unexpected status %s", url, resp.Status)
	}

	var raw []timeEntryJSON
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("forgejo: decoding response from %s: %w", url, err)
	}

	entries := make([]TimeEntry, len(raw))
	for i, r := range raw {
		entries[i] = TimeEntry{
			ID:          r.ID,
			Created:     r.Created,
			Seconds:     r.Time,
			IssueNumber: r.Issue.Number,
		}
	}
	return entries, nil
}
