// Package tokencount provides a fast, model-agnostic *approximation* of
// token counts. It is intentionally not a real BPE tokenizer: the gateway
// uses it only to pre-reserve TPM quota before a call (see
// internal/adapter/ratelimit/redis) and reconciles against the provider's
// reported usage afterwards, so exactness is not required — speed and a
// conservative (slightly high) estimate are.
package tokencount

import "unicode"

// Estimate returns an approximate token count for s. CJK characters are
// counted ~1 token each (they tend to map close to 1:1 in real tokenizers);
// other text is approximated at ~4 bytes per token, matching common
// English-centric BPE vocabularies.
func Estimate(s string) int {
	if s == "" {
		return 0
	}

	var cjkChars, otherBytes int
	for _, r := range s {
		if isCJK(r) {
			cjkChars++
		} else {
			otherBytes += utf8Len(r)
		}
	}

	tokens := cjkChars + (otherBytes+3)/4
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

func isCJK(r rune) bool {
	switch {
	case unicode.Is(unicode.Han, r),
		unicode.Is(unicode.Hiragana, r),
		unicode.Is(unicode.Katakana, r),
		unicode.Is(unicode.Hangul, r):
		return true
	default:
		return false
	}
}

func utf8Len(r rune) int {
	switch {
	case r < 0x80:
		return 1
	case r < 0x800:
		return 2
	case r < 0x10000:
		return 3
	default:
		return 4
	}
}
