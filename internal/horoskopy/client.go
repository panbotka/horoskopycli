package horoskopy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// apiBaseURL is the JSON API backing www.horoskopy.cz. The site itself renders
// client-side from this API, so reading it directly is both simpler and far
// more stable than scraping the rendered page.
const apiBaseURL = "https://api-web.horoskopy.cz"

// requestTimeout bounds a single horoscope lookup; the API is a plain read and
// should answer well within this.
const requestTimeout = 15 * time.Second

// userAgent identifies this CLI to horoskopy.cz, so the traffic is
// attributable rather than anonymous.
const userAgent = "horoskopycli (+https://github.com/kozaktomas/horoskopycli)"

// Block type names used by the horoskopy.cz content model. Articles are built
// from "molecules"; horoscopes only ever use these two.
const (
	blockHeadline  = "HeadlineH2Molecule"
	blockParagraph = "ParagraphMolecule"
)

// ErrUnavailable is returned when horoskopy.cz cannot be reached or answers
// with an unexpected status code.
var ErrUnavailable = errors.New("horoskopy.cz is unavailable")

// ErrUnexpectedArticle is returned when the API answers with a different
// article than the one requested, which means horoskopy.cz has renumbered its
// articles and the ID table in this package is stale.
var ErrUnexpectedArticle = errors.New("horoskopy.cz returned an unexpected article")

// ErrEmptyArticle is returned when the requested article exists but carries no
// readable text.
var ErrEmptyArticle = errors.New("horoskopy.cz returned an empty article")

// Section is one part of a horoscope: a body of text under an optional
// heading. The opening summary of an article has no heading.
type Section struct {
	Heading string
	Text    string
}

// Horoscope is a single horoscope article, reduced to the parts worth printing.
type Horoscope struct {
	Title    string
	URL      string
	Sections []Section
}

// Client reads horoscopes from the horoskopy.cz JSON API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient returns a Client pointed at the public horoskopy.cz API.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    apiBaseURL,
	}
}

// apiDocument is the subset of a horoskopy.cz article the CLI cares about.
type apiDocument struct {
	Title   string     `json:"title"`
	Slug    string     `json:"slug"`
	UID     int        `json:"uid"`
	Content []apiBlock `json:"content"`
}

// apiBlock is one content block of an article. Class names the block type and
// Properties.Text carries its plain text.
type apiBlock struct {
	Class      string `json:"_cls"`
	Properties struct {
		Text string `json:"text"`
	} `json:"properties"`
}

// Fetch downloads the horoscope for a sign and period.
//
// It returns ErrUnavailable if horoskopy.cz cannot be reached or refuses the
// request, ErrUnexpectedArticle if the returned article is not the one asked
// for, and ErrEmptyArticle if the article carries no text.
func (c *Client) Fetch(ctx context.Context, sign Sign, period Period) (Horoscope, error) {
	uid, err := documentUID(sign, period)
	if err != nil {
		return Horoscope{}, err
	}

	document, err := c.get(ctx, uid)
	if err != nil {
		return Horoscope{}, err
	}

	wantSlug := documentSlug(sign, period)
	if document.Slug != wantSlug {
		return Horoscope{}, fmt.Errorf(
			"article %d is %q, expected %q: %w", uid, document.Slug, wantSlug, ErrUnexpectedArticle)
	}

	sections := toSections(document.Content)
	if len(sections) == 0 {
		return Horoscope{}, fmt.Errorf("article %d has no text: %w", uid, ErrEmptyArticle)
	}

	url, err := ArticleURL(sign, period)
	if err != nil {
		return Horoscope{}, err
	}

	return Horoscope{
		Title:    sanitizeText(document.Title),
		URL:      url,
		Sections: sections,
	}, nil
}

// get downloads and decodes a single article by its horoskopy.cz ID.
func (c *Client) get(ctx context.Context, uid int) (apiDocument, error) {
	url := fmt.Sprintf("%s/v1/documents/%d", c.baseURL, uid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return apiDocument{}, fmt.Errorf("could not build request for %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return apiDocument{}, fmt.Errorf("could not reach %s: %w: %w", url, ErrUnavailable, err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return apiDocument{}, fmt.Errorf("%s returned status %d: %w", url, res.StatusCode, ErrUnavailable)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return apiDocument{}, fmt.Errorf("could not read response from %s: %w: %w", url, ErrUnavailable, err)
	}

	var document apiDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return apiDocument{}, fmt.Errorf("could not decode response from %s: %w", url, err)
	}

	return document, nil
}

// toSections folds a flat list of content blocks into sections. A headline
// block opens a new section and the paragraphs that follow become its text;
// paragraphs appearing before any headline form the opening summary.
func toSections(blocks []apiBlock) []Section {
	sections := make([]Section, 0, len(blocks)/2+1)

	for _, block := range blocks {
		text := sanitizeText(block.Properties.Text)
		if text == "" {
			continue
		}

		switch block.Class {
		case blockHeadline:
			sections = append(sections, Section{Heading: text})
		case blockParagraph:
			appendParagraph(&sections, text)
		}
	}

	return dropEmptySections(sections)
}

// appendParagraph adds text to the section being built, starting an unheaded
// section if no headline has been seen yet.
func appendParagraph(sections *[]Section, text string) {
	if len(*sections) == 0 {
		*sections = append(*sections, Section{Text: text})

		return
	}

	current := &(*sections)[len(*sections)-1]
	if current.Text == "" {
		current.Text = text

		return
	}
	current.Text += "\n\n" + text
}

// dropEmptySections removes headings that ended up with no text under them, so
// the output never shows a bare heading.
func dropEmptySections(sections []Section) []Section {
	kept := make([]Section, 0, len(sections))
	for _, section := range sections {
		if section.Text != "" {
			kept = append(kept, section)
		}
	}

	return kept
}

// whitespace matches any run of whitespace, including newlines.
var whitespace = regexp.MustCompile(`\s+`)

// sanitizeText collapses whitespace runs into single spaces and trims the
// result, so text pasted into the CMS with stray newlines still prints tidily.
func sanitizeText(text string) string {
	return strings.TrimSpace(whitespace.ReplaceAllString(text, " "))
}
