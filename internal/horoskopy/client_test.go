package horoskopy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newTestClient serves the given handler and returns a Client pointed at it.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := NewClient()
	client.baseURL = server.URL

	return client
}

// writeBody sends a canned response body from a test HTTP handler.
func writeBody(t *testing.T, w http.ResponseWriter, body []byte) {
	t.Helper()

	if _, err := w.Write(body); err != nil {
		t.Errorf("could not write test response: %v", err)
	}
}

// fixture reads a recorded horoskopy.cz API response from testdata.
func fixture(t *testing.T, name string) []byte {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("could not read fixture %q: %v", name, err)
	}

	return body
}

func TestClientFetchParsesArticle(t *testing.T) {
	t.Parallel()

	var gotPath string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeBody(t, w, fixture(t, "ryby-dnes.json"))
	})

	horoscope, err := client.Fetch(context.Background(), SignRyby, PeriodDnes)
	if err != nil {
		t.Fatalf("Fetch returned an unexpected error: %v", err)
	}

	if want := "/v1/documents/546"; gotPath != want {
		t.Errorf("Fetch requested %q, want %q", gotPath, want)
	}
	if want := "Ryby dnes"; horoscope.Title != want {
		t.Errorf("Title = %q, want %q", horoscope.Title, want)
	}
	if want := "https://www.horoskopy.cz/clanek/horoskop-znameni-zverokruhu-ryby-dnes-546"; horoscope.URL != want {
		t.Errorf("URL = %q, want %q", horoscope.URL, want)
	}

	// The fixture opens with an unheaded summary and then five headed sections.
	if got, want := len(horoscope.Sections), 6; got != want {
		t.Fatalf("got %d sections, want %d", got, want)
	}
	if horoscope.Sections[0].Heading != "" {
		t.Errorf("first section should be the unheaded summary, got heading %q", horoscope.Sections[0].Heading)
	}
	if horoscope.Sections[0].Text == "" {
		t.Error("summary section has no text")
	}

	wantHeadings := []string{"Láska a přátelství", "Peníze a práce", "Rodina a vztahy", "Zdraví a kondice", "Aktivity vhodné pro dnešní den"}
	for i, want := range wantHeadings {
		section := horoscope.Sections[i+1]
		if section.Heading != want {
			t.Errorf("section %d heading = %q, want %q", i+1, section.Heading, want)
		}
		if section.Text == "" {
			t.Errorf("section %q has no text", want)
		}
	}
}

func TestClientFetchRejectsWrongArticle(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, []byte(`{"title":"Lev dnes","slug":"horoskop-znameni-zverokruhu-lev-dnes","uid":539,"content":[]}`))
	})

	_, err := client.Fetch(context.Background(), SignRyby, PeriodDnes)
	if !errors.Is(err, ErrUnexpectedArticle) {
		t.Fatalf("Fetch error = %v, want ErrUnexpectedArticle", err)
	}
}

func TestClientFetchOnServerError(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := client.Fetch(context.Background(), SignRyby, PeriodDnes)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Fetch error = %v, want ErrUnavailable", err)
	}
}

func TestClientFetchOnMalformedJSON(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, []byte("<html>not json</html>"))
	})

	if _, err := client.Fetch(context.Background(), SignRyby, PeriodDnes); err == nil {
		t.Fatal("Fetch should fail on malformed JSON, got nil error")
	}
}

func TestClientFetchOnEmptyArticle(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, []byte(`{"title":"Ryby dnes","slug":"horoskop-znameni-zverokruhu-ryby-dnes","uid":546,"content":[]}`))
	})

	_, err := client.Fetch(context.Background(), SignRyby, PeriodDnes)
	if !errors.Is(err, ErrEmptyArticle) {
		t.Fatalf("Fetch error = %v, want ErrEmptyArticle", err)
	}
}

func TestToSections(t *testing.T) {
	t.Parallel()

	headline := func(text string) apiBlock { return newBlock(blockHeadline, text) }
	paragraph := func(text string) apiBlock { return newBlock(blockParagraph, text) }

	tests := map[string]struct {
		blocks []apiBlock
		want   []Section
	}{
		"summary then one headed section": {
			blocks: []apiBlock{paragraph("Souhrn."), headline("Láska"), paragraph("Text.")},
			want:   []Section{{Text: "Souhrn."}, {Heading: "Láska", Text: "Text."}},
		},
		"several paragraphs under one heading keep a blank line between them": {
			blocks: []apiBlock{headline("Rok"), paragraph("První."), paragraph("Druhý.")},
			want:   []Section{{Heading: "Rok", Text: "První.\n\nDruhý."}},
		},
		"several paragraphs before any heading form one summary": {
			blocks: []apiBlock{paragraph("První."), paragraph("Druhý.")},
			want:   []Section{{Text: "První.\n\nDruhý."}},
		},
		"heading with no text under it is dropped": {
			blocks: []apiBlock{headline("Prázdná"), headline("Plná"), paragraph("Text.")},
			want:   []Section{{Heading: "Plná", Text: "Text."}},
		},
		"unknown block types are ignored": {
			blocks: []apiBlock{newBlock("GalleryMolecule", "obrázek"), headline("Láska"), paragraph("Text.")},
			want:   []Section{{Heading: "Láska", Text: "Text."}},
		},
		"blank blocks are ignored": {
			blocks: []apiBlock{headline("Láska"), paragraph("   \n  "), paragraph("Text.")},
			want:   []Section{{Heading: "Láska", Text: "Text."}},
		},
		"whitespace inside text is collapsed": {
			blocks: []apiBlock{paragraph("Dva  řádky\n\ntextu.")},
			want:   []Section{{Text: "Dva řádky textu."}},
		},
		"no blocks at all": {
			blocks: nil,
			want:   []Section{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := toSections(tt.blocks)
			if len(got) != len(tt.want) {
				t.Fatalf("toSections() = %+v, want %+v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("section %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// newBlock builds a content block of the given type carrying the given text.
func newBlock(class, text string) apiBlock {
	var block apiBlock
	block.Class = class
	block.Properties.Text = text

	return block
}
