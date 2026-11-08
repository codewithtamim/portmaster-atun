package aconn

import (
	"strings"
	"testing"
)

func TestSafeLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Signal", "Signal"},
		{"  Two   words ", "Two words"},
		{"Chat\nProtection disabled, reinstall", "Chat"},
		{"Chat Second line", "Chat"},
		{"Ev‮il​⁦ app\u0007", "Evil app"},
		{"‮\n", ""},
		{"Tab\tbed", "Tab bed"},
		{strings.Repeat("a", 60), strings.Repeat("a", 49) + "…"},
		{strings.Repeat("é", 51), strings.Repeat("é", 49) + "…"},
	}
	for _, c := range cases {
		if got := SafeLabel(c.in); got != c.want {
			t.Errorf("SafeLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNameFallsBackWhenLabelIsEmptyAfterCleaning(t *testing.T) {
	info := &AppInfo{PackageName: "org.example", Label: "‮​"}
	if got := info.Name(); got != "org.example" {
		t.Fatalf("Name() = %q", got)
	}
}

func TestSignedBySameDeveloper(t *testing.T) {
	info := &AppInfo{Signer: "BB", SignerHistory: []string{"aa", "bb"}}
	for known, want := range map[string]bool{"bb": true, "AA": true, "cc": false, "": false} {
		if got := info.SignedBySameDeveloper(known); got != want {
			t.Errorf("SignedBySameDeveloper(%q) = %v, want %v", known, got, want)
		}
	}
	if (*AppInfo)(nil).SignedBySameDeveloper("aa") {
		t.Error("nil info matched")
	}
}
