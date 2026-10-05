package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var updateRender = flag.Bool("update-render", false, "rewrite testdata/render golden text output")

// now is fixed just after the recorded fixtures' snapshot
// (generatedAt 2026-10-04T12:00:00Z; deploys 2026-07-21).
var now = time.Date(2026, 10, 4, 12, 5, 0, 0, time.UTC)

const testToken = "sgt_acme_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fixture struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

func load(t *testing.T, name string) fixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// server replays recorded fixtures by path and records what the CLI sent.
type server struct {
	*httptest.Server
	byPath  map[string]fixture
	headers http.Header
	seen    []*http.Request
}

func newServer(t *testing.T, byPath map[string]string, headers http.Header) *server {
	t.Helper()
	s := &server{byPath: map[string]fixture{}, headers: headers}
	for path, name := range byPath {
		s.byPath[path] = load(t, name)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.seen = append(s.seen, r.Clone(context.Background()))
		f, ok := s.byPath[r.URL.Path]
		if !ok {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusTeapot)
			return
		}
		for k, v := range s.headers {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.Status)
		_, _ = w.Write(f.Body)
	}))
	t.Cleanup(s.Close)
	return s
}

type ran struct {
	code           int
	stdout, stderr string
}

func runCLI(t *testing.T, srv *server, tty bool, environ map[string]string, args ...string) ran {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if srv != nil {
		args = append(args, "--url", srv.URL)
	}
	if environ == nil {
		environ = map[string]string{"SHIPGLANCE_TOKEN": testToken}
	}
	code := run(context.Background(), args, env{
		stdout: &stdout, stderr: &stderr, getenv: func(k string) string { return environ[k] },
		now: func() time.Time { return now }, stdoutIsTTY: tty, cacheDir: t.TempDir(),
	})
	return ran{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func envelope(t *testing.T, out string) (string, json.RawMessage, json.RawMessage) {
	t.Helper()
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("--json prints exactly one line, got %q", out)
	}
	var e struct {
		Outcome string          `json:"outcome"`
		Notice  json.RawMessage `json:"notice"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &e); err != nil {
		t.Fatalf("envelope %q: %v", out, err)
	}
	return e.Outcome, e.Notice, e.Data
}

func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return bytes.Equal(ja, jb)
}

func TestJSONEnvelopeCarriesTheServerBodyVerbatim(t *testing.T) {
	srv := newServer(t, map[string]string{"/api/v1/whoami": "whoami-hosted.json", "/api/v1/status": "status.json"}, nil)
	for command, fixtureName := range map[string]string{"whoami": "whoami-hosted.json", "status": "status.json", "drift": "status.json"} {
		got := runCLI(t, srv, false, nil, command)
		outcome, notice, data := envelope(t, got.stdout)
		if got.code != 0 || outcome != "ok" || string(notice) != "null" || !sameJSON(data, load(t, fixtureName).Body) || got.stderr != "" {
			t.Errorf("%s: code %d outcome %s notice %s stderr %q", command, got.code, outcome, notice, got.stderr)
		}
	}
	ua := srv.seen[0].Header.Get("User-Agent")
	if !regexp.MustCompile(`^shipglance-cli/dev \([a-z0-9]+/[a-z0-9]+; cli\)$`).MatchString(ua) {
		t.Errorf("User-Agent %q does not match the server's version-gate grammar", ua)
	}
	if srv.seen[0].Header.Get("Authorization") != "Bearer "+testToken {
		t.Error("the token must be sent as a bearer credential")
	}
}

func TestRefusalsMapToExitCodes(t *testing.T) {
	cases := []struct {
		fixture string
		code    int
		outcome string
	}{
		{"proxy-error-unauthenticated.json", 4, "auth"},
		{"proxy-error-token-revoked.json", 4, "auth"},
		{"proxy-error-not-found.json", 5, "usage"},
		{"proxy-error-workspace-unavailable.json", 3, "transient"},
		{"proxy-error-cli-upgrade-required.json", 7, "upgrade_required"},
		{"error-invalid-request.json", 5, "usage"},
	}
	for _, tc := range cases {
		srv := newServer(t, map[string]string{"/api/v1/status": tc.fixture}, nil)
		got := runCLI(t, srv, false, nil, "status")
		outcome, _, data := envelope(t, got.stdout)
		if got.code != tc.code || outcome != tc.outcome || !sameJSON(data, load(t, tc.fixture).Body) {
			t.Errorf("%s: exit %d outcome %s, want %d %s", tc.fixture, got.code, outcome, tc.code, tc.outcome)
		}
		text := runCLI(t, srv, true, nil, "status")
		if text.code != tc.code || !strings.HasPrefix(text.stderr, "error: ") || text.stdout != "" {
			t.Errorf("%s (terminal): exit %d stdout %q stderr %q", tc.fixture, text.code, text.stdout, text.stderr)
		}
	}
}

func TestLocalFailures(t *testing.T) {
	srv := newServer(t, map[string]string{"/api/v1/status": "status.json"}, nil)
	cases := []struct {
		name    string
		environ map[string]string
		args    []string
		code    int
		kind    string
	}{
		{"no token", map[string]string{}, []string{"status"}, 4, "unauthenticated"},
		{"not a token", map[string]string{"SHIPGLANCE_TOKEN": "ghp_x"}, []string{"status"}, 4, "unauthenticated"},
		{"unknown command", nil, []string{"deploy"}, 5, "invalid_request"},
		{"unknown flag", nil, []string{"status", "--repoo", "a/b"}, 5, "invalid_request"},
		{"stray argument", nil, []string{"status", "extra"}, 5, "invalid_request"},
		{"flag for another command", nil, []string{"whoami", "--repo", "a/b"}, 5, "invalid_request"},
	}
	for _, tc := range cases {
		got := runCLI(t, nil, false, tc.environ, append(tc.args, "--url", srv.URL)...)
		_, _, data := envelope(t, got.stdout)
		var failure struct{ Kind, Message string }
		if err := json.Unmarshal(data, &failure); err != nil || got.code != tc.code || failure.Kind != tc.kind || failure.Message == "" {
			t.Errorf("%s: exit %d data %s", tc.name, got.code, data)
		}
	}
	if len(srv.seen) != 0 {
		t.Fatalf("%d requests were sent for runs that should fail before the network", len(srv.seen))
	}
}

func TestTransportFailureIsTransient(t *testing.T) {
	srv := newServer(t, nil, nil)
	srv.Close()
	got := runCLI(t, nil, false, nil, "status", "--url", srv.URL)
	outcome, _, data := envelope(t, got.stdout)
	if got.code != 3 || outcome != "transient" || !strings.Contains(string(data), `"kind":"transport"`) {
		t.Fatalf("exit %d outcome %s data %s", got.code, outcome, data)
	}
}

func TestPanicIsAnInternalFailureNotATimeout(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"status"}, env{stdout: &bytes.Buffer{}, stderr: &stderr, now: time.Now})
	if code != 6 || !strings.Contains(stderr.String(), "internal failure") {
		t.Fatalf("a panic exits %d (%q), want 6", code, stderr.String())
	}
}

func TestUpdateNotice(t *testing.T) {
	previous := version
	version = "0.1.0"
	t.Cleanup(func() { version = previous })
	headers := http.Header{"X-Shipglance-Cli-Latest": {"0.2.0"}, "X-Shipglance-Cli-Minimum": {"0.1.0"}}
	srv := newServer(t, map[string]string{"/api/v1/whoami": "whoami-hosted.json"}, headers)

	_, notice, _ := envelope(t, runCLI(t, srv, false, nil, "whoami").stdout)
	if !strings.Contains(string(notice), `"latest":"0.2.0"`) {
		t.Fatalf("--json carries the notice every time: %s", notice)
	}

	cache := t.TempDir()
	terminal := func(environ map[string]string) string {
		var stdout, stderr bytes.Buffer
		run(context.Background(), []string{"whoami", "--url", srv.URL}, env{
			stdout: &stdout, stderr: &stderr, getenv: func(k string) string { return environ[k] },
			now: func() time.Time { return now }, stdoutIsTTY: true, cacheDir: cache,
		})
		return stderr.String()
	}
	tok := map[string]string{"SHIPGLANCE_TOKEN": testToken}
	if first := terminal(tok); !strings.Contains(first, "A newer shipglance CLI is available: 0.2.0 (you have 0.1.0)") {
		t.Fatalf("first terminal run shows the notice: %q", first)
	}
	if second := terminal(tok); second != "" {
		t.Fatalf("the notice is throttled to once a day: %q", second)
	}
	if optedOut := terminal(map[string]string{"SHIPGLANCE_TOKEN": testToken, "SHIPGLANCE_NO_UPDATE_NOTIFIER": "1"}); optedOut != "" {
		t.Fatalf("opt-out silences it: %q", optedOut)
	}
}

// TestTextOutput pins what a person sees, from the recorded fixtures.
// Re-record with: go test ./cmd/shipglance -run TestTextOutput -update-render
func TestTextOutput(t *testing.T) {
	srv := newServer(t, map[string]string{"/api/v1/whoami": "whoami-hosted.json", "/api/v1/status": "status.json"}, nil)
	for _, command := range []string{"whoami", "status", "drift"} {
		got := runCLI(t, srv, true, nil, command)
		if got.code != 0 || got.stderr != "" {
			t.Fatalf("%s: exit %d stderr %q", command, got.code, got.stderr)
		}
		output := strings.ReplaceAll(got.stdout, srv.URL, "http://BOARD")
		path := filepath.Join("..", "..", "testdata", "render", command+".txt")
		if *updateRender {
			if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (record with -update-render)", err)
		}
		if string(want) != output {
			t.Errorf("%s output changed:\n--- want\n%s--- got\n%s", command, want, output)
		}
	}
}
