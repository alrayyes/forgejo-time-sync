//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const prismImage = "stoplight/prism:5.14.2@sha256:87972b6cf73da2c0339fca18f362b6f93118152d652c970fab93ed13cfd55bae"

// startPrismMock boots a Prism mock server, loaded with the vendored Toggl
// Focus (2.0) API OpenAPI spec, and returns its base URL. Every request
// Prism accepts is a request that conforms to Toggl's real published
// contract — the point of mocking against the spec rather than hand-writing
// fake responses.
//
// The vendored spec (testdata/focus-openapi.json) has two hand-patches on
// top of what Toggl publishes, both worked around at the mock layer rather
// than in toggl.Client, since neither reflects real API behavior: every
// operation's `security` requirement was stripped (as vendored, it demands
// a bearerAuth token *and* a cookieAuth cookie together, which would 401 a
// client that only ever sends a bearer token — not how the real API works),
// and every `id` property gained `"minimum": 1` (Prism's dynamic response
// generator will otherwise happily hand out negative ids, which this tool's
// state cache reads the same as Track's zero-id case: "nothing resolved
// yet").
func startPrismMock(t *testing.T, ctx context.Context) string {
	t.Helper()

	specPath, err := filepath.Abs("testdata/focus-openapi.json")
	require.NoError(t, err)

	req := testcontainers.ContainerRequest{
		Image:        prismImage,
		ExposedPorts: []string{"4010/tcp"},
		// -d (dynamic) generates fake response data from the schema
		// itself rather than a static all-zeros/all-"string" default —
		// needed so a created resource's id looks like a real Toggl id
		// (a positive integer) rather than always 0, which the state
		// cache would read as "nothing resolved yet". --seed keeps it
		// deterministic across runs.
		Cmd: []string{"mock", "-h", "0.0.0.0", "-d", "--seed", "1", "/data/spec.json"},
		Files: []testcontainers.ContainerFile{
			{HostFilePath: specPath, ContainerFilePath: "/data/spec.json", FileMode: 0o644},
		},
		WaitingFor: wait.ForLog("Prism is listening"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	endpoint, err := container.PortEndpoint(ctx, "4010/tcp", "http")
	require.NoError(t, err)
	return endpoint
}

// recordedRequest is what recordingProxy captured on its way to Prism.
type recordedRequest struct {
	Method string
	Path   string
}

// recordingProxy sits between the client under test and Prism, so a test
// can assert on exactly which requests were made (in particular: that
// .../time-entries only ever sees a POST, never a PUT/PATCH/DELETE that
// could start, stop, or modify an existing entry) while Prism still does
// the real contract validation and response generation.
type recordingProxy struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest
}

func newRecordingProxy(t *testing.T, targetBaseURL string) *recordingProxy {
	t.Helper()

	p := &recordingProxy{}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.record(r)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		// toggl.Client's default BaseURL already carries the real API's
		// /api prefix, so the paths it builds (e.g. /organizations/...)
		// are bare — matching Prism, which ignores the spec's Swagger 2
		// `basePath` and serves every path bare too. Nothing to rewrite.
		targetURL := targetBaseURL + r.URL.Path
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		outbound, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		outbound.Header = r.Header.Clone()

		resp, err := http.DefaultClient.Do(outbound)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		for key, values := range resp.Header {
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *recordingProxy) record(r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, recordedRequest{Method: r.Method, Path: r.URL.Path})
}

func (p *recordingProxy) URL() string { return p.server.URL }

func (p *recordingProxy) Requests() []recordedRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]recordedRequest, len(p.requests))
	copy(out, p.requests)
	return out
}
