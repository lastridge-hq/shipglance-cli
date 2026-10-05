package notice

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThrottle(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	dir := filepath.Join(t.TempDir(), "shipglance")
	th := Throttle{Dir: dir, Now: func() time.Time { return now }}
	if !th.Due() {
		t.Fatal("never shown = due")
	}
	th.Shown()
	if th.Due() {
		t.Fatal("just shown = not due")
	}
	now = now.Add(Interval - time.Second)
	if th.Due() {
		t.Fatal("inside the interval = not due")
	}
	now = now.Add(time.Second)
	if !th.Due() {
		t.Fatal("after the interval = due")
	}
	if err := os.WriteFile(filepath.Join(dir, "update-notice"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !th.Due() {
		t.Fatal("a corrupt stamp fails open")
	}
	if !(Throttle{Now: time.Now}).Due() {
		t.Fatal("no cache dir fails open")
	}
}
