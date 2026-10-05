package render

import (
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lastridge/shipglance-cli/internal/query"
)

// Problem writes a refusal or failure for a person.
func Problem(w io.Writer, result query.Result) {
	fmt.Fprintf(w, "error: %s\n", result.Problem)
}

// Notice writes the update notice for a person.
func Notice(w io.Writer, n *query.Notice) {
	if n == nil {
		return
	}
	fmt.Fprintf(w, "A newer shipglance CLI is available: %s (you have %s). Upgrade: %s\n", n.Latest, n.Current, n.UpgradeURL)
}

func Whoami(w io.Writer, v *query.WhoamiView, baseURL string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	name := v.Title
	if v.Workspace != nil {
		name = *v.Workspace
	}
	fmt.Fprintf(tw, "Workspace\t%s (%s)\n", name, baseURL)
	if v.Plan != nil {
		fmt.Fprintf(tw, "Plan\t%s — %d days of history\n", v.Plan.Label, v.Plan.HistoryDays)
	} else {
		fmt.Fprintf(tw, "Plan\tnone (self-hosted)\n")
	}
	fmt.Fprintf(tw, "History\t%s · replay %s\n", onOff(v.Capabilities.History), onOff(v.Capabilities.Replay))
	fmt.Fprintf(tw, "Server\t%s\n", shortVersion(v.ServerVersion))
	tw.Flush()
}

// Status writes one line per board, attention first, with what is deployed
// and whether it drifts; board errors and plan-capped workflows follow.
func Status(w io.Writer, v *query.StatusView, now time.Time) {
	if len(v.Boards) == 0 {
		fmt.Fprintln(w, "No boards match.")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  BOARD\tATTENTION\tDEPLOYED\tDRIFT")
		for _, b := range v.Boards {
			fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\n", statusMark(b.Status), boardName(b), b.Attention, deployedSummary(b, now), driftText(b.Drift))
		}
		tw.Flush()
		for _, b := range v.Boards {
			if b.Error != nil {
				fmt.Fprintf(w, "! %s: %s\n", boardName(b), b.Error.Message)
			}
		}
	}
	notMonitored(w, v)
	fmt.Fprintf(w, "Snapshot %s.\n", age(&v.GeneratedAt, now))
}

// Drift writes one line per configured deploy stage.
func Drift(w io.Writer, v *query.StatusView, now time.Time) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "BOARD\tENV\tDEPLOYED\tDRIFT")
	rows := 0
	for _, b := range v.Boards {
		for _, d := range b.Deployments {
			rows++
			deployed := "none in the recent run window"
			if d.Current != nil {
				deployed = fmt.Sprintf("%s (%s)", short(d.Current.SHA), age(d.Current.CompletedAt, now))
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", boardName(b), d.Env, deployed, driftText(d.Drift))
		}
	}
	tw.Flush()
	if rows == 0 {
		fmt.Fprintln(w, "No deploy stages are configured on the matching boards.")
	}
	notMonitored(w, v)
}

func notMonitored(w io.Writer, v *query.StatusView) {
	count := len(v.NotMonitored.Items)
	if t := v.NotMonitored.Truncation; t != nil {
		count += t.Omitted
	}
	if count == 0 {
		return
	}
	plan := "your plan"
	upgrade := ""
	if v.Plan != nil {
		plan = "the " + v.Plan.Label + " plan"
		upgrade = " Upgrade: " + v.Plan.UpgradeURL
	}
	fmt.Fprintf(w, "%d workflow%s not monitored on %s.%s\n", count, plural(count), plan, upgrade)
}

func boardName(b query.Board) string {
	if b.Repo == "" {
		return b.Name
	}
	return b.Repo + " · " + path.Base(b.Workflow)
}

func deployedSummary(b query.Board, now time.Time) string {
	var parts []string
	for _, d := range b.Deployments {
		if d.Current == nil {
			parts = append(parts, d.Env+" —")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s %s", d.Env, short(d.Current.SHA), age(d.Current.CompletedAt, now)))
	}
	if len(parts) == 0 {
		return "no deploy stage"
	}
	return strings.Join(parts, ", ")
}

func driftText(d query.DriftState) string {
	switch d.Kind {
	case "up_to_date":
		return "up to date"
	case "drifting":
		detail := []string{}
		if d.CommitsTotal > 0 {
			detail = append(detail, fmt.Sprintf("%d commit%s behind", d.CommitsTotal, plural(d.CommitsTotal)))
		}
		if d.ChangedPathsTotal > 0 {
			detail = append(detail, fmt.Sprintf("%d file%s changed", d.ChangedPathsTotal, plural(d.ChangedPathsTotal)))
		}
		if len(detail) == 0 {
			return "drifting"
		}
		return "drifting (" + strings.Join(detail, ", ") + ")"
	case "unknown":
		if d.Reason != "" {
			return "drift unknown (" + strings.ReplaceAll(d.Reason, "_", " ") + ")"
		}
		return "drift unknown"
	default: // not_evaluated: absent, never "up to date"
		return "drift not checked"
	}
}

func statusMark(status string) string {
	switch status {
	case "success":
		return "✓"
	case "failure":
		return "✗"
	case "running":
		return "●"
	case "queued":
		return "○"
	default: // skipped, cancelled, notrun, unknown
		return "–"
	}
}

func age(at *time.Time, now time.Time) string {
	if at == nil || at.IsZero() {
		return "time unknown"
	}
	d := now.Sub(*at)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 36*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// shortVersion abbreviates a commit SHA and leaves anything else (a release
// version) whole.
func shortVersion(v string) string {
	if fullSHA.MatchString(v) {
		return v[:7]
	}
	return v
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
