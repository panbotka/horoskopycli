# Architecture

`horoskopycli` prints one horoskopy.cz horoscope and exits. It is a single Go binary with no
state, no configuration and one outbound dependency: the horoskopy.cz JSON API.

## Components

```
main.go                        CLI: argument parsing, usage text, exit codes
└── internal/horoskopy
    ├── catalog.go             signs, periods, and parsing user input into them
    ├── document.go            sign + period -> horoskopy.cz article ID and slug
    ├── client.go              HTTP client, JSON decoding, article -> Horoscope
    └── render.go              Horoscope -> plain text for a terminal
```

`main.go` depends on `internal/horoskopy`; nothing depends on `main.go`. The package is
transport- and CLI-agnostic: it returns a `Horoscope` value, and `Render` is a separate step
so the data could be formatted differently without touching the client.

## Data flow

```
"ryby" "zitra"
      │
      ├─ ParseSign / ParsePeriod      lower-case, trim, strip diacritics, match a known slug
      │
      ├─ documentUID                  ryby + zitra -> article 558
      │
      ├─ GET api-web.horoskopy.cz/v1/documents/558
      │
      ├─ verify slug                  response slug must be "…-ryby-zitra"
      │
      ├─ toSections                   content blocks -> headed sections
      │
      └─ Render                       -> text on stdout
```

## Why the JSON API and not the page

The tool originally scraped `www.horoskopy.cz` with an XPath expression for `#content-detail`.
The 2026 redesign moved the site onto Seznam's IMA.js stack: the page is rendered client-side,
that element no longer exists, and the remaining class names are build-time hashes (`c_bn`,
`g_e4`) that change on every deploy. Scraping it again would break again.

The site itself renders from `https://api-web.horoskopy.cz`, which serves clean structured JSON
and no markup. Reading it directly is both simpler and considerably more stable. It is an
undocumented API and could change without notice, hence the slug check described below.

## Article IDs

horoskopy.cz keeps one evergreen article per sign and period — 48 in total — and rewrites the
body in place as the horoscope changes. The IDs are therefore stable and laid out in blocks of
twelve consecutive numbers, one block per period, in zodiac order starting at Beran:

| Period  | Beran | … | Ryby |
|---------|-------|---|------|
| `dnes`  | 535   | … | 546  |
| `zitra` | 547   | … | 558  |
| `mesic` | 559   | … | 570  |
| `rok`   | 571   | … | 582  |

So `documentUID` is `periodBaseUID[period] + <index of sign in signs>`. The order of the `signs`
slice in `catalog.go` is load-bearing.

This table is hardcoded because the API supports no filtering: `slug`, `where`, `uid`,
`sections`, `q` and `search` are all accepted and all ignored, each returning the same
unfiltered document listing. There is no way to look an article up by name.

To make a stale table loud rather than silent, `Client.Fetch` compares the `slug` the API echoes
back against the slug it expected and returns `ErrUnexpectedArticle` on a mismatch. Without that
check, a renumbering would print the wrong sign's horoscope and look perfectly fine.

`TestDocumentUID` pins the boundary IDs of every block, so the table is also asserted in CI.

## Errors

| Error                  | Meaning                                              | CLI behaviour                  |
|------------------------|------------------------------------------------------|--------------------------------|
| `ErrUnknownSign`       | input is not a zodiac sign                            | usage text, exit 1             |
| `ErrUnknownPeriod`     | input is not a supported period                       | usage text, exit 1             |
| `ErrUnavailable`       | horoskopy.cz unreachable or non-200                   | apology ASCII art, exit 1      |
| `ErrUnexpectedArticle` | API returned a different article than requested       | error message, exit 1          |
| `ErrEmptyArticle`      | article exists but carries no text                    | error message, exit 1          |

Only `ErrUnavailable` prints the apology; bad user input is not an outage.

## Testing

`internal/horoskopy` is tested against `httptest` servers, with a recorded API response in
`internal/horoskopy/testdata/ryby-dnes.json`. `main.go` is tested through `run`, which takes a
`fetcher` interface, so the CLI is exercised end to end without network access. No test reaches
the real horoskopy.cz.
