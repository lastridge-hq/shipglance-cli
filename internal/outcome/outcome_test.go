package outcome

import (
	"encoding/json"
	"os"
	"testing"
)

// TestExitCodesArePinned: the exit code table is a contract agents branch on.
func TestExitCodesArePinned(t *testing.T) {
	want := map[Outcome]int{
		OK: 0, Deployed: 0, Failed: 1, Timeout: 2, Transient: 3, Auth: 4, Usage: 5,
		Internal: 6, UpgradeRequired: 7, NotDeployable: 8, PlanLocked: 9,
		Outcome("from-the-future"): 6,
	}
	for outcome, code := range want {
		if got := outcome.ExitCode(); got != code {
			t.Errorf("%s exits %d, want %d", outcome, got, code)
		}
	}
}

// TestEveryServerErrorKindIsKnown reads the frozen kinds from the vendored
// server fixtures: a server kind this CLI cannot map would make it exit 6 on
// an ordinary refusal.
func TestEveryServerErrorKindIsKnown(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/golden/enums.json")
	if err != nil {
		t.Fatal(err)
	}
	var enums map[string][]string
	if err := json.Unmarshal(raw, &enums); err != nil {
		t.Fatal(err)
	}
	if len(enums["errorKind"]) == 0 {
		t.Fatal("enums.json carries no errorKind set")
	}
	for _, kind := range enums["errorKind"] {
		if outcome, ok := ForKind(kind); !ok || outcome == Internal {
			t.Errorf("server error kind %q maps to %s (known=%v)", kind, outcome, ok)
		}
	}
	if _, ok := ForKind("brand_new_kind"); ok {
		t.Fatal("an unknown kind must not be silently known")
	}
}
