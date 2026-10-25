package main

import (
	"strings"
	"unicode/utf8"
)

// The station's own spotter only reacts to "Алиса" and strips it from the
// recognised text, so the agent word is the first word of what remains.
var defaultWakeWords = []string{"агент"}

// wakeWords normalises the configured agent words and falls back to the
// default when the config leaves the list empty.
func wakeWords(configured []string) []string {
	out := make([]string, 0, len(configured))
	for _, w := range configured {
		if w = normalizeSpeech(w); w != "" {
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		return defaultWakeWords
	}
	return out
}

func normalizeSpeech(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	return strings.Join(strings.Fields(s), " ")
}

func isBoundary(r rune) bool {
	switch r {
	case ' ', '\t', ',', ':', '-', '—', '.', '!', '?':
		return true
	}
	return false
}

// stripAddress removes a leading "Алиса"/"Яндекс" if the text query path
// kept it. Voice input already has the spotter word removed.
func stripAddress(s string) string {
	for _, p := range []string{"алиса", "яндекс"} {
		if strings.HasPrefix(s, p) {
			rest := strings.TrimLeft(s[len(p):], " \t,:-—.!?")
			if rest != s[len(p):] {
				return rest
			}
		}
	}
	return s
}

// looksLikeWake detects ASR variants of the wake word for diagnostics only.
func looksLikeWake(text string, words []string) bool {
	norm := normalizeSpeech(text)
	for _, p := range words {
		root := []rune(p)
		if len(root) > 4 {
			root = root[:4]
		}
		if strings.Contains(norm, string(root)) {
			return true
		}
	}
	return false
}

// splitWake reports whether the phrase addresses the agent and returns the
// remaining question without the wake word.
func splitWake(text string, words []string) (string, bool) {
	norm := stripAddress(normalizeSpeech(text))
	for _, p := range words {
		if norm == p {
			return "", true
		}
		if !strings.HasPrefix(norm, p) {
			continue
		}
		rest := norm[len(p):]
		r, _ := utf8.DecodeRuneInString(rest)
		if !isBoundary(r) {
			continue
		}
		return strings.TrimSpace(strings.TrimLeft(rest, " \t,:-—.!?")), true
	}
	return "", false
}
