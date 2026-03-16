package translate

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type textSegment struct {
	Text      string
	Translate bool
}

var paragraphSeparatorPattern = regexp.MustCompile(`\n(?:[ \t]*\n)+`)

func segmentForTranslation(text string, maxTokens int, countTokens func(string) int) []textSegment {
	if text == "" {
		return nil
	}
	if strings.TrimSpace(text) == "" {
		return []textSegment{{Text: text}}
	}
	if countTokens(text) <= maxTokens {
		return []textSegment{{Text: text, Translate: true}}
	}

	if parts := splitParagraphSegments(text); len(parts) > 1 {
		return segmentNested(parts, maxTokens, countTokens)
	}
	if parts := splitSentenceSegments(text); len(parts) > 1 {
		return segmentNested(parts, maxTokens, countTokens)
	}
	if parts := splitHardSegment(text); len(parts) > 1 {
		return segmentNested(parts, maxTokens, countTokens)
	}

	return []textSegment{{Text: text, Translate: true}}
}

func segmentNested(parts []textSegment, maxTokens int, countTokens func(string) int) []textSegment {
	var out []textSegment
	for _, part := range parts {
		if !part.Translate || strings.TrimSpace(part.Text) == "" {
			out = append(out, textSegment{Text: part.Text})
			continue
		}
		out = append(out, segmentForTranslation(part.Text, maxTokens, countTokens)...)
	}
	return mergeAdjacentSegments(out)
}

func splitParagraphSegments(text string) []textSegment {
	matches := paragraphSeparatorPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}

	var out []textSegment
	cursor := 0
	for _, match := range matches {
		if match[0] > cursor {
			out = append(out, textSegment{Text: text[cursor:match[0]], Translate: true})
		}
		out = append(out, textSegment{Text: text[match[0]:match[1]]})
		cursor = match[1]
	}
	if cursor < len(text) {
		out = append(out, textSegment{Text: text[cursor:], Translate: true})
	}
	return mergeAdjacentSegments(out)
}

func splitSentenceSegments(text string) []textSegment {
	var out []textSegment
	last := 0

	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 0 {
			break
		}

		switch {
		case isLineSeparator(r):
			if i > last {
				out = append(out, textSegment{Text: text[last:i], Translate: true})
			}
			j := i + size
			for j < len(text) {
				next, step := utf8.DecodeRuneInString(text[j:])
				if !isLineSeparator(next) && !isHorizontalWhitespace(next) {
					break
				}
				j += step
			}
			out = append(out, textSegment{Text: text[i:j]})
			last = j
			i = j
			continue
		case isSentenceTerminator(r) && !isDecimalPoint(text, i, size, r):
			end := i + size
			for end < len(text) {
				next, step := utf8.DecodeRuneInString(text[end:])
				if !isSentenceCloser(next) {
					break
				}
				end += step
			}
			if end > last {
				out = append(out, textSegment{Text: text[last:end], Translate: true})
			}
			sepStart := end
			for end < len(text) {
				next, step := utf8.DecodeRuneInString(text[end:])
				if !unicode.IsSpace(next) {
					break
				}
				end += step
			}
			if end > sepStart {
				out = append(out, textSegment{Text: text[sepStart:end]})
			}
			last = end
			i = end
			continue
		}

		i += size
	}

	if last < len(text) {
		out = append(out, textSegment{Text: text[last:], Translate: true})
	}
	if len(out) <= 1 {
		return nil
	}
	return mergeAdjacentSegments(out)
}

func splitHardSegment(text string) []textSegment {
	runes := []rune(text)
	if len(runes) < 2 {
		return nil
	}

	mid := len(runes) / 2
	if boundaryStart, boundaryEnd, ok := nearestWhitespaceBoundary(runes, mid); ok {
		out := make([]textSegment, 0, 3)
		if boundaryStart > 0 {
			out = append(out, textSegment{Text: string(runes[:boundaryStart]), Translate: true})
		}
		if boundaryEnd > boundaryStart {
			out = append(out, textSegment{Text: string(runes[boundaryStart:boundaryEnd])})
		}
		if boundaryEnd < len(runes) {
			out = append(out, textSegment{Text: string(runes[boundaryEnd:]), Translate: true})
		}
		if len(out) > 1 {
			return mergeAdjacentSegments(out)
		}
	}

	return []textSegment{
		{Text: string(runes[:mid]), Translate: true},
		{Text: string(runes[mid:]), Translate: true},
	}
}

func splitRetrySegment(text string) []textSegment {
	return splitHardSegment(text)
}

func mergeAdjacentSegments(parts []textSegment) []textSegment {
	if len(parts) == 0 {
		return nil
	}

	out := make([]textSegment, 0, len(parts))
	for _, part := range parts {
		if part.Text == "" {
			continue
		}
		if len(out) == 0 || out[len(out)-1].Translate != part.Translate {
			out = append(out, part)
			continue
		}
		out[len(out)-1].Text += part.Text
	}
	return out
}

func nearestWhitespaceBoundary(runes []rune, mid int) (int, int, bool) {
	for offset := 0; offset < len(runes); offset++ {
		left := mid - offset
		if left >= 0 && unicode.IsSpace(runes[left]) {
			return expandWhitespaceRun(runes, left)
		}
		right := mid + offset
		if right < len(runes) && unicode.IsSpace(runes[right]) {
			return expandWhitespaceRun(runes, right)
		}
	}
	return 0, 0, false
}

func expandWhitespaceRun(runes []rune, idx int) (int, int, bool) {
	if idx < 0 || idx >= len(runes) || !unicode.IsSpace(runes[idx]) {
		return 0, 0, false
	}
	start := idx
	for start > 0 && unicode.IsSpace(runes[start-1]) {
		start--
	}
	end := idx + 1
	for end < len(runes) && unicode.IsSpace(runes[end]) {
		end++
	}
	return start, end, start != end
}

func isLineSeparator(r rune) bool {
	return r == '\n' || r == '\r'
}

func isHorizontalWhitespace(r rune) bool {
	return r == ' ' || r == '\t'
}

func isSentenceTerminator(r rune) bool {
	switch r {
	case '.', '?', '!', '。', '？', '！', ';', '；':
		return true
	default:
		return false
	}
}

func isSentenceCloser(r rune) bool {
	switch r {
	case '"', '\'', ')', ']', '}', '”', '’', '»', '》', '】':
		return true
	default:
		return false
	}
}

func isDecimalPoint(text string, index, size int, r rune) bool {
	if r != '.' {
		return false
	}
	prev, okPrev := previousRune(text, index)
	next, okNext := nextRune(text, index+size)
	return okPrev && okNext && unicode.IsDigit(prev) && unicode.IsDigit(next)
}

func previousRune(text string, end int) (rune, bool) {
	if end <= 0 {
		return 0, false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:end])
	if r == utf8.RuneError {
		return 0, false
	}
	return r, true
}

func nextRune(text string, start int) (rune, bool) {
	if start >= len(text) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(text[start:])
	if r == utf8.RuneError {
		return 0, false
	}
	return r, true
}
