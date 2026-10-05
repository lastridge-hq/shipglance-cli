package auth

import (
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	token := "sgt_acme-co_" + strings.Repeat("a", 64)
	got, err := Resolve(" "+token+"\n", "")
	if err != nil || got.BaseURL != "https://acme-co.shipglance.com" || got.Workspace != "acme-co" || got.Token != token {
		t.Fatalf("Resolve = %+v, %v", got, err)
	}
	local, err := Resolve(token, "http://acme.localhost:8080/")
	if err != nil || local.BaseURL != "http://acme.localhost:8080" || local.Token != token {
		t.Fatalf("override = %+v, %v", local, err)
	}
	for name, tc := range map[string][2]string{
		"missing":   {"", ""},
		"github":    {"ghp_abc", ""},
		"truncated": {token[:40], ""},
		"bad url":   {token, "acme.shipglance.com"},
	} {
		_, err := Resolve(tc[0], tc[1])
		problem, ok := AsProblem(err)
		if !ok || problem.Message == "" {
			t.Errorf("%s: want a problem, got %v", name, err)
		}
	}
	if _, err := Resolve("", ""); err == nil || !strings.Contains(err.Error(), "Settings → API tokens") {
		t.Fatalf("a missing token says where tokens come from: %v", err)
	}
}
