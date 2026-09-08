// Package horoskopy reads horoscopes from the public horoskopy.cz JSON API.
package horoskopy

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// ErrUnknownSign is returned when the user asks for a zodiac sign horoskopy.cz
// does not publish.
var ErrUnknownSign = errors.New("unknown zodiac sign")

// ErrUnknownPeriod is returned when the user asks for a time span horoskopy.cz
// does not publish.
var ErrUnknownPeriod = errors.New("unknown horoscope period")

// Sign is a zodiac sign, identified by the diacritics-free Czech slug
// horoskopy.cz uses in its article URLs (for example "byk").
type Sign string

// The twelve zodiac signs, declared in the order horoskopy.cz numbers their
// articles. The order is load-bearing: documentUID derives an article ID from
// a sign's position in signs.
const (
	SignBeran    Sign = "beran"
	SignByk      Sign = "byk"
	SignBlizenci Sign = "blizenci"
	SignRak      Sign = "rak"
	SignLev      Sign = "lev"
	SignPanna    Sign = "panna"
	SignVahy     Sign = "vahy"
	SignStir     Sign = "stir"
	SignStrelec  Sign = "strelec"
	SignKozoroh  Sign = "kozoroh"
	SignVodnar   Sign = "vodnar"
	SignRyby     Sign = "ryby"
)

// signs lists every zodiac sign in horoskopy.cz article order. Do not reorder.
var signs = []Sign{
	SignBeran, SignByk, SignBlizenci, SignRak,
	SignLev, SignPanna, SignVahy, SignStir,
	SignStrelec, SignKozoroh, SignVodnar, SignRyby,
}

// Period is the time span a horoscope covers, identified by the
// diacritics-free Czech slug horoskopy.cz uses in its article URLs.
type Period string

// The four time spans horoskopy.cz publishes for every sign.
const (
	PeriodDnes  Period = "dnes"
	PeriodZitra Period = "zitra"
	PeriodMesic Period = "mesic"
	PeriodRok   Period = "rok"
)

// DefaultPeriod is used when the user names a sign but no period.
const DefaultPeriod = PeriodDnes

// periods lists every period in the order the CLI advertises them.
var periods = []Period{PeriodDnes, PeriodZitra, PeriodMesic, PeriodRok}

// periodBaseUID maps a period to the horoskopy.cz article ID of its first
// zodiac sign (Beran). Every other sign of that period follows consecutively,
// so the Ryby article for a period is baseUID+11.
var periodBaseUID = map[Period]int{
	PeriodDnes:  535,
	PeriodZitra: 547,
	PeriodMesic: 559,
	PeriodRok:   571,
}

// ParseSign turns user input into a Sign, ignoring case, surrounding
// whitespace and Czech diacritics, so that "Býk", "byk" and " BYK " all
// resolve to SignByk. It returns ErrUnknownSign for anything else.
func ParseSign(input string) (Sign, error) {
	candidate := Sign(normalize(input))
	for _, sign := range signs {
		if sign == candidate {
			return sign, nil
		}
	}

	return "", fmt.Errorf("%q: %w", input, ErrUnknownSign)
}

// ParsePeriod turns user input into a Period, ignoring case, surrounding
// whitespace and Czech diacritics, so that "zítra" and "ZITRA" both resolve to
// PeriodZitra. It returns ErrUnknownPeriod for anything else, including an
// empty string; callers wanting the default should not call ParsePeriod at all.
func ParsePeriod(input string) (Period, error) {
	candidate := Period(normalize(input))
	for _, period := range periods {
		if period == candidate {
			return period, nil
		}
	}

	return "", fmt.Errorf("%q: %w", input, ErrUnknownPeriod)
}

// SignSlugs returns every supported zodiac sign slug, for usage messages.
func SignSlugs() []string {
	slugs := make([]string, 0, len(signs))
	for _, sign := range signs {
		slugs = append(slugs, string(sign))
	}

	return slugs
}

// PeriodSlugs returns every supported period slug, for usage messages.
func PeriodSlugs() []string {
	slugs := make([]string, 0, len(periods))
	for _, period := range periods {
		slugs = append(slugs, string(period))
	}

	return slugs
}

// nonSpacingMarks matches the combining marks left behind by NFD
// normalisation, which is how diacritics are stripped below.
type nonSpacingMarks struct{}

// Contains reports whether r is a Unicode non-spacing mark (category Mn).
func (nonSpacingMarks) Contains(r rune) bool {
	return unicode.Is(unicode.Mn, r)
}

// normalize lower-cases input, trims surrounding whitespace and removes Czech
// diacritics, producing the slug form horoskopy.cz uses. Input that cannot be
// transformed is returned lower-cased and trimmed but otherwise untouched,
// which simply means it will not match any known slug.
func normalize(input string) string {
	trimmed := strings.ToLower(strings.TrimSpace(input))

	t := transform.Chain(norm.NFD, runes.Remove(nonSpacingMarks{}), norm.NFC)
	stripped, _, err := transform.String(t, trimmed)
	if err != nil {
		return trimmed
	}

	return stripped
}
