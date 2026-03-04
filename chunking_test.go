package main

import (
	"strings"
	"testing"
)

// --- splitText ---

func TestSplitText_Short(t *testing.T) {
	got := splitText("hello", 100)
	if len(got) != 1 || got[0] != "hello" {
		t.Fatalf("expected single chunk, got %v", got)
	}
}

func TestSplitText_ExactFit(t *testing.T) {
	text := strings.Repeat("a", 50)
	got := splitText(text, 50)
	if len(got) != 1 || got[0] != text {
		t.Fatalf("expected single chunk for exact fit")
	}
}

func TestSplitText_Empty(t *testing.T) {
	got := splitText("", 100)
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("expected single empty chunk, got %v", got)
	}
}

func TestSplitText_ZeroMaxLen(t *testing.T) {
	got := splitText("hello", 0)
	if len(got) != 1 || got[0] != "hello" {
		t.Fatalf("expected passthrough for maxLen=0, got %v", got)
	}
}

func TestSplitText_NegativeMaxLen(t *testing.T) {
	got := splitText("hello", -5)
	if len(got) != 1 || got[0] != "hello" {
		t.Fatalf("expected passthrough for negative maxLen, got %v", got)
	}
}

func TestSplitText_MaxLenOne(t *testing.T) {
	got := splitText("abc", 1)
	if len(got) != 3 {
		t.Fatalf("expected 3 single-char chunks, got %d: %v", len(got), got)
	}
	for i, ch := range got {
		if len(ch) != 1 {
			t.Errorf("chunk[%d] = %q, want single char", i, ch)
		}
	}
}

func TestSplitText_NewlineBoundary(t *testing.T) {
	text := "first line\nsecond line"
	got := splitText(text, 15)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(got), got)
	}
	if got[0] != "first line" {
		t.Errorf("chunk[0] = %q, want %q", got[0], "first line")
	}
	if got[1] != "second line" {
		t.Errorf("chunk[1] = %q, want %q", got[1], "second line")
	}
}

func TestSplitText_ConsecutiveNewlines(t *testing.T) {
	text := "aaa\n\n\nbbb"
	got := splitText(text, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(got), got)
	}
	if got[0] != "aaa" {
		t.Errorf("chunk[0] = %q, want %q", got[0], "aaa")
	}
	if got[1] != "bbb" {
		t.Errorf("chunk[1] = %q, want %q", got[1], "bbb")
	}
}

func TestSplitText_SentenceBoundary(t *testing.T) {
	text := "Hello world. Goodbye world."
	got := splitText(text, 20)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(got), got)
	}
	if got[0] != "Hello world." {
		t.Errorf("chunk[0] = %q, want %q", got[0], "Hello world.")
	}
	if got[1] != "Goodbye world." {
		t.Errorf("chunk[1] = %q, want %q", got[1], "Goodbye world.")
	}
}

func TestSplitText_SentenceNoPrecedingSpace(t *testing.T) {
	// Period without space = not a sentence boundary, should fall to word/hard split.
	text := "abc.def ghi"
	got := splitText(text, 8)
	if len(got) < 2 {
		t.Fatalf("expected 2+ chunks, got %d: %v", len(got), got)
	}
	// Should split at the space, not at the period.
	if got[0] != "abc.def" {
		t.Errorf("chunk[0] = %q, want %q", got[0], "abc.def")
	}
}

func TestSplitText_QuestionExclamation(t *testing.T) {
	text := "Is it good? Yes! Very much."
	got := splitText(text, 15)
	if len(got) < 2 {
		t.Fatalf("expected 2+ chunks, got %d: %v", len(got), got)
	}
}

func TestSplitText_WordBoundary(t *testing.T) {
	text := "aaaa bbbb cccc"
	got := splitText(text, 10)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(got), got)
	}
	if got[0] != "aaaa bbbb" {
		t.Errorf("chunk[0] = %q, want %q", got[0], "aaaa bbbb")
	}
}

func TestSplitText_HardCut(t *testing.T) {
	text := strings.Repeat("x", 20)
	got := splitText(text, 10)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(got), got)
	}
	if len(got[0]) != 10 {
		t.Errorf("chunk[0] len = %d, want 10", len(got[0]))
	}
}

