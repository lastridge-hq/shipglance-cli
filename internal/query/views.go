package query

import (
	"encoding/json"
	"fmt"
	"time"
)

// The views decode the parts of v1 bodies the renderer shows. Unknown fields
// are ignored (v1 only ever adds fields); unknown values of the frozen enums
// the renderer branches on are an error, because rendering them as a known
// value would be a lie.

type Plan struct {
	Label       string `json:"label"`
	HistoryDays int    `json:"historyDays"`
	UpgradeURL  string `json:"upgradeUrl"`
}

type WhoamiView struct {
	SchemaVersion int     `json:"schemaVersion"`
	ServerVersion string  `json:"serverVersion"`
	Title         string  `json:"title"`
	Workspace     *string `json:"workspace"`
	Plan          *Plan   `json:"plan"`
	Capabilities  struct {
		History bool `json:"history"`
		Replay  bool `json:"replay"`
	} `json:"capabilities"`
}

type Truncation struct {
	Reason   string     `json:"reason"`
	Boundary *time.Time `json:"boundary"`
	Omitted  int        `json:"omitted"`
}

type DriftState struct {
	Kind              string     `json:"kind"`
	Reason            string     `json:"reason"`
	ChangedPathsTotal int        `json:"changedPathsTotal"`
	CommitsTotal      int        `json:"commitsTotal"`
	DriftingSince     *time.Time `json:"driftingSince"`
}

type Deployment struct {
	Stage   string `json:"stage"`
	Env     string `json:"env"`
	Target  string `json:"target"`
	Current *struct {
		SHA         string     `json:"sha"`
		RunNumber   int64      `json:"runNumber"`
		CompletedAt *time.Time `json:"completedAt"`
	} `json:"current"`
	Drift DriftState `json:"drift"`
}

type Run struct {
	RunID     string    `json:"runId"`
	RunNumber int64     `json:"runNumber"`
	Status    string    `json:"status"`
	SHA       string    `json:"sha"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
	WebURL    string    `json:"webUrl"`
}

type Board struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Repo        string       `json:"repo"`
	Branch      string       `json:"branch"`
	Workflow    string       `json:"workflow"`
	WebURL      string       `json:"webUrl"`
	Status      string       `json:"status"`
	Attention   string       `json:"attention"`
	Activity    string       `json:"activity"`
	Drift       DriftState   `json:"drift"`
	Deployments []Deployment `json:"deployments"`
	Runs        struct {
		Items      []Run       `json:"items"`
		Truncation *Truncation `json:"truncation"`
	} `json:"runs"`
	Error *struct {
		Kind      string `json:"kind"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable"`
	} `json:"error"`
	Freshness struct {
		Kind      string     `json:"kind"`
		FetchedAt *time.Time `json:"fetchedAt"`
	} `json:"freshness"`
}

type NotMonitored struct {
	Repo     string  `json:"repo"`
	Workflow *string `json:"workflow"`
	Name     string  `json:"name"`
}

type StatusView struct {
	SchemaVersion int       `json:"schemaVersion"`
	GeneratedAt   time.Time `json:"generatedAt"`
	Boards        []Board   `json:"boards"`
	NotMonitored  struct {
		Items      []NotMonitored `json:"items"`
		Truncation *Truncation    `json:"truncation"`
	} `json:"notMonitored"`
	Plan *Plan `json:"plan"`
}

// The frozen v1 value sets this CLI renders (the server's enums.json; the
// tests pin these against the vendored copy).
var (
	RunStatuses     = set("success", "failure", "running", "queued", "skipped", "cancelled", "notrun", "unknown")
	Attentions      = set("failed", "active", "drifting", "uncertain", "steady", "quiet")
	Activities      = set("quiet", "steady", "queued", "running", "failed")
	DriftKinds      = set("not_evaluated", "up_to_date", "drifting", "unknown")
	FreshnessKinds  = set("fresh", "retained")
	TruncationKinds = set("retention", "provider_window", "list_cap")
)

func set(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

func DecodeWhoami(body []byte) (any, error) {
	var w WhoamiView
	if err := decodeV1(body, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func DecodeStatus(body []byte) (any, error) {
	var s StatusView
	if err := decodeV1(body, &s); err != nil {
		return nil, err
	}
	for _, b := range s.Boards {
		checks := [][2]string{{"status", b.Status}, {"attention", b.Attention}, {"activity", b.Activity}, {"drift.kind", b.Drift.Kind}, {"freshness.kind", b.Freshness.Kind}}
		for _, d := range b.Deployments {
			checks = append(checks, [2]string{"deployment drift.kind", d.Drift.Kind})
		}
		for _, r := range b.Runs.Items {
			checks = append(checks, [2]string{"run status", r.Status})
		}
		if t := b.Runs.Truncation; t != nil {
			checks = append(checks, [2]string{"truncation.reason", t.Reason})
		}
		for _, c := range checks {
			if !known(c[0], c[1]) {
				return nil, fmt.Errorf("board %s has %s %q, which this CLI does not know. Upgrade the CLI: %s", b.ID, c[0], c[1], UpgradeURL)
			}
		}
	}
	return &s, nil
}

func known(field, value string) bool {
	switch field {
	case "status", "run status":
		return RunStatuses[value]
	case "attention":
		return Attentions[value]
	case "activity":
		return Activities[value]
	case "drift.kind", "deployment drift.kind":
		return DriftKinds[value]
	case "freshness.kind":
		return FreshnessKinds[value]
	case "truncation.reason":
		return TruncationKinds[value]
	default:
		return false
	}
}

// decodeV1 decodes a v1 body and requires schema version 1.
func decodeV1(body []byte, into any) error {
	var probe struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return fmt.Errorf("the server's answer could not be read: %v", err)
	}
	if probe.SchemaVersion != 1 {
		return fmt.Errorf("the server answered with schema version %d; this CLI reads version 1. Upgrade the CLI: %s", probe.SchemaVersion, UpgradeURL)
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("the server's answer could not be read: %v", err)
	}
	return nil
}
