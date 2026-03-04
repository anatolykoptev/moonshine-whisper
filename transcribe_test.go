package main

import (
	"encoding/binary"
	"strings"
	"testing"
)

// --- compressionRatio ---

func TestCompressionRatio_Empty(t *testing.T) {
	if got := compressionRatio(""); got != 0 {
		t.Errorf("compressionRatio empty = %f, want 0", got)
	}
}

func TestCompressionRatio_BelowThreshold(t *testing.T) {
	// 9 chars — below the 10-char minimum.
	if got := compressionRatio("123456789"); got != 0 {
		t.Errorf("compressionRatio 9 chars = %f, want 0", got)
	}
}

func TestCompressionRatio_ExactlyTenChars(t *testing.T) {
	ratio := compressionRatio("1234567890")
	if ratio <= 0 {
		t.Errorf("compressionRatio 10 chars = %f, want > 0", ratio)
	}
}

func TestCompressionRatio_Normal(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog near the river"
	ratio := compressionRatio(text)
	if ratio <= 0 || ratio > 2.4 {
		t.Errorf("compressionRatio normal = %f, want (0, 2.4]", ratio)
	}
}

func TestCompressionRatio_Repetitive(t *testing.T) {
	text := strings.Repeat("aaaa ", 40)
	ratio := compressionRatio(text)
	if ratio <= 2.4 {
		t.Errorf("compressionRatio repetitive = %f, want > 2.4", ratio)
	}
}

func TestCompressionRatio_AllSameChar(t *testing.T) {
	text := strings.Repeat("z", 100)
	ratio := compressionRatio(text)
	if ratio <= 2.4 {
		t.Errorf("compressionRatio all-same = %f, want > 2.4", ratio)
	}
}

func TestCompressionRatio_CyrillicNormal(t *testing.T) {
	text := "Быстрая коричневая лиса перепрыгивает через ленивую собаку"
	ratio := compressionRatio(text)
	if ratio <= 0 {
		t.Errorf("compressionRatio cyrillic = %f, want > 0", ratio)
	}
}

func TestCompressionRatio_Deterministic(t *testing.T) {
	text := "deterministic test with enough characters to pass threshold"
	r1 := compressionRatio(text)
	r2 := compressionRatio(text)
	if r1 != r2 {
		t.Errorf("compressionRatio not deterministic: %f vs %f", r1, r2)
	}
}

// --- parsePCM ---

func TestParsePCM_Mono16(t *testing.T) {
	// Two samples: 0 and max positive.
	data := make([]byte, 4)
	binary.LittleEndian.PutUint16(data[0:2], 0)
	binary.LittleEndian.PutUint16(data[2:4], 0x7FFF) // 32767

	samples, sr, err := parsePCM(data, 1, 16, 16000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sr != 16000 {
		t.Errorf("sampleRate = %d, want 16000", sr)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}
	if samples[0] != 0 {
		t.Errorf("samples[0] = %f, want 0", samples[0])
	}
	// 32767/32768 ≈ 0.99997
	if samples[1] < 0.999 || samples[1] > 1.0 {
		t.Errorf("samples[1] = %f, want ~1.0", samples[1])
	}
}

func TestParsePCM_Mono16_Negative(t *testing.T) {
	// Min negative: -32768 → -1.0.
	data := make([]byte, 2)
	binary.LittleEndian.PutUint16(data[0:2], 0x8000) // -32768 as int16

	samples, _, err := parsePCM(data, 1, 16, 16000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0] != -1.0 {
		t.Errorf("samples[0] = %f, want -1.0", samples[0])
	}
}

func TestParsePCM_Stereo16(t *testing.T) {
	// L=16384 (0.5), R=0 → avg=0.25.
	data := make([]byte, 4)
	binary.LittleEndian.PutUint16(data[0:2], 0x4000) // 16384
	binary.LittleEndian.PutUint16(data[2:4], 0)       // 0

	samples, _, err := parsePCM(data, 2, 16, 16000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	// (16384 + 0) / 2.0 / 32768.0 = 0.25
	if samples[0] < 0.249 || samples[0] > 0.251 {
		t.Errorf("samples[0] = %f, want ~0.25", samples[0])
	}
}

func TestParsePCM_StereoSymmetric(t *testing.T) {
	// L=10000, R=10000 → avg should be same as mono 10000.
	data := make([]byte, 4)
	binary.LittleEndian.PutUint16(data[0:2], uint16(10000))
	binary.LittleEndian.PutUint16(data[2:4], uint16(10000))

	samples, _, err := parsePCM(data, 2, 16, 16000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := float32(10000) / 32768.0
	if samples[0] < expected-0.001 || samples[0] > expected+0.001 {
		t.Errorf("samples[0] = %f, want ~%f", samples[0], expected)
	}
}

func TestParsePCM_EmptyData(t *testing.T) {
	samples, _, err := parsePCM([]byte{}, 1, 16, 16000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 0 {
		t.Errorf("expected 0 samples for empty data, got %d", len(samples))
	}
}

func TestParsePCM_OddBytes(t *testing.T) {
	// 3 bytes for mono 16-bit: only 1 complete sample (2 bytes), last byte ignored.
	data := []byte{0x00, 0x01, 0xFF}
	samples, _, err := parsePCM(data, 1, 16, 16000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 1 {
		t.Errorf("expected 1 sample, got %d", len(samples))
	}
}

func TestParsePCM_UnsupportedFormat(t *testing.T) {
	_, _, err := parsePCM([]byte{0, 0}, 1, 8, 16000)
	if err == nil {
		t.Error("expected error for 8-bit audio")
	}
}

func TestParsePCM_Unsupported24Bit(t *testing.T) {
	_, _, err := parsePCM([]byte{0, 0, 0}, 1, 24, 16000)
	if err == nil {
		t.Error("expected error for 24-bit audio")
	}
}

func TestParsePCM_Unsupported3Channels(t *testing.T) {
	_, _, err := parsePCM([]byte{0, 0, 0, 0, 0, 0}, 3, 16, 16000)
	if err == nil {
		t.Error("expected error for 3-channel audio")
	}
}

func TestParsePCM_SampleRatePassthrough(t *testing.T) {
	data := make([]byte, 2)
	_, sr, err := parsePCM(data, 1, 16, 44100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sr != 44100 {
		t.Errorf("sampleRate = %d, want 44100", sr)
	}
}

// --- ensureWav ---

func TestEnsureWav_AlreadyWav(t *testing.T) {
	wavPath, cleanup, err := ensureWav("/tmp/test.wav")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wavPath != "/tmp/test.wav" {
		t.Errorf("wavPath = %q, want %q", wavPath, "/tmp/test.wav")
	}
	if cleanup != "" {
		t.Errorf("cleanup should be empty for .wav, got %q", cleanup)
	}
}

func TestEnsureWav_UppercaseWav(t *testing.T) {
	wavPath, cleanup, err := ensureWav("/tmp/test.WAV")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wavPath != "/tmp/test.WAV" {
		t.Errorf("wavPath = %q, want passthrough for .WAV", wavPath)
	}
	if cleanup != "" {
		t.Errorf("cleanup should be empty for .WAV")
	}
}

func TestEnsureWav_NonExistentMp3(t *testing.T) {
	// Non-existent file: ffmpeg should fail.
	_, _, err := ensureWav("/tmp/nonexistent_12345.mp3")
	if err == nil {
		t.Error("expected error for non-existent mp3 file")
	}
}
