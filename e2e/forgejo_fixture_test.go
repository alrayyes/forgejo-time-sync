//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	forgejoImage    = "codeberg.org/forgejo/forgejo:12@sha256:dbb0f88677f0c65cd1b66fb83504225aa5a04c4bc4a5ffdf9fc9a3a6d5bb1c68"
	forgejoAdmin    = "sync-test-admin"
	forgejoPassword = "sync-test-password-123!"
)

// forgejoFixture is a running Forgejo instance with an admin token, ready
// to have a repo, issue, and time entry provisioned against it.
type forgejoFixture struct {
	baseURL string
	token   string
}

// startForgejo boots a real Forgejo container, unattended (no setup
// wizard), and mints an admin API token via `forgejo admin`, executed as
// the `git` user — Forgejo refuses to run its own CLI as root.
func startForgejo(t *testing.T, ctx context.Context) forgejoFixture {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        forgejoImage,
		ExposedPorts: []string{"3000/tcp"},
		Env: map[string]string{
			"FORGEJO__security__INSTALL_LOCK":        "true",
			"FORGEJO__database__DB_TYPE":             "sqlite3",
			"FORGEJO__service__DISABLE_REGISTRATION": "true",
			"FORGEJO__service__REQUIRE_SIGNIN_VIEW":  "false",
		},
		WaitingFor: wait.ForLog("Starting new Web server"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	baseURL, err := container.PortEndpoint(ctx, "3000/tcp", "http")
	require.NoError(t, err)

	mustExec(t, ctx, container, []string{
		"forgejo", "admin", "user", "create",
		"--username", forgejoAdmin,
		"--password", forgejoPassword,
		"--email", "admin@example.com",
		"--admin",
	})

	tokenOutput := mustExecOutput(t, ctx, container, []string{
		"forgejo", "admin", "user", "generate-access-token",
		"--username", forgejoAdmin,
		"--token-name", "e2e",
		"--scopes", "all",
	})

	return forgejoFixture{baseURL: baseURL, token: parseGeneratedToken(t, tokenOutput)}
}

var generatedTokenPattern = regexp.MustCompile(`(?m)Access token was successfully created:\s*(\S+)`)

func parseGeneratedToken(t *testing.T, output string) string {
	t.Helper()
	match := generatedTokenPattern.FindStringSubmatch(output)
	require.Lenf(t, match, 2, "could not find a generated token in forgejo CLI output: %q", output)
	return match[1]
}

func mustExec(t *testing.T, ctx context.Context, c testcontainers.Container, cmd []string) {
	t.Helper()
	mustExecOutput(t, ctx, c, cmd)
}

func mustExecOutput(t *testing.T, ctx context.Context, c testcontainers.Container, cmd []string) string {
	t.Helper()

	code, reader, err := c.Exec(ctx, cmd, tcexec.WithUser("git"), tcexec.Multiplexed())
	require.NoError(t, err)

	out, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Zerof(t, code, "exec %v failed: %s", cmd, out)

	return string(out)
}

// createRepoWithTrackedTime provisions a repo, one issue, and one tracked
// time entry on that issue, all owned by the admin user, and returns the
// created time entry's Forgejo ID.
func (f forgejoFixture) createRepoWithTrackedTime(t *testing.T, repo string, seconds int64) int64 {
	t.Helper()

	f.apiRequest(t, http.MethodPost, "/api/v1/user/repos", map[string]any{
		"name":    repo,
		"private": false,
	})

	var issue struct {
		Index int64 `json:"number"`
	}
	f.apiRequestInto(t, &issue, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/issues", forgejoAdmin, repo), map[string]any{
		"title": "time tracking test issue",
	})

	var entry struct {
		ID int64 `json:"id"`
	}
	f.apiRequestInto(t, &entry, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/times", forgejoAdmin, repo, issue.Index), map[string]any{
		"time": seconds,
	})

	return entry.ID
}

func (f forgejoFixture) apiRequest(t *testing.T, method, path string, body any) {
	t.Helper()
	f.apiRequestInto(t, nil, method, path, body)
}

func (f forgejoFixture) apiRequestInto(t *testing.T, into any, method, path string, body any) {
	t.Helper()

	data, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(method, f.baseURL+path, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Authorization", "token "+f.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Lessf(t, resp.StatusCode, 300, "%s %s: status %d: %s", method, path, resp.StatusCode, respBody)

	if into != nil {
		require.NoError(t, json.Unmarshal(respBody, into))
	}
}
