package seznam

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubExecutable writes an executable file with the given name into dir.
func stubExecutable(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("could not create %s: %v", path, err)
	}

	return path
}

func TestFindBrowserPrefersTheRequestedOne(t *testing.T) {
	dir := t.TempDir()
	wanted := stubExecutable(t, dir, "my-browser")
	stubExecutable(t, dir, "chromium")
	t.Setenv("PATH", dir)

	got, err := findBrowser("my-browser")
	if err != nil {
		t.Fatalf("findBrowser returned %v", err)
	}
	if got != wanted {
		t.Errorf("findBrowser(%q) = %q, want %q", "my-browser", got, wanted)
	}
}

func TestFindBrowserSearchesPath(t *testing.T) {
	dir := t.TempDir()
	wanted := stubExecutable(t, dir, "chromium")
	t.Setenv("PATH", dir)

	got, err := findBrowser("")
	if err != nil {
		t.Fatalf("findBrowser returned %v", err)
	}
	if got != wanted {
		t.Errorf("findBrowser() = %q, want %q", got, wanted)
	}
}

func TestFindBrowserReportsAMissingBrowser(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := findBrowser(""); !errors.Is(err, ErrNoBrowser) {
		// A machine with Chrome installed in a standard location still finds
		// one, which is a pass as far as this function is concerned.
		if err == nil {
			t.Skip("this machine has a browser in a well-known location")
		}

		t.Errorf("findBrowser() = %v, want ErrNoBrowser", err)
	}
}

func TestFindBrowserRejectsAnUnusableRequest(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := findBrowser("definitely-not-a-browser"); err == nil {
		t.Error("findBrowser with a missing executable should fail")
	}
}

func TestReadEndpoint(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		contents string
		want     string
		wantOK   bool
	}{
		"port and path": {
			contents: "45037\n/devtools/browser/c9fa5370\n",
			want:     "ws://127.0.0.1:45037/devtools/browser/c9fa5370",
			wantOK:   true,
		},
		"still being written": {contents: "45037\n"},
		"empty file":          {contents: ""},
		"blank path":          {contents: "45037\n\n"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "DevToolsActivePort")
			if err := os.WriteFile(path, []byte(tt.contents), 0o600); err != nil {
				t.Fatalf("could not write the port file: %v", err)
			}

			got, ok := readEndpoint(path)
			if ok != tt.wantOK {
				t.Fatalf("readEndpoint(%q) ok = %v, want %v", tt.contents, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("readEndpoint(%q) = %q, want %q", tt.contents, got, tt.want)
			}
		})
	}
}

func TestReadEndpointOnAMissingFile(t *testing.T) {
	t.Parallel()

	if _, ok := readEndpoint(filepath.Join(t.TempDir(), "DevToolsActivePort")); ok {
		t.Error("readEndpoint should report a missing file as not ready")
	}
}

func TestWaitForEndpointGivesUp(t *testing.T) {
	t.Parallel()

	_, err := waitForEndpoint(t.TempDir(), 150*time.Millisecond)
	if !errors.Is(err, ErrNoDebugPort) {
		t.Errorf("waitForEndpoint on an empty profile = %v, want ErrNoDebugPort", err)
	}
}

func TestWaitForEndpointFindsALateFile(t *testing.T) {
	t.Parallel()

	profile := t.TempDir()
	go func() {
		time.Sleep(150 * time.Millisecond)
		path := filepath.Join(profile, "DevToolsActivePort")
		if err := os.WriteFile(path, []byte("9222\n/devtools/browser/late\n"), 0o600); err != nil {
			t.Errorf("could not write the port file: %v", err)
		}
	}()

	got, err := waitForEndpoint(profile, 5*time.Second)
	if err != nil {
		t.Fatalf("waitForEndpoint returned %v", err)
	}
	if got != "ws://127.0.0.1:9222/devtools/browser/late" {
		t.Errorf("waitForEndpoint = %q", got)
	}
}

func TestBrowserStopIsSafeWithoutAProcess(t *testing.T) {
	t.Parallel()

	profile := t.TempDir()
	(&browser{profile: profile}).stop()

	if _, err := os.Stat(profile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stop should remove the throwaway profile, stat returned %v", err)
	}
}
