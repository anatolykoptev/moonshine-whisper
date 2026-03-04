package main

import "strings"

// splitText splits text into chunks of at most maxLen bytes.
// Tries boundaries in order: newline → sentence end → word boundary → hard cut.
// Ported from Vaelor's splitMessage, extended for raw STT output.
func splitText(text string, maxLen int) []string {
	if maxLen <= 0 || len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}

		chunk := text[:maxLen]
		splitAt := findSplitPoint(chunk)

		chunks = append(chunks, strings.TrimSpace(text[:splitAt]))
		text = strings.TrimSpace(text[splitAt:])
	}

	return chunks
}

// findSplitPoint returns the best position to split within chunk.
// Priority: newline > sentence end (. ! ?) > word boundary (space) > len.
func findSplitPoint(chunk string) int {
	// 1. Try newline
	if idx := strings.LastIndex(chunk, "\n"); idx > 0 {
		return idx
	}

	// 2. Try sentence boundary (". " or "! " or "? ")
	for _, sep := range []string{". ", "! ", "? "} {
		if idx := strings.LastIndex(chunk, sep); idx > 0 {
			return idx + len(sep) - 1 // include the punctuation, split before space
		}
	}

	// 3. Try word boundary
	if idx := strings.LastIndex(chunk, " "); idx > 0 {
		return idx
	}

	// 4. Hard cut
	return len(chunk)
}

// sanitizeUTF8 ensures text is valid UTF-8 and strips null bytes.
// Ported from Vaelor's channels.sanitizeUTF8.
func sanitizeUTF8(text string) string {
	text = strings.ToValidUTF8(text, "")
	text = strings.ReplaceAll(text, "\x00", "")
	return text
}
