package seznam

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

func TestEngineFor(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path          string
		want          engine
		wantSupported bool
	}{
		"firefox":            {path: "/usr/bin/firefox", want: engineFirefox, wantSupported: true},
		"firefox on windows": {path: `C:\Program Files\Mozilla Firefox\firefox.exe`, want: engineFirefox, wantSupported: true},
		"firefox on macos":   {path: "/Applications/Firefox.app/Contents/MacOS/firefox", want: engineFirefox, wantSupported: true},
		"librewolf":          {path: "/usr/bin/librewolf", want: engineFirefox, wantSupported: true},
		"chrome":             {path: "/opt/google/chrome/google-chrome", want: engineChromium, wantSupported: true},
		"chromium":           {path: "/usr/bin/chromium-browser", want: engineChromium, wantSupported: true},
		"brave":              {path: "/usr/bin/brave-browser", want: engineChromium, wantSupported: true},
		"edge":               {path: "/usr/bin/microsoft-edge", want: engineChromium, wantSupported: true},
		"safari":             {path: "/Applications/Safari.app/Contents/MacOS/Safari", wantSupported: false},
		"something else":     {path: "/usr/bin/epiphany", wantSupported: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, supported := engineFor(tt.path)
			if supported != tt.wantSupported {
				t.Fatalf("engineFor(%q) supported = %v, want %v", tt.path, supported, tt.wantSupported)
			}
			if supported && got != tt.want {
				t.Errorf("engineFor(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestFindBrowserPrefersTheRequestedOne(t *testing.T) {
	dir := t.TempDir()
	wanted := stubExecutable(t, dir, "firefox")
	stubExecutable(t, dir, "chromium")
	t.Setenv("PATH", dir)

	got, err := findBrowser("firefox")
	if err != nil {
		t.Fatalf("findBrowser returned %v", err)
	}
	if got.path != wanted {
		t.Errorf("findBrowser(%q) = %q, want %q", "firefox", got.path, wanted)
	}
	if got.engine != engineFirefox {
		t.Errorf("findBrowser(%q) engine = %v, want Firefox", "firefox", got.engine)
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
	if got.path != wanted {
		t.Errorf("findBrowser() = %q, want %q", got.path, wanted)
	}
	if got.engine != engineChromium {
		t.Errorf("findBrowser() engine = %v, want Chromium", got.engine)
	}
}

func TestFindBrowserReportsAMissingBrowser(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := findBrowser(""); !errors.Is(err, ErrNoBrowser) {
		// A machine with a browser installed in a standard location still
		// finds one, which is a pass as far as this function is concerned.
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

func TestBrowserArgs(t *testing.T) {
	t.Parallel()

	firefox := strings.Join(browserArgs(engineFirefox, "/profile"), " ")
	for _, want := range []string{"--profile /profile", "--no-remote", "--remote-debugging-port=0"} {
		if !strings.Contains(firefox, want) {
			t.Errorf("Firefox args %q should contain %q", firefox, want)
		}
	}
	if strings.Contains(firefox, "--user-data-dir") {
		t.Error("Firefox does not understand --user-data-dir")
	}

	chromium := strings.Join(browserArgs(engineChromium, "/profile"), " ")
	for _, want := range []string{"--user-data-dir=/profile", "--remote-debugging-port=0"} {
		if !strings.Contains(chromium, want) {
			t.Errorf("Chromium args %q should contain %q", chromium, want)
		}
	}
	if strings.Contains(chromium, "--profile ") {
		t.Error("Chromium does not understand --profile")
	}

	// The address is opened over the protocol instead, so that a browser which
	// is already running cannot pick it up.
	for _, args := range []string{firefox, chromium} {
		if strings.Contains(args, "http") {
			t.Errorf("args %q should carry no address", args)
		}
	}
}

func TestSnapProfileDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, ok := snapProfileDir("/usr/bin/firefox"); ok {
		t.Error("snapProfileDir should report nothing without a snap directory")
	}

	snap := filepath.Join(home, "snap", "firefox", "common")
	if err := os.MkdirAll(snap, 0o700); err != nil {
		t.Fatalf("could not create the snap directory: %v", err)
	}

	got, ok := snapProfileDir("/usr/bin/firefox")
	if !ok {
		t.Fatal("snapProfileDir should find the snap directory")
	}
	if got != snap {
		t.Errorf("snapProfileDir() = %q, want %q", got, snap)
	}

	// A browser that is not that snap keeps the ordinary temporary directory.
	if _, ok := snapProfileDir("/opt/google/chrome/google-chrome"); ok {
		t.Error("snapProfileDir should not send Chrome into the Firefox snap")
	}
}

func TestNewProfileUsesTheSnapDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	snap := filepath.Join(home, "snap", "firefox", "common")
	if err := os.MkdirAll(snap, 0o700); err != nil {
		t.Fatalf("could not create the snap directory: %v", err)
	}

	profile, err := newProfile(foundBrowser{path: "/snap/bin/firefox", engine: engineFirefox})
	if err != nil {
		t.Fatalf("newProfile returned %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(profile)
	})

	if filepath.Dir(profile) != snap {
		t.Errorf("newProfile() = %q, want it inside %q: a snap cannot read /tmp", profile, snap)
	}
}

func TestWaitForAnnouncementFindsTheEndpoint(t *testing.T) {
	t.Parallel()

	output := strings.NewReader(
		"*** You are running in headless mode.\n" +
			"WebDriver BiDi listening on ws://127.0.0.1:43227\n" +
			"console.warn: something else entirely\n")

	got, err := waitForAnnouncement(output, 2*time.Second)
	if err != nil {
		t.Fatalf("waitForAnnouncement returned %v", err)
	}
	if got != "ws://127.0.0.1:43227" {
		t.Errorf("waitForAnnouncement = %q, want the announced address", got)
	}
}

func TestWaitForAnnouncementGivesUp(t *testing.T) {
	t.Parallel()

	// A browser that says nothing useful: the reader stays open, as a live
	// process's output would.
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = writer.Close()
	})

	_, err := waitForAnnouncement(reader, 100*time.Millisecond)
	if !errors.Is(err, ErrNoDebugPort) {
		t.Errorf("waitForAnnouncement = %v, want ErrNoDebugPort", err)
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

func TestBrowserWaitOrKillEndsAStubbornBrowser(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start a stand-in for a browser: %v", err)
	}

	started := time.Now()
	(&browser{cmd: cmd}).waitOrKill(50 * time.Millisecond)

	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("waitOrKill took %s, want it to give up quickly", elapsed)
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err == nil {
		t.Error("waitOrKill should have killed a browser that would not leave")
	}
}

func TestBrowserWaitOrKillReturnsWhenTheBrowserLeaves(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start a stand-in for a browser: %v", err)
	}

	started := time.Now()
	(&browser{cmd: cmd}).waitOrKill(10 * time.Second)

	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("waitOrKill waited %s for a browser that had already left", elapsed)
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

func TestEndpointAddress(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		endpoint string
		want     string
		wantOK   bool
	}{
		"chromium endpoint": {
			endpoint: "ws://127.0.0.1:45037/devtools/browser/c9fa5370",
			want:     "127.0.0.1:45037",
			wantOK:   true,
		},
		"firefox endpoint": {endpoint: "ws://127.0.0.1:43227", want: "127.0.0.1:43227", wantOK: true},
		"with a session path": {
			endpoint: "ws://127.0.0.1:43227/session",
			want:     "127.0.0.1:43227",
			wantOK:   true,
		},
		"not a websocket": {endpoint: "http://127.0.0.1:43227"},
		"empty":           {endpoint: ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, ok := endpointAddress(tt.endpoint)
			if ok != tt.wantOK {
				t.Fatalf("endpointAddress(%q) ok = %v, want %v", tt.endpoint, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("endpointAddress(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestBrowserWaitForPortReturnsWhenNothingListens(t *testing.T) {
	t.Parallel()

	started := time.Now()
	(&browser{endpoint: "ws://127.0.0.1:0/devtools"}).waitForPort(5 * time.Second)

	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("waitForPort took %s against a dead port, want it to return at once", elapsed)
	}
}

func TestBrowserWaitForPortGivesUpOnALiveBrowser(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	started := time.Now()
	(&browser{endpoint: "ws://" + listener.Addr().String() + "/devtools"}).waitForPort(300 * time.Millisecond)

	if elapsed := time.Since(started); elapsed < 300*time.Millisecond {
		t.Errorf("waitForPort returned after %s, want it to wait out its timeout", elapsed)
	}
}
