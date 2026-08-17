// Package forgejo wraps the official Forgejo SDK
// (codeberg.org/mvdkleijn/forgejo-sdk) for the handful of calls this tool
// needs: reading a repo's tracked time, and — for stopwatch mirroring —
// listing, starting and stopping issue stopwatches.
package forgejo

import (
	"context"
	"errors"
	"fmt"
	"time"

	sdk "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
)

// TimeEntry is one tracked-time record on a Forgejo issue.
type TimeEntry struct {
	ID          int64
	Created     time.Time
	Seconds     int64
	IssueNumber int64
}

// Client talks to a Forgejo instance via the official SDK.
type Client struct {
	sdk *sdk.Client
}

// NewClient connects to the Forgejo instance at baseURL, authenticated with
// token. The SDK checks the server's version as part of construction, so
// this is one network round-trip up front rather than a zero-cost struct
// literal — an unknown/unrecognized version is tolerated (Forgejo is a
// young-enough fork that a strict lower-bound check isn't worth failing
// startup over), but a real connection failure is returned as an error.
func NewClient(baseURL, token string) (*Client, error) {
	c, err := sdk.NewClient(baseURL, sdk.SetToken(token))
	if err != nil {
		var unknownVersion *sdk.ErrUnknownVersion
		if !errors.As(err, &unknownVersion) {
			return nil, fmt.Errorf("forgejo: connecting to %s: %w", baseURL, err)
		}
	}
	return &Client{sdk: c}, nil
}

// ListRepoTimes returns every tracked time entry on owner/repo.
func (c *Client) ListRepoTimes(ctx context.Context, owner, repo string) ([]TimeEntry, error) {
	c.sdk.SetContext(ctx)

	raw, _, err := c.sdk.ListRepoTrackedTimes(owner, repo, sdk.ListTrackedTimesOptions{
		ListOptions: sdk.ListOptions{Page: -1}, // -1 disables pagination: fetch everything in one call.
	})
	if err != nil {
		return nil, fmt.Errorf("forgejo: listing tracked times for %s/%s: %w", owner, repo, err)
	}

	entries := make([]TimeEntry, len(raw))
	for i, r := range raw {
		entries[i] = TimeEntry{ID: r.ID, Created: r.Created, Seconds: r.Time}
		if r.Issue != nil {
			entries[i].IssueNumber = r.Issue.Index
		}
	}
	return entries, nil
}
