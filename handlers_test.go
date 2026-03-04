package main

import "testing"

// --- normLang ---

func TestNormLang(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "en"},
		{"en", "en"},
		{"ru", "ru"},
		{"RU", "ru"},
		{"EN", "en"},
		{"  en  ", "en"},
		{"  RU  ", "ru"},
		{"de", "de"},
		{"JA", "ja"},
		{"\t en \n", "en"},
	}
	for _, tt := range tests {
		if got := normLang(tt.input); got != tt.want {
			t.Errorf("normLang(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- parseBoolPtr ---

func TestParseBoolPtr_True(t *testing.T) {
	for _, s := range []string{"true", "1", "yes", "TRUE", "Yes", "YES", "True"} {
		got := parseBoolPtr(s)
		if got == nil || !*got {
			t.Errorf("parseBoolPtr(%q) should be true", s)
		}
	}
}

func TestParseBoolPtr_False(t *testing.T) {
	for _, s := range []string{"false", "0", "no", "FALSE", "No", "NO", "False"} {
		got := parseBoolPtr(s)
		if got == nil || *got {
			t.Errorf("parseBoolPtr(%q) should be false", s)
		}
	}
}

func TestParseBoolPtr_Nil(t *testing.T) {
	for _, s := range []string{"", "maybe", "auto", "2", "-1", "tru", "ye", "nah"} {
		got := parseBoolPtr(s)
		if got != nil {
			t.Errorf("parseBoolPtr(%q) should be nil, got %v", s, *got)
		}
	}
}

func TestParseBoolPtr_WithSpaces(t *testing.T) {
	got := parseBoolPtr("  true  ")
	if got == nil || !*got {
		t.Error("parseBoolPtr with leading/trailing spaces should parse true")
	}

	got = parseBoolPtr("  false  ")
	if got == nil || *got {
		t.Error("parseBoolPtr with leading/trailing spaces should parse false")
	}
}
