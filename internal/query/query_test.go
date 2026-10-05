package query

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lastridge-hq/shipglance-cli/internal/client"
	"github.com/lastridge-hq/shipglance-cli/internal/outcome"
)

// fixture is one response recorded from the real server (make cli-fixtures
// in the server repository; testdata/golden/SOURCE names the commit).
type fixture struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

func loadFixture(t *testing.T, name string) fixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return f
}

type replay struct {
	resp client.Response
	err  error
	got  string
}

func (r *replay) Get(_ context.Context, path string, q url.Values) (client.Response, error) {
	r.got = path
	if len(q) > 0 {
		r.got += "?" + q.Encode()
	}
	return r.resp, r.err
}

func replayOf(f fixture) *replay {
	return &replay{resp: client.Response{Status: f.Status, ContentType: "application/json", Body: f.Body}}
}

// TestEveryRecordedResponseHasAnOutcome replays every fixture the server
// recorded through the real interpretation and pins the outcome an agent
// would branch on. A new fixture without a row fails, so the CLI is never
// silently untested against a response the server can send.
func TestEveryRecordedResponseHasAnOutcome(t *testing.T) {
	want := map[string]outcome.Outcome{
		"whoami.json": outcome.OK, "whoami-hosted.json": outcome.OK, "whoami-self-hosted.json": outcome.OK,
		"status.json": outcome.OK, "status-filtered-empty.json": outcome.OK,
		"error-unauthenticated.json": outcome.Auth, "proxy-error-unauthenticated.json": outcome.Auth,
		"proxy-error-token-revoked.json":         outcome.Auth,
		"error-workspace-unavailable.json":       outcome.Transient,
		"proxy-error-workspace-unavailable.json": outcome.Transient,
		"error-not-found.json":                   outcome.Usage, "proxy-error-not-found.json": outcome.Usage,
		"error-invalid-request.json":            outcome.Usage,
		"proxy-error-cli-upgrade-required.json": outcome.UpgradeRequired,
		// deploys arrives with `shipglance deploys`; recorded already.
		"deploys.json": "", "deploys-env.json": "",
	}
	entries, err := os.ReadDir(filepath.Join("..", "..", "testdata", "golden"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "SOURCE" || name == "enums.json" {
			continue
		}
		expected, ok := want[name]
		if !ok {
			t.Errorf("fixture %s has no expected outcome; add one", name)
			continue
		}
		if expected == "" {
			continue
		}
		f := loadFixture(t, name)
		decode := DecodeStatus
		if strings.HasPrefix(name, "whoami") {
			decode = DecodeWhoami
		}
		result := Exchange(context.Background(), replayOf(f), "0.1.0", "/api/v1/x", nil, decode)
		if result.Outcome != expected {
			t.Errorf("%s: outcome %s (%s), want %s", name, result.Outcome, result.Problem, expected)
		}
		if !sameJSON(t, result.Data, f.Body) {
			t.Errorf("%s: data must be the server's body verbatim", name)
		}
		if expected != outcome.OK && result.Problem == "" {
			t.Errorf("%s: a refusal needs a human message", name)
		}
	}
}

func TestUpgradeRequiredNamesWhereToUpgrade(t *testing.T) {
	result := Exchange(context.Background(), replayOf(loadFixture(t, "proxy-error-cli-upgrade-required.json")), "0.1.0", "/api/v1/status", nil, DecodeStatus)
	if result.Outcome.ExitCode() != 7 || !strings.Contains(result.Problem, "Upgrade: https://") {
		t.Fatalf("upgrade refusal = %+v", result)
	}
}

func TestStatusViewDecodesTheRecordedBoard(t *testing.T) {
	result := Exchange(context.Background(), replayOf(loadFixture(t, "status.json")), "0.1.0", "/api/v1/status", nil, DecodeStatus)
	view, ok := result.View.(*StatusView)
	if !ok || len(view.Boards) != 1 {
		t.Fatalf("view = %#v", result.View)
	}
	b := view.Boards[0]
	if b.Repo != "acme/api" || b.Drift.Kind != "drifting" || len(b.Deployments) != 1 || b.Deployments[0].Current == nil || b.Runs.Truncation == nil {
		t.Fatalf("board = %+v", b)
	}
	if view.Plan == nil || view.NotMonitored.Truncation == nil || view.NotMonitored.Truncation.Omitted != 2 {
		t.Fatalf("plan/notMonitored = %+v %+v", view.Plan, view.NotMonitored)
	}
}

// TestCompiledEnumsMatchTheServer pins the value sets this CLI renders to
// the server's frozen enums (testdata/golden/enums.json).
func TestCompiledEnumsMatchTheServer(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "enums.json"))
	if err != nil {
		t.Fatal(err)
	}
	var enums map[string][]string
	if err := json.Unmarshal(raw, &enums); err != nil {
		t.Fatal(err)
	}
	for name, compiled := range map[string]map[string]bool{
		"runStatus": RunStatuses, "attention": Attentions, "activity": Activities,
		"driftKind": DriftKinds, "freshnessKind": FreshnessKinds, "truncationReason": TruncationKinds,
	} {
		var got []string
		for v := range compiled {
			got = append(got, v)
		}
		sort.Strings(got)
		server := append([]string(nil), enums[name]...)
		sort.Strings(server)
		if strings.Join(got, ",") != strings.Join(server, ",") {
			t.Errorf("%s: CLI knows %v, server sends %v", name, got, server)
		}
	}
}

