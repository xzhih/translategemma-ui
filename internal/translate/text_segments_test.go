package translate

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSegmentForTranslationPreservesParagraphSeparators(t *testing.T) {
	text := "Alpha.\n\nBeta gamma delta."
	segments := segmentForTranslation(text, 8, func(input string) int {
		return utf8.RuneCountInString(input)
	})

	if len(segments) < 3 {
		t.Fatalf("expected split segments for long text, got %#v", segments)
	}

	var rebuilt strings.Builder
	foundSeparator := false
	for _, segment := range segments {
		rebuilt.WriteString(segment.Text)
		if segment.Text == "\n\n" && !segment.Translate {
			foundSeparator = true
		}
	}

	if rebuilt.String() != text {
		t.Fatalf("segments did not preserve original text structure:\nwant: %q\ngot:  %q", text, rebuilt.String())
	}
	if !foundSeparator {
		t.Fatalf("expected paragraph separator to remain as a non-translated segment: %#v", segments)
	}
}
