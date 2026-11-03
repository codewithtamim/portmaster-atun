//go:build android

package profile

import "testing"

func TestAndroidUserIdentity(t *testing.T) {
	cases := []struct {
		in   string
		id   string
		user int
	}{
		{"com.whatsapp", "com.whatsapp", 0},
		{"com.whatsapp@10", "com.whatsapp", 10},
		{"com.whatsapp@999", "com.whatsapp", 999},
		{"odd@name", "odd@name", 0},
		{"@5", "@5", 0},
	}
	for _, c := range cases {
		id, user := SplitAndroidUserIdentity(c.in)
		if id != c.id || user != c.user {
			t.Errorf("Split(%q) = %q, %d; want %q, %d", c.in, id, user, c.id, c.user)
		}
		if c.user > 0 && AndroidUserIdentity(id, user) != c.in {
			t.Errorf("round trip of %q failed", c.in)
		}
	}
	if AndroidUserIdentity("com.whatsapp", 0) != "com.whatsapp" {
		t.Error("user 0 must keep the bare package")
	}
}