func TestSplitText_MultipleChunks(t *testing.T) {
	text := "One. Two. Three. Four. Five."
	got := splitText(text, 12)
	if len(got) < 3 {
		t.Fatalf("expected 3+ chunks, got %d: %v", len(got), got)
	}
	joined := strings.Join(got, " ")
	if joined != text {
		t.Errorf("rejoined = %q, want %q", joined, text)
	}
}

func TestSplitText_OnlyWhitespace(t *testing.T) {
	got := splitText("     ", 3)
	// After TrimSpace, empty chunks should collapse.
	for _, ch := range got {
		if strings.TrimSpace(ch) != "" {
			t.Errorf("expected empty/whitespace chunk, got %q", ch)
		}
	}
}

func TestSplitText_CyrillicMultibyte(t *testing.T) {
	// Cyrillic chars are 2 bytes each in UTF-8.
	text := "Привет мир. Как дела?"
	got := splitText(text, 22) // "Привет мир." = 20 bytes
	if len(got) < 2 {
		t.Fatalf("expected 2+ chunks for cyrillic, got %d: %v", len(got), got)
	}
}

func TestSplitText_Emoji(t *testing.T) {
	text := "Hello! Nice day."
	got := splitText(text, 10)
	if len(got) < 2 {
		t.Fatalf("expected 2+ chunks with emoji, got %d: %v", len(got), got)
	}
}

func TestSplitText_OneLessThanLen(t *testing.T) {
	text := "abcde"
	got := splitText(text, 4)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(got), got)
	}
}

func TestSplitText_NewlinePreferredOverSentence(t *testing.T) {
	// When both newline and sentence boundary exist, newline wins.
	text := "Hello. World\nBye."
	got := splitText(text, 15)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(got), got)
	}
	if got[0] != "Hello. World" {
		t.Errorf("chunk[0] = %q, want %q (split at newline, not sentence)", got[0], "Hello. World")
	}
}

func TestSplitText_LongSingleWord(t *testing.T) {
	word := strings.Repeat("a", 100)
	got := splitText(word, 30)
	if len(got) < 3 {
		t.Fatalf("expected 3+ chunks for 100-char word at maxLen=30, got %d", len(got))
	}
	total := 0
	for _, ch := range got {
		total += len(ch)
	}
	if total != 100 {
		t.Errorf("total chars = %d, want 100", total)
	}
}

func TestSplitText_TabsNotTreatedAsNewline(t *testing.T) {
	text := "hello\tworld\tthere"
	got := splitText(text, 12)
	// Tabs are not split boundaries (not \n), should split at word boundary if needed.
	if len(got) < 1 {
		t.Fatalf("expected at least 1 chunk, got %v", got)
	}
}

// --- findSplitPoint ---

func TestFindSplitPoint_Newline(t *testing.T) {
	if idx := findSplitPoint("hello\nworld"); idx != 5 {
		t.Errorf("findSplitPoint = %d, want 5", idx)
	}
}

func TestFindSplitPoint_NewlineAtStart(t *testing.T) {
	// \n at index 0 means idx=0 which is <= 0, should fall through.
	idx := findSplitPoint("\nhello")
	// Should not split at 0, should try sentence/word/hard.
	if idx == 0 {
		t.Errorf("findSplitPoint should not split at position 0")
	}
}

func TestFindSplitPoint_MultipleNewlines(t *testing.T) {
	// Should pick the LAST newline.
	idx := findSplitPoint("aa\nbb\ncc")
	if idx != 5 {
		t.Errorf("findSplitPoint = %d, want 5 (last newline)", idx)
	}
}

func TestFindSplitPoint_Sentence(t *testing.T) {
	idx := findSplitPoint("Hello world. Bye")
	if idx != 12 {
		t.Errorf("findSplitPoint = %d, want 12", idx)
	}
}

func TestFindSplitPoint_QuestionMark(t *testing.T) {
	idx := findSplitPoint("Is it ok? Yes")
	if idx != 9 {
		t.Errorf("findSplitPoint = %d, want 9", idx)
	}
}

