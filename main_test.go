package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kozaktomas/horoskopycli/internal/horoskopy"
)

// fakeFetcher stands in for the horoskopy.cz client so the CLI can be exercised
// without touching the network.
type fakeFetcher struct {
	horoscope horoskopy.Horoscope
	err       error

	gotSign   horoskopy.Sign
	gotPeriod horoskopy.Period
	calls     int
}

// Fetch records the arguments it was called with and returns the canned result.
func (f *fakeFetcher) Fetch(_ context.Context, sign horoskopy.Sign, period horoskopy.Period) (horoskopy.Horoscope, error) {
	f.calls++
	f.gotSign = sign
	f.gotPeriod = period

	return f.horoscope, f.err
}

// newFakeFetcher returns a fetcher answering with a minimal valid horoscope.
func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{
		horoscope: horoskopy.Horoscope{
			Title:    "Býk dnes",
			Sections: []horoskopy.Section{{Text: "Dnes to bude dobré."}},
		},
	}
}

func TestRunFetchesRequestedSignAndPeriod(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args       []string
		wantSign   horoskopy.Sign
		wantPeriod horoskopy.Period
	}{
		"sign only defaults to dnes": {args: []string{"byk"}, wantSign: horoskopy.SignByk, wantPeriod: horoskopy.PeriodDnes},
		"explicit dnes":              {args: []string{"byk", "dnes"}, wantSign: horoskopy.SignByk, wantPeriod: horoskopy.PeriodDnes},
		"tomorrow":                   {args: []string{"ryby", "zitra"}, wantSign: horoskopy.SignRyby, wantPeriod: horoskopy.PeriodZitra},
		"month":                      {args: []string{"lev", "mesic"}, wantSign: horoskopy.SignLev, wantPeriod: horoskopy.PeriodMesic},
		"year":                       {args: []string{"stir", "rok"}, wantSign: horoskopy.SignStir, wantPeriod: horoskopy.PeriodRok},
		"czech diacritics":           {args: []string{"Býk", "zítra"}, wantSign: horoskopy.SignByk, wantPeriod: horoskopy.PeriodZitra},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fetcher := newFakeFetcher()
			var out bytes.Buffer

			if err := run(context.Background(), fetcher, tt.args, &out); err != nil {
				t.Fatalf("run(%v) unexpected error: %v", tt.args, err)
			}
			if fetcher.gotSign != tt.wantSign {
				t.Errorf("run(%v) fetched sign %q, want %q", tt.args, fetcher.gotSign, tt.wantSign)
			}
			if fetcher.gotPeriod != tt.wantPeriod {
				t.Errorf("run(%v) fetched period %q, want %q", tt.args, fetcher.gotPeriod, tt.wantPeriod)
			}
			if !strings.Contains(out.String(), "Dnes to bude dobré.") {
				t.Errorf("run(%v) did not print the horoscope, got %q", tt.args, out.String())
			}
		})
	}
}

func TestRunWithoutArgumentsPrintsUsage(t *testing.T) {
	t.Parallel()

	fetcher := newFakeFetcher()
	var out bytes.Buffer

	if err := run(context.Background(), fetcher, nil, &out); err != nil {
		t.Fatalf("run with no arguments should succeed, got %v", err)
	}
	if fetcher.calls != 0 {
		t.Errorf("run with no arguments should not fetch, got %d calls", fetcher.calls)
	}

	got := out.String()
	for _, want := range []string{"byk", "ryby", "dnes", "zitra", "mesic", "rok"} {
		if !strings.Contains(got, want) {
			t.Errorf("usage should mention %q, got %q", want, got)
		}
	}
}

func TestRunRejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args    []string
		wantErr error
	}{
		"unknown sign":   {args: []string{"dymovnica"}, wantErr: horoskopy.ErrUnknownSign},
		"unknown period": {args: []string{"byk", "tyden"}, wantErr: horoskopy.ErrUnknownPeriod},
		"swapped order":  {args: []string{"dnes", "byk"}, wantErr: horoskopy.ErrUnknownSign},
		"too many args":  {args: []string{"byk", "dnes", "navic"}, wantErr: errTooManyArguments},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fetcher := newFakeFetcher()
			var out bytes.Buffer

			err := run(context.Background(), fetcher, tt.args, &out)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("run(%v) error = %v, want %v", tt.args, err, tt.wantErr)
			}
			if fetcher.calls != 0 {
				t.Errorf("run(%v) should not fetch on bad input, got %d calls", tt.args, fetcher.calls)
			}
		})
	}
}

func TestRunPrintsApologyWhenSiteIsDown(t *testing.T) {
	t.Parallel()

	fetcher := newFakeFetcher()
	fetcher.err = horoskopy.ErrUnavailable
	var out bytes.Buffer

	err := run(context.Background(), fetcher, []string{"byk"}, &out)
	if !errors.Is(err, horoskopy.ErrUnavailable) {
		t.Fatalf("run error = %v, want ErrUnavailable", err)
	}
	if !strings.Contains(out.String(), "This is fine.") {
		t.Errorf("run should print the apology when the site is down, got %q", out.String())
	}
}

func TestRunDoesNotPrintApologyForBadInput(t *testing.T) {
	t.Parallel()

	fetcher := newFakeFetcher()
	var out bytes.Buffer

	if err := run(context.Background(), fetcher, []string{"dymovnica"}, &out); err == nil {
		t.Fatal("run with an unknown sign should fail, got nil error")
	}
	if strings.Contains(out.String(), "This is fine.") {
		t.Errorf("bad input is not an outage, apology should not be printed, got %q", out.String())
	}
}
