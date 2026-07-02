package render

import "testing"

func TestParseGECOSName(t *testing.T) {
	tests := []struct {
		gecos, want string
	}{
		{"Doe, Patrick W. (Pat)", "Pat Johnson"},
		{"Doe, John", "John Doe"},
		{"Doe, John M.", "John Doe"},
		{"Doe, John,Room 202,555-1234", "John Doe"},
		{"Smith, Jane (Janey),Building A,x1234", "Janey Smith"},
		{"Jane Doe", "Jane Doe"},
		{"Jane Doe,Room 3,555-9999", "Jane Doe"},
		{"", ""},
		{"root", "root"},
		{"O'Brien, Patrick", "Patrick O'Brien"},
		{"Maria del Carmen Rodriguez", "Maria del Carmen Rodriguez"},
		{"To run the daily model in cron", ""},
		{"Service account for foo bar baz quux", ""},
	}
	for _, tt := range tests {
		got := ParseGECOSName(tt.gecos)
		if got != tt.want {
			t.Errorf("ParseGECOSName(%q) = %q, want %q", tt.gecos, got, tt.want)
		}
	}
}

func TestLookupNameFallback(t *testing.T) {
	// Cache off → lanid returned untouched.
	SetNames(false)
	SetNameResolver(map[string]string{"m1": "Alice"})
	if got := LookupName("m1"); got != "m1" {
		t.Errorf("names disabled: LookupName = %q, want %q", got, "m1")
	}
	// Cache on + hit → name returned.
	SetNames(true)
	if got := LookupName("m1"); got != "Alice" {
		t.Errorf("names enabled + hit: LookupName = %q, want %q", got, "Alice")
	}
	// Cache on + miss → lanid returned.
	if got := LookupName("m9"); got != "m9" {
		t.Errorf("names enabled + miss: LookupName = %q, want %q", got, "m9")
	}
	// LookupNameFull adds parens on hit.
	if got := LookupNameFull("m1"); got != "Alice (m1)" {
		t.Errorf("LookupNameFull hit: got %q, want %q", got, "Alice (m1)")
	}
	if got := LookupNameFull("m9"); got != "m9" {
		t.Errorf("LookupNameFull miss: got %q, want %q", got, "m9")
	}
	// Reset for other tests.
	SetNames(false)
	SetNameResolver(nil)
}
