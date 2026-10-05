// Package query turns one /api/v1 exchange into a Result: the outcome, the
// verbatim data, a typed view for rendering, and the update notice. It does
// no rendering, no file I/O and never exits, so the CLI and (later) the MCP
// server share it unchanged.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/url"

	"github.com/lastridge-hq/shipglance-cli/internal/client"
	"github.com/lastridge-hq/shipglance-cli/internal/outcome"
	"github.com/lastridge-hq/shipglance-cli/internal/version"
)

// Getter is the network seam (internal/client.Client in production).
type Getter interface {
	Get(ctx context.Context, path string, query url.Values) (client.Response, error)
}

// UpgradeURL is where every update prompt points: the latest release lists
// every install channel.
const UpgradeURL = "https://github.com/lastridge-hq/shipglance-cli/releases/latest"

// Notice says a newer CLI exists. It is data; each edge decides how to show
// it (stderr once a day on a terminal, inline in --json and MCP).
type Notice struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	Minimum    string `json:"minimum"`
	UpgradeURL string `json:"upgradeUrl"`
}

// Failure is the {kind, message} the CLI reports when there is no server
// error body to pass through (a transport failure, a response it cannot
// read, a missing token).
type Failure struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// ServerError is the part of a v1 error body the CLI reads for display.
type ServerError struct {
	Kind           string `json:"kind"`
	Message        string `json:"message"`
	Retryable      bool   `json:"retryable"`
	MinimumVersion string `json:"minimumVersion"`
	UpgradeURL     string `json:"upgradeUrl"`
	Feature        string `json:"feature"`
}

// Result is one command's outcome. Data is the v1 response body verbatim
// (success or error), or a CLI-authored Failure; View is the typed decoding
// used for rendering (nil unless Outcome is OK).
type Result struct {
	Outcome outcome.Outcome
	Data    json.RawMessage
	View    any
	Notice  *Notice
	// Problem is the human message for a non-OK outcome.
	Problem string
}

// Fail builds a Result for a failure found before or without a server body.
func Fail(kind, message string) Result {
	o, ok := outcome.ForKind(kind)
	if !ok {
		o = outcome.Internal
	}
	data, _ := json.Marshal(Failure{Kind: kind, Message: message})
	return Result{Outcome: o, Data: data, Problem: message}
}

// Exchange performs one GET and interprets it. decode turns a successful
// body into its view and checks the frozen enums it relies on.
func Exchange(ctx context.Context, g Getter, cliVersion, path string, q url.Values, decode func([]byte) (any, error)) Result {
	resp, err := g.Get(ctx, path, q)
	if err != nil {
		return Fail(outcome.KindTransport, fmt.Sprintf("Could not reach the workspace: %v", err))
	}
	result := interpret(resp, decode)
	result.Notice = notice(cliVersion, resp)
	return result
}

func interpret(resp client.Response, decode func([]byte) (any, error)) Result {
	if !isJSON(resp.ContentType) || !json.Valid(resp.Body) {
		return Fail(outcome.KindInternal, fmt.Sprintf("The server answered HTTP %d with something other than JSON; this CLI cannot read it.", resp.Status))
	}
	if resp.Status >= 400 {
		var body struct {
			SchemaVersion int         `json:"schemaVersion"`
			Error         ServerError `json:"error"`
		}
		if err := json.Unmarshal(resp.Body, &body); err != nil || body.Error.Kind == "" {
			return Fail(outcome.KindInternal, fmt.Sprintf("The server answered HTTP %d with an error this CLI cannot read.", resp.Status))
		}
		o, known := outcome.ForKind(body.Error.Kind)
		if !known {
			// Fail loud: an error kind from a newer server is not guessed at.
			return Result{Outcome: outcome.Internal, Data: resp.Body, Problem: fmt.Sprintf("The server reported %q, which this CLI does not know. Upgrade the CLI: %s", body.Error.Kind, UpgradeURL)}
		}
		return Result{Outcome: o, Data: resp.Body, Problem: describe(body.Error)}
	}
	view, err := decode(resp.Body)
	if err != nil {
		return Result{Outcome: outcome.Internal, Data: resp.Body, Problem: err.Error()}
	}
	return Result{Outcome: outcome.OK, Data: resp.Body, View: view}
}

func describe(e ServerError) string {
	switch e.Kind {
	case "cli_upgrade_required":
		url := e.UpgradeURL
		if url == "" {
			url = UpgradeURL
		}
		return fmt.Sprintf("%s\nUpgrade: %s", e.Message, url)
	case "plan_feature_locked":
		if e.UpgradeURL != "" {
			return fmt.Sprintf("%s\nUpgrade your plan: %s", e.Message, e.UpgradeURL)
		}
	}
	return e.Message
}

// notice compares this CLI with the server's advertised latest. A build
// without a parsable version (a local "dev" build) never nags.
func notice(cliVersion string, resp client.Response) *Notice {
	current, ok := version.Parse(cliVersion)
	latest, okLatest := version.Parse(resp.LatestCLI)
	if !ok || !okLatest || !current.Less(latest) {
		return nil
	}
	minimum, _ := version.Parse(resp.MinimumCLI)
	return &Notice{Current: current.String(), Latest: latest.String(), Minimum: minimum.String(), UpgradeURL: UpgradeURL}
}

func isJSON(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	return err == nil && media == "application/json"
}
