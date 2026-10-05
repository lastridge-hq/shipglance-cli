// Package notice throttles the terminal update notice to once per interval
// (docs/cli-plan.md §4.4): agents read stderr too, and a line on every call
// is noise. The state is one timestamp file in the user's cache directory.
// --json and MCP output carry the notice in every response instead and never
// consult this.
package notice

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Interval is how often a terminal sees the notice.
const Interval = 24 * time.Hour

// Throttle decides when the notice was last shown. Dir "" means no cache
// directory is available: the notice is shown every time (failing open — it
// is informational, and hiding it silently is the worse failure).
type Throttle struct {
	Dir string
	Now func() time.Time
}

func (t Throttle) path() string { return filepath.Join(t.Dir, "update-notice") }

// Due reports whether the notice should be shown now.
func (t Throttle) Due() bool {
	if t.Dir == "" {
		return true
	}
	raw, err := os.ReadFile(t.path())
	if err != nil {
		return true
	}
	last, err := time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
	if err != nil {
		return true
	}
	return t.Now().Sub(last) >= Interval
}

// Shown records that the notice was shown. A failed write is ignored: the
// cost is seeing the notice again sooner.
func (t Throttle) Shown() {
	if t.Dir == "" {
		return
	}
	if err := os.MkdirAll(t.Dir, 0o700); err != nil {
		return
	}
	_ = os.WriteFile(t.path(), []byte(t.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
}