// TestUnknownFrozenValuesFailLoud: a value from a newer server is not shown
// as a known one; the run fails with exit 6 and says to upgrade.
func TestUnknownFrozenValuesFailLoud(t *testing.T) {
	f := loadFixture(t, "status.json")
	var body map[string]any
	if err := json.Unmarshal(f.Body, &body); err != nil {
		t.Fatal(err)
	}
	body["boards"].([]any)[0].(map[string]any)["attention"] = "on_fire"
	tampered, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	f.Body = tampered
	result := Exchange(context.Background(), replayOf(f), "0.1.0", "/api/v1/status", nil, DecodeStatus)
	if result.Outcome != outcome.Internal || !strings.Contains(result.Problem, "on_fire") || !strings.Contains(result.Problem, "Upgrade") {
		t.Fatalf("result = %+v", result)
	}
	unknownKind := fixture{Status: 409, Body: json.RawMessage(`{"schemaVersion":1,"error":{"kind":"brand_new","message":"x","retryable":false}}`)}
	if r := Exchange(context.Background(), replayOf(unknownKind), "0.1.0", "/x", nil, DecodeStatus); r.Outcome != outcome.Internal {
		t.Fatalf("unknown error kind = %s, want internal", r.Outcome)
	}
}

func TestTransportAndUnreadableResponses(t *testing.T) {
	transport := Exchange(context.Background(), &replay{err: errors.New("dial tcp: no such host")}, "0.1.0", "/x", nil, DecodeStatus)
	if transport.Outcome != outcome.Transient || !strings.Contains(string(transport.Data), `"kind":"transport"`) {
		t.Fatalf("transport = %+v (%s)", transport, transport.Data)
	}
	html := Exchange(context.Background(), &replay{resp: client.Response{Status: 502, ContentType: "text/html", Body: []byte("<html>bad gateway</html>")}}, "0.1.0", "/x", nil, DecodeStatus)
	if html.Outcome != outcome.Internal || !json.Valid(html.Data) {
		t.Fatalf("non-JSON = %+v", html)
	}
}

func TestNotice(t *testing.T) {
	f := loadFixture(t, "whoami.json")
	at := func(cli, latest, minimum string) *Notice {
		r := replayOf(f)
		r.resp.LatestCLI, r.resp.MinimumCLI = latest, minimum
		return Exchange(context.Background(), r, cli, "/api/v1/whoami", nil, DecodeWhoami).Notice
	}
	if n := at("0.1.0", "0.2.0", "0.1.0"); n == nil || n.Latest != "0.2.0" || n.Current != "0.1.0" || n.UpgradeURL != UpgradeURL {
		t.Fatalf("older CLI notice = %+v", n)
	}
	for name, n := range map[string]*Notice{
		"current":           at("0.2.0", "0.2.0", "0.1.0"),
		"newer than latest": at("0.3.0", "0.2.0", "0.1.0"),
		"dev build":         at("dev", "0.2.0", "0.1.0"),
		"no releases":       at("0.1.0", "", ""),
	} {
		if n != nil {
			t.Errorf("%s: unexpected notice %+v", name, n)
		}
	}
}

func TestSelectorsReachTheServer(t *testing.T) {
	r := replayOf(loadFixture(t, "status.json"))
	Status(context.Background(), r, "0.1.0", Selectors{Repo: "acme/api", Workflow: "deploy.yml"})
	if r.got != "/api/v1/status?repo=acme%2Fapi&workflow=deploy.yml" {
		t.Fatalf("requested %s", r.got)
	}
}

func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return string(ja) == string(jb)
}
