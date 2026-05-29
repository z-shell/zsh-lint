package diag

import "testing"

func TestSeverityString(t *testing.T) {
	cases := []struct {
		sev  Severity
		want string
	}{
		{Error, "error"},
		{Warning, "warning"},
		{Info, "info"},
		{Hint, "hint"},
	}
	for _, c := range cases {
		if got := c.sev.String(); got != c.want {
			t.Errorf("Severity(%d).String() = %q; want %q", int(c.sev), got, c.want)
		}
	}
}

func TestSeverityStringOutOfRange(t *testing.T) {
	got := Severity(99).String()
	want := "severity(99)"
	if got != want {
		t.Errorf("out-of-range String() = %q; want %q", got, want)
	}
}

func TestParseSeverityRoundTrip(t *testing.T) {
	for _, sev := range []Severity{Error, Warning, Info, Hint} {
		got, err := ParseSeverity(sev.String())
		if err != nil {
			t.Fatalf("ParseSeverity(%q) unexpected error: %v", sev.String(), err)
		}
		if got != sev {
			t.Errorf("ParseSeverity(%q) = %d; want %d", sev.String(), int(got), int(sev))
		}
	}
}

func TestParseSeverityUnknown(t *testing.T) {
	_, err := ParseSeverity("ERROR") // uppercase is not canonical
	if err == nil {
		t.Fatal("ParseSeverity(\"ERROR\") expected error, got nil")
	}
	if got := err.Error(); got[:5] != "diag:" {
		t.Errorf("error should be prefixed \"diag:\"; got %q", got)
	}
}
