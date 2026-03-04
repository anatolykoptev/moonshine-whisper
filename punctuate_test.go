package main

import "testing"

// --- addPunctuation ---

func TestAddPunctuation_NilPunctuator(t *testing.T) {
	// punctuator is nil by default in tests (no model loaded).
	got := addPunctuation("hello world")
	if got != "hello world" {
		t.Errorf("addPunctuation with nil punctuator = %q, want passthrough", got)
	}
}

func TestAddPunctuation_Empty(t *testing.T) {
	got := addPunctuation("")
	if got != "" {
		t.Errorf("addPunctuation empty = %q, want empty", got)
	}
}
