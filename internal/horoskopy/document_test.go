package horoskopy

import (
	"errors"
	"testing"
)

// TestDocumentUID pins the article IDs against the values observed on the live
// horoskopy.cz API, so a silent renumbering shows up here first.
func TestDocumentUID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sign   Sign
		period Period
		want   int
	}{
		"first sign of dnes":  {sign: SignBeran, period: PeriodDnes, want: 535},
		"last sign of dnes":   {sign: SignRyby, period: PeriodDnes, want: 546},
		"middle sign of dnes": {sign: SignPanna, period: PeriodDnes, want: 540},
		"first sign of zitra": {sign: SignBeran, period: PeriodZitra, want: 547},
		"last sign of zitra":  {sign: SignRyby, period: PeriodZitra, want: 558},
		"first sign of mesic": {sign: SignBeran, period: PeriodMesic, want: 559},
		"last sign of mesic":  {sign: SignRyby, period: PeriodMesic, want: 570},
		"first sign of rok":   {sign: SignBeran, period: PeriodRok, want: 571},
		"last sign of rok":    {sign: SignRyby, period: PeriodRok, want: 582},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := documentUID(tt.sign, tt.period)
			if err != nil {
				t.Fatalf("documentUID(%q, %q) unexpected error: %v", tt.sign, tt.period, err)
			}
			if got != tt.want {
				t.Errorf("documentUID(%q, %q) = %d, want %d", tt.sign, tt.period, got, tt.want)
			}
		})
	}
}

func TestDocumentUIDIsUniquePerCombination(t *testing.T) {
	t.Parallel()

	seen := make(map[int]string, len(signs)*len(periods))
	for _, period := range periods {
		for _, sign := range signs {
			uid, err := documentUID(sign, period)
			if err != nil {
				t.Fatalf("documentUID(%q, %q) unexpected error: %v", sign, period, err)
			}
			if other, clash := seen[uid]; clash {
				t.Errorf("uid %d used by both %s and %s/%s", uid, other, sign, period)
			}
			seen[uid] = string(sign) + "/" + string(period)
		}
	}

	if len(seen) != 48 {
		t.Errorf("got %d distinct article ids, want 48", len(seen))
	}
}

func TestDocumentUIDRejectsUnknownPeriod(t *testing.T) {
	t.Parallel()

	if _, err := documentUID(SignByk, Period("tyden")); err == nil {
		t.Error("documentUID with an unknown period should fail, got nil error")
	}
}

func TestDocumentSlug(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sign   Sign
		period Period
		want   string
	}{
		"ryby dnes":  {sign: SignRyby, period: PeriodDnes, want: "horoskop-znameni-zverokruhu-ryby-dnes"},
		"beran rok":  {sign: SignBeran, period: PeriodRok, want: "horoskop-znameni-zverokruhu-beran-rok"},
		"stir mesic": {sign: SignStir, period: PeriodMesic, want: "horoskop-znameni-zverokruhu-stir-mesic"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := documentSlug(tt.sign, tt.period); got != tt.want {
				t.Errorf("documentSlug(%q, %q) = %q, want %q", tt.sign, tt.period, got, tt.want)
			}
		})
	}
}

func TestArticleURL(t *testing.T) {
	t.Parallel()

	want := "https://www.horoskopy.cz/clanek/horoskop-znameni-zverokruhu-ryby-dnes-546"
	got, err := ArticleURL(SignRyby, PeriodDnes)
	if err != nil {
		t.Fatalf("ArticleURL unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("ArticleURL(ryby, dnes) = %q, want %q", got, want)
	}
}

func TestDocumentUIDRejectsUnknownSign(t *testing.T) {
	t.Parallel()

	if _, err := documentUID(Sign("dymovnica"), PeriodDnes); !errors.Is(err, ErrUnknownSign) {
		t.Errorf("documentUID with an unknown sign error = %v, want ErrUnknownSign", err)
	}
}

func TestArticleURLRejectsUnknownPeriod(t *testing.T) {
	t.Parallel()

	if _, err := ArticleURL(SignByk, Period("tyden")); !errors.Is(err, ErrUnknownPeriod) {
		t.Errorf("ArticleURL with an unknown period error = %v, want ErrUnknownPeriod", err)
	}
}
