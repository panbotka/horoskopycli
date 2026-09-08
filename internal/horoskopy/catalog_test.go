package horoskopy

import (
	"errors"
	"testing"
)

func TestParseSign(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    Sign
		wantErr error
	}{
		"plain slug":           {input: "byk", want: SignByk},
		"uppercase":            {input: "Býk", want: SignByk},
		"diacritics stripped":  {input: "šťír", want: SignStir},
		"surrounding spaces":   {input: "  ryby  ", want: SignRyby},
		"unknown sign":         {input: "dymovnica", wantErr: ErrUnknownSign},
		"empty input":          {input: "", wantErr: ErrUnknownSign},
		"period is not a sign": {input: "dnes", wantErr: ErrUnknownSign},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseSign(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseSign(%q) error = %v, want %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr == nil && got != tt.want {
				t.Errorf("ParseSign(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePeriod(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    Period
		wantErr error
	}{
		"today":                {input: "dnes", want: PeriodDnes},
		"diacritics stripped":  {input: "zítra", want: PeriodZitra},
		"uppercase":            {input: "MĚSÍC", want: PeriodMesic},
		"year":                 {input: "rok", want: PeriodRok},
		"empty defaults":       {input: "", wantErr: ErrUnknownPeriod},
		"unknown period":       {input: "tyden", wantErr: ErrUnknownPeriod},
		"sign is not a period": {input: "byk", wantErr: ErrUnknownPeriod},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParsePeriod(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParsePeriod(%q) error = %v, want %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr == nil && got != tt.want {
				t.Errorf("ParsePeriod(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSignSlugsCoversWholeZodiac(t *testing.T) {
	t.Parallel()

	got := SignSlugs()
	if len(got) != 12 {
		t.Fatalf("SignSlugs() returned %d signs, want 12", len(got))
	}

	seen := make(map[string]bool, len(got))
	for _, slug := range got {
		if seen[slug] {
			t.Errorf("SignSlugs() contains duplicate %q", slug)
		}
		seen[slug] = true
	}
}

func TestPeriodSlugs(t *testing.T) {
	t.Parallel()

	want := []string{"dnes", "zitra", "mesic", "rok"}
	got := PeriodSlugs()

	if len(got) != len(want) {
		t.Fatalf("PeriodSlugs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("PeriodSlugs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
