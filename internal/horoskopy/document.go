package horoskopy

import "fmt"

// webBaseURL is the public site, used only to build human-readable article
// links shown alongside a horoscope.
const webBaseURL = "https://www.horoskopy.cz"

// slugPrefix is shared by every zodiac article slug on horoskopy.cz.
const slugPrefix = "horoskop-znameni-zverokruhu"

// documentUID returns the horoskopy.cz article ID for a sign and period.
//
// horoskopy.cz keeps one evergreen article per sign and period and rewrites its
// body as the horoscope changes, so these IDs are stable and can be derived
// arithmetically: each period occupies a block of twelve consecutive IDs
// starting at Beran. It returns ErrUnknownPeriod if the period has no such
// block, which can only happen for a Period value built outside ParsePeriod.
func documentUID(sign Sign, period Period) (int, error) {
	base, ok := periodBaseUID[period]
	if !ok {
		return 0, fmt.Errorf("%q: %w", period, ErrUnknownPeriod)
	}

	for offset, candidate := range signs {
		if candidate == sign {
			return base + offset, nil
		}
	}

	return 0, fmt.Errorf("%q: %w", sign, ErrUnknownSign)
}

// documentSlug returns the horoskopy.cz article slug for a sign and period,
// for example "horoskop-znameni-zverokruhu-ryby-dnes". The API echoes this slug
// back, which lets the client confirm it received the article it asked for.
func documentSlug(sign Sign, period Period) string {
	return fmt.Sprintf("%s-%s-%s", slugPrefix, sign, period)
}

// ArticleURL returns the public horoskopy.cz page for a sign and period, so the
// CLI can point users at the original article. It fails only for a period that
// did not come from ParsePeriod.
func ArticleURL(sign Sign, period Period) (string, error) {
	uid, err := documentUID(sign, period)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/clanek/%s-%d", webBaseURL, documentSlug(sign, period), uid), nil
}
