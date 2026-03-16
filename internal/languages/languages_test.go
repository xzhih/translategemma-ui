package languages

import (
	"slices"
	"testing"

	"golang.org/x/text/language"
)

func TestLanguageCodeToLanguageName(t *testing.T) {
	tests := []struct {
		name   string
		locale language.Tag
		code   string
		want   string
	}{
		{name: "english label", locale: language.English, code: "fr", want: "French"},
		{name: "localized label", locale: language.SimplifiedChinese, code: "en", want: "英语"},
		{name: "auto detect label", locale: language.Japanese, code: "auto", want: "自動検出"},
		{name: "zh-CN compatibility alias", locale: language.English, code: "zh-CN", want: "Simplified Chinese"},
		{name: "unknown blank", locale: language.English, code: "", want: "Unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LanguageCodeToLanguageName(tc.locale, tc.code); got != tc.want {
				t.Fatalf("LanguageCodeToLanguageName(%q, %q) = %q, want %q", tc.locale, tc.code, got, tc.want)
			}
		})
	}
}

func TestSupportedIncludesLocalizedLabels(t *testing.T) {
	options := Supported()
	if len(options) == 0 {
		t.Fatalf("Supported returned no options")
	}

	var english Option
	found := false
	seenHiddenEnglishVariant := false
	for _, option := range options {
		if option.Code == "en" {
			english = option
			found = true
		}
		if option.Code == "en-AE" && !option.Visible {
			seenHiddenEnglishVariant = true
		}
	}
	if !found {
		t.Fatalf("Supported did not include English")
	}
	if english.Label != "English" {
		t.Fatalf("unexpected English fallback label %q", english.Label)
	}
	if english.Labels["zh-CN"] != "英语" {
		t.Fatalf("unexpected Chinese label %q", english.Labels["zh-CN"])
	}
	if english.Labels["de-DE"] != "Englisch" {
		t.Fatalf("unexpected German label %q", english.Labels["de-DE"])
	}
	if !english.Visible {
		t.Fatalf("expected English to stay visible")
	}
	if !seenHiddenEnglishVariant {
		t.Fatalf("expected a regional English variant to be hidden")
	}
}

func TestWithoutAutoExcludesAutoDetectAndHiddenVariants(t *testing.T) {
	for _, option := range WithoutAuto() {
		if option.Code == autoDetectCode {
			t.Fatalf("WithoutAuto unexpectedly included %q", autoDetectCode)
		}
		if !option.Visible {
			t.Fatalf("WithoutAuto unexpectedly included hidden option %q", option.Code)
		}
	}
}

func TestWithoutAutoSortsCommonLanguagesFirst(t *testing.T) {
	options := WithoutAuto()
	if len(options) < len(commonVisibleLanguageCodes) {
		t.Fatalf("expected enough visible options for common language ordering")
	}

	gotCodes := make([]string, 0, len(commonVisibleLanguageCodes))
	for _, option := range options[:len(commonVisibleLanguageCodes)] {
		gotCodes = append(gotCodes, option.Code)
	}
	if !slices.Equal(gotCodes, commonVisibleLanguageCodes) {
		t.Fatalf("unexpected common language order: got %v want %v", gotCodes, commonVisibleLanguageCodes)
	}
}

func TestWithoutAutoSortsRemainingLanguagesAlphabetically(t *testing.T) {
	options := WithoutAuto()
	if len(options) <= len(commonVisibleLanguageCodes)+1 {
		t.Fatalf("expected additional non-common language options")
	}

	remainder := options[len(commonVisibleLanguageCodes):]
	for i := 1; i < len(remainder); i++ {
		prev := remainder[i-1]
		curr := remainder[i]
		if prev.Label > curr.Label {
			t.Fatalf("expected remaining languages to be sorted alphabetically, got %q before %q", prev.Label, curr.Label)
		}
		if prev.Label == curr.Label && prev.Code > curr.Code {
			t.Fatalf("expected matching labels to be sorted by code, got %q before %q", prev.Code, curr.Code)
		}
	}
}