func TestFindSplitPoint_ExclamationMark(t *testing.T) {
	idx := findSplitPoint("Wow! Amazing")
	if idx != 4 {
		t.Errorf("findSplitPoint = %d, want 4", idx)
	}
}

func TestFindSplitPoint_PeriodNoSpace(t *testing.T) {
	// "abc.def" — period without trailing space is NOT a sentence boundary.
	idx := findSplitPoint("abc.def")
	// Should fall to hard cut (no space, no newline, no sentence boundary).
	if idx != 7 {
		t.Errorf("findSplitPoint = %d, want 7 (hard cut)", idx)
	}
}

func TestFindSplitPoint_Space(t *testing.T) {
	if idx := findSplitPoint("hello world"); idx != 5 {
		t.Errorf("findSplitPoint = %d, want 5", idx)
	}
}

func TestFindSplitPoint_NoBoundary(t *testing.T) {
	if idx := findSplitPoint("helloworld"); idx != 10 {
		t.Errorf("findSplitPoint = %d, want 10 (hard cut)", idx)
	}
}

func TestFindSplitPoint_SingleChar(t *testing.T) {
	if idx := findSplitPoint("x"); idx != 1 {
		t.Errorf("findSplitPoint = %d, want 1", idx)
	}
}

func TestFindSplitPoint_AllSpaces(t *testing.T) {
	idx := findSplitPoint("     ")
	// Last space at position 4.
	if idx != 4 {
		t.Errorf("findSplitPoint = %d, want 4", idx)
	}
}

func TestFindSplitPoint_MultipleSentences(t *testing.T) {
	// Should pick the LAST sentence boundary.
	idx := findSplitPoint("A. B. C")
	if idx != 5 {
		t.Errorf("findSplitPoint = %d, want 5 (last '. ')", idx)
	}
}

// --- sanitizeUTF8 ---

func TestSanitizeUTF8_Valid(t *testing.T) {
	if got := sanitizeUTF8("hello world"); got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestSanitizeUTF8_Empty(t *testing.T) {
	if got := sanitizeUTF8(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestSanitizeUTF8_NullBytes(t *testing.T) {
	if got := sanitizeUTF8("hello\x00world"); got != "helloworld" {
		t.Errorf("got %q, want %q", got, "helloworld")
	}
}

func TestSanitizeUTF8_MultipleNullBytes(t *testing.T) {
	if got := sanitizeUTF8("\x00\x00\x00"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestSanitizeUTF8_OnlyInvalidBytes(t *testing.T) {
	if got := sanitizeUTF8("\xff\xfe\xfd"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestSanitizeUTF8_InvalidBytes(t *testing.T) {
	if got := sanitizeUTF8("hello\xff\xfeworld"); got != "helloworld" {
		t.Errorf("got %q, want %q", got, "helloworld")
	}
}

func TestSanitizeUTF8_Cyrillic(t *testing.T) {
	if got := sanitizeUTF8("Привет мир"); got != "Привет мир" {
		t.Errorf("got %q, want %q", got, "Привет мир")
	}
}

func TestSanitizeUTF8_Emoji(t *testing.T) {
	if got := sanitizeUTF8("hello 🌍 world"); got != "hello 🌍 world" {
		t.Errorf("got %q, want %q", got, "hello 🌍 world")
	}
}

func TestSanitizeUTF8_MixedInvalidAndNull(t *testing.T) {
	got := sanitizeUTF8("ok\xff\x00good\xfe")
	if got != "okgood" {
		t.Errorf("got %q, want %q", got, "okgood")
	}
}

func TestSanitizeUTF8_SurrogatePairs(t *testing.T) {
	// UTF-8 encoded surrogates (U+D800) are invalid and should be stripped.
	got := sanitizeUTF8("a\xed\xa0\x80b")
	if got != "ab" {
		t.Errorf("got %q, want %q", got, "ab")
	}
}

func TestSanitizeUTF8_TruncatedMultibyte(t *testing.T) {
	// First byte of a 2-byte sequence without continuation.
	got := sanitizeUTF8("abc\xc3")
	if got != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}
