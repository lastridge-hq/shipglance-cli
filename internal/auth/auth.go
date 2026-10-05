// Package auth resolves which workspace the CLI talks to and with what
// credential. A workspace token (sgt_<slug>_<secret>) names its own
// workspace: the slug selects the host, so no workspace flag is needed. The
// slug is only a routing hint — the server decides what the token may read.
package auth

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// TokenEnv is the environment variable the CLI reads its token from.
const TokenEnv = "SHIPGLANCE_TOKEN"

// DefaultBaseDomain is the hosted product's domain.
const DefaultBaseDomain = "shipglance.com"

var tokenPattern = regexp.MustCompile(`^sgt_([a-z0-9][a-z0-9-]{0,38}[a-z0-9])_[0-9a-f]{64}$`)

// Target is a resolved destination: where to send requests and the bearer
// token to send.
type Target struct {
	BaseURL   string
	Workspace string
	Token     string
}

// Problem is why no target could be resolved. Kind is a v1/local error kind
// (internal/outcome) so it is reported like any other refusal.
type Problem struct {
	Kind    string
	Message string
}

func (p *Problem) Error() string { return p.Message }

// Resolve builds the target from the token and an optional --url override.
// The override exists for development against a local board; it keeps the
// token's credential but replaces the host.
func Resolve(token, overrideURL string) (Target, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Target{}, &Problem{Kind: "unauthenticated", Message: "No token. Create one in your board's Settings → API tokens, then export " + TokenEnv + "=<token>."}
	}
	match := tokenPattern.FindStringSubmatch(token)
	if match == nil {
		return Target{}, &Problem{Kind: "unauthenticated", Message: TokenEnv + " is not a workspace token. Tokens start with sgt_ and are copied whole from Settings → API tokens."}
	}
	target := Target{Workspace: match[1], Token: token, BaseURL: "https://" + match[1] + "." + DefaultBaseDomain}
	if overrideURL != "" {
		parsed, err := url.Parse(overrideURL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return Target{}, &Problem{Kind: "invalid_request", Message: "--url must be an absolute http(s) URL, like https://acme.shipglance.com."}
		}
		target.BaseURL = strings.TrimSuffix(parsed.Scheme+"://"+parsed.Host+parsed.Path, "/")
	}
	return target, nil
}

// AsProblem extracts a Problem from an error.
func AsProblem(err error) (*Problem, bool) {
	var p *Problem
	ok := errors.As(err, &p)
	return p, ok
}
