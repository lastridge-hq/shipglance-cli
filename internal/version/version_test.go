package version

import "testing"

func TestParseAndLess(t *testing.T) {
	for _, raw := range []string{"dev", "", "1.2", "01.2.3", "1.2.3-"} {
		if _, ok := Parse(raw); ok {
			t.Errorf("Parse(%q) accepted", raw)
		}
	}
	ordered := []string{"0.9.0", "1.0.0-rc.1", "1.0.0", "v1.0.1", "1.10.0"}
	for i := 0; i+1 < len(ordered); i++ {
		a, _ := Parse(ordered[i])
		b, _ := Parse(ordered[i+1])
		if !a.Less(b) || b.Less(a) || a.Less(a) {
			t.Errorf("want %s < %s", ordered[i], ordered[i+1])
		}
	}
}
