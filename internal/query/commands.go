package query

import (
	"context"
	"net/url"
)

// Selectors narrow status and drift to one repository and/or workflow; the
// server applies them (a workflow matches by path or base name).
type Selectors struct {
	Repo     string
	Workflow string
}

func (s Selectors) values() url.Values {
	q := url.Values{}
	if s.Repo != "" {
		q.Set("repo", s.Repo)
	}
	if s.Workflow != "" {
		q.Set("workflow", s.Workflow)
	}
	return q
}

func Whoami(ctx context.Context, g Getter, cliVersion string) Result {
	return Exchange(ctx, g, cliVersion, "/api/v1/whoami", nil, DecodeWhoami)
}

func Status(ctx context.Context, g Getter, cliVersion string, sel Selectors) Result {
	return Exchange(ctx, g, cliVersion, "/api/v1/status", sel.values(), DecodeStatus)
}

// Drift reads the same body as Status (docs/cli-plan.md §5: drift emits the
// full status body); only the rendering differs.
func Drift(ctx context.Context, g Getter, cliVersion string, sel Selectors) Result {
	return Exchange(ctx, g, cliVersion, "/api/v1/status", sel.values(), DecodeStatus)
}
