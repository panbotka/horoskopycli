package horoskopy

import (
	"strings"
	"unicode/utf8"
)

// headingMarker prefixes each section heading, mirroring the layout the CLI
// printed before horoskopy.cz moved to its JSON API.
const headingMarker = "=> "

// Render lays a horoscope out as plain text for a terminal: the title with an
// underline, then each section, then a link to the original article. The
// result ends with a newline, so callers can print it as is.
func Render(horoscope Horoscope) string {
	var out strings.Builder

	out.WriteString(horoscope.Title)
	out.WriteString("\n")
	out.WriteString(strings.Repeat("=", utf8.RuneCountInString(horoscope.Title)))
	out.WriteString("\n\n")

	for _, section := range horoscope.Sections {
		if section.Heading != "" {
			out.WriteString(headingMarker)
			out.WriteString(section.Heading)
			out.WriteString("\n")
		}
		out.WriteString(section.Text)
		out.WriteString("\n\n")
	}

	if horoscope.URL != "" {
		out.WriteString(horoscope.URL)
		out.WriteString("\n")
	}

	return out.String()
}
