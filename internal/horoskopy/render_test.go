package horoskopy

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	t.Parallel()

	horoscope := Horoscope{
		Title: "Ryby dnes",
		URL:   "https://www.horoskopy.cz/clanek/horoskop-znameni-zverokruhu-ryby-dnes-546",
		Sections: []Section{
			{Text: "Dnešek vás upozorní na napětí."},
			{Heading: "Láska a přátelství", Text: "Nepříznivý den pro vaše znamení."},
			{Heading: "Peníze a práce", Text: "Práce se začne zrychlovat."},
		},
	}

	want := strings.Join([]string{
		"Ryby dnes",
		"=========",
		"",
		"Dnešek vás upozorní na napětí.",
		"",
		"=> Láska a přátelství",
		"Nepříznivý den pro vaše znamení.",
		"",
		"=> Peníze a práce",
		"Práce se začne zrychlovat.",
		"",
		"https://www.horoskopy.cz/clanek/horoskop-znameni-zverokruhu-ryby-dnes-546",
		"",
	}, "\n")

	if got := Render(horoscope); got != want {
		t.Errorf("Render() =\n%q\nwant\n%q", got, want)
	}
}

// TestRenderUnderlinesByRuneCount guards against underlining a title by its
// byte length, which would overshoot for Czech titles carrying diacritics.
func TestRenderUnderlinesByRuneCount(t *testing.T) {
	t.Parallel()

	horoscope := Horoscope{
		Title:    "Býk dnes", // 8 runes, 9 bytes
		URL:      "https://example.test/",
		Sections: []Section{{Text: "text"}},
	}

	lines := strings.Split(Render(horoscope), "\n")
	if len(lines) < 2 {
		t.Fatalf("Render() produced too few lines: %q", lines)
	}
	if want := "========"; lines[1] != want {
		t.Errorf("underline = %q (%d chars), want %q (%d chars)", lines[1], len(lines[1]), want, len(want))
	}
}

func TestRenderOmitsURLWhenAbsent(t *testing.T) {
	t.Parallel()

	horoscope := Horoscope{
		Title:    "Ryby dnes",
		Sections: []Section{{Text: "text"}},
	}

	if got := Render(horoscope); strings.Contains(got, "http") {
		t.Errorf("Render() should not print a link when URL is empty, got %q", got)
	}
}

func TestRenderKeepsParagraphBreaksWithinASection(t *testing.T) {
	t.Parallel()

	horoscope := Horoscope{
		Title:    "Beran rok",
		Sections: []Section{{Heading: "Peníze a práce", Text: "První odstavec.\n\nDruhý odstavec."}},
	}

	got := Render(horoscope)
	if !strings.Contains(got, "První odstavec.\n\nDruhý odstavec.") {
		t.Errorf("Render() lost the paragraph break, got %q", got)
	}
}
