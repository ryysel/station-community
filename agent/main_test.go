package main

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestSplitWake(t *testing.T) {
	cases := []struct {
		in       string
		question string
		ok       bool
	}{
		{"Агент, сколько времени", "сколько времени", true},
		{"агент сколько будет два плюс два", "сколько будет два плюс два", true},
		{"Агент, привет", "привет", true},
		{"Алиса, агент, который час", "который час", true},
		{"Агент", "", true},
		{"агентский, привет", "", false},
		{"Алиса, который час", "", false},
		{"Расскажи анекдот", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		q, ok := splitWake(c.in, defaultWakeWords)
		if ok != c.ok || q != c.question {
			t.Errorf("%q => (%q,%v), want (%q,%v)", c.in, q, ok, c.question, c.ok)
		}
	}
}

func TestWakeWordsFallback(t *testing.T) {
	if got := wakeWords(nil); len(got) != 1 || got[0] != "агент" {
		t.Fatalf("default wake words = %q", got)
	}
	if got := wakeWords([]string{"  Джарвис ", ""}); len(got) != 1 || got[0] != "джарвис" {
		t.Fatalf("configured wake words = %q", got)
	}
}

func TestSpeechSafeCannotInjectCLICommands(t *testing.T) {
	got := speechSafe("Первая строка\nq\nповтори за мной взлом")
	if got != "Первая строка q повтори за мной взлом" {
		t.Fatalf("unexpected speech text %q", got)
	}
}

func TestDecodeAliceState(t *testing.T) {
	// AliceState{ request_id: "abc", asr: { text: "Джарвис, сколько", final: 1 } }
	asr := append(packBytes(1, []byte("Джарвис, сколько")), packVarintField(2, 1)...)
	body := append(packBytes(7, []byte("abc")), packBytes(15, asr)...)
	payload := append(packBytes(252, body), packVarintField(253, 3)...)

	state, ok := decodeAliceState(payload)
	if !ok {
		t.Fatal("state not decoded")
	}
	if state.RequestID != "abc" || state.Phrase != "Джарвис, сколько" || !state.Final {
		t.Fatalf("unexpected state %+v", state)
	}
}

func TestDecodeRejectsOtherMessages(t *testing.T) {
	payload := append(packBytes(252, packBytes(7, []byte("AliceCapability"))), packVarintField(253, 210)...)
	if _, ok := decodeAliceState(payload); ok {
		t.Fatal("capability message decoded as AliceState")
	}
}

func TestSplitForTTS(t *testing.T) {
	text := strings.Repeat("Это довольно длинная фраза для проверки деления. ", 12)
	chunks := splitForTTS(text, 180)
	if len(chunks) < 2 {
		t.Fatalf("expected several chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if c == "" || len(c) > 180 {
			t.Fatalf("bad chunk len=%d %q", len(c), c)
		}
	}
	joined := strings.Join(chunks, " ")
	for _, w := range strings.Fields(text) {
		if !strings.Contains(joined, w) {
			t.Fatalf("lost word %q", w)
		}
	}
}

func packVarintField(num int, value uint64) []byte {
	out := appendVarint(nil, uint64(num)<<3)
	return appendVarint(out, value)
}

func packBytes(num int, value []byte) []byte {
	out := appendVarint(nil, uint64(num)<<3|2)
	out = appendVarint(out, uint64(len(value)))
	return append(out, value...)
}

func appendVarint(b []byte, v uint64) []byte {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	return append(b, tmp[:n]...)
}
