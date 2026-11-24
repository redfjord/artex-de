package server

import "testing"

func TestParseTargetStripsProsePunctuation(t *testing.T) {
	cases := []struct {
		text string
		host string
	}{
		{`In scope: https://getfailsafe.com, https://app.getfailsafe.com`, "getfailsafe.com"},
		{`https://getfailsafe.com.`, "getfailsafe.com"},
		{`(see https://app.getfailsafe.com/login)`, "app.getfailsafe.com"},
		{`target: https://getfailsafe.com;`, "getfailsafe.com"},
		{`"https://getfailsafe.com"`, "getfailsafe.com"},
		{`https://getfailsafe.com/path,`, "getfailsafe.com"},
	}
	for _, tc := range cases {
		_, host, _, ok := parseTarget(tc.text)
		if !ok {
			t.Fatalf("parseTarget(%q) failed", tc.text)
		}
		if host != tc.host {
			t.Errorf("parseTarget(%q) host = %q, want %q", tc.text, host, tc.host)
		}
	}
}

func TestParseTargetRejectsNonTargets(t *testing.T) {
	if _, _, _, ok := parseTarget("no target here"); ok {
		t.Fatal("parseTarget accepted text without a host")
	}
}
