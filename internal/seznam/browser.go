package seznam

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// browserCandidates are the Chrome-based browsers looked for in PATH, in order
// of preference. Firefox cannot be used: it speaks a different remote protocol.
var browserCandidates = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
	"brave-browser",
	"microsoft-edge",
}

// browserFallbacks are absolute paths worth trying when nothing is in PATH,
// which is the normal situation on macOS.
var browserFallbacks = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
	"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
}

// portFilePollInterval is how often the profile directory is checked for the
// file the browser writes once its debugging port is open.
const portFilePollInterval = 100 * time.Millisecond

// profileRemovalDelay is how long to wait before a second attempt at deleting
// the throwaway profile, giving a killed browser's helpers time to exit.
const profileRemovalDelay = 500 * time.Millisecond

// ErrNoBrowser is returned when no Chrome-based browser can be found to run
// the login in.
var ErrNoBrowser = errors.New("no Chrome-based browser found")

// ErrNoDebugPort is returned when a browser starts but never opens its
// DevTools port, so its cookies cannot be read.
var ErrNoDebugPort = errors.New("browser did not open a debugging port")

// browser is a browser process started by this CLI, together with the
// throwaway profile it was given and its DevTools endpoint.
type browser struct {
	cmd      *exec.Cmd
	profile  string
	endpoint string
}

// findBrowser returns the browser executable to use. A non-empty preferred
// path is taken as given, otherwise PATH and the well-known install locations
// are searched. It returns ErrNoBrowser when nothing is found.
func findBrowser(preferred string) (string, error) {
	if preferred != "" {
		path, err := exec.LookPath(preferred)
		if err != nil {
			return "", fmt.Errorf("could not use browser %q: %w", preferred, err)
		}

		return path, nil
	}

	for _, candidate := range browserCandidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}

	for _, candidate := range browserFallbacks {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("%w: tried %s", ErrNoBrowser, strings.Join(browserCandidates, ", "))
}

// launchBrowser starts the browser at binary on url with a fresh throwaway
// profile and remote debugging enabled, and waits until its DevTools endpoint
// is ready. The caller must call stop to shut the browser down and delete the
// profile.
func launchBrowser(ctx context.Context, binary, url string, timeout time.Duration) (*browser, error) {
	profile, err := os.MkdirTemp("", "horoskopycli-login-")
	if err != nil {
		return nil, fmt.Errorf("could not create a browser profile: %w", err)
	}

	//nolint:gosec // binary is a browser the user chose or one of the known names.
	cmd := exec.CommandContext(ctx, binary,
		"--user-data-dir="+profile,
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
		"--window-size=520,780",
		url,
	)

	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(profile)

		return nil, fmt.Errorf("could not start %s: %w", binary, err)
	}

	started := &browser{cmd: cmd, profile: profile}

	endpoint, err := waitForEndpoint(profile, timeout)
	if err != nil {
		started.stop()

		return nil, err
	}
	started.endpoint = endpoint

	return started, nil
}

// waitForEndpoint waits for the browser to write its DevToolsActivePort file
// and turns it into a WebSocket URL. It returns ErrNoDebugPort on timeout.
func waitForEndpoint(profile string, timeout time.Duration) (string, error) {
	path := filepath.Join(profile, "DevToolsActivePort")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if endpoint, ok := readEndpoint(path); ok {
			return endpoint, nil
		}

		time.Sleep(portFilePollInterval)
	}

	return "", fmt.Errorf("%w within %s", ErrNoDebugPort, timeout)
}

// readEndpoint parses a DevToolsActivePort file, whose first line is the port
// and whose second line is the browser's WebSocket path. It reports false
// while the file is missing or still incomplete.
func readEndpoint(path string) (string, bool) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is inside a directory this package created.
	if err != nil {
		return "", false
	}

	lines := strings.SplitN(strings.TrimSpace(string(raw)), "\n", 2)
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return "", false
	}

	return fmt.Sprintf("ws://127.0.0.1:%s%s", strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])), true
}

// stop kills the browser and removes its throwaway profile, taking the login
// session with it: the cookie the CLI kept is the only thing that survives.
//
//nolint:errcheck // the browser is being torn down, so a failed kill or wait leaves nothing to act on.
func (b *browser) stop() {
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
		_, _ = b.cmd.Process.Wait()
	}

	if b.profile == "" {
		return
	}

	// A browser that was killed rather than closed may still have helpers
	// writing to the profile, so removing it is worth a second attempt.
	if err := os.RemoveAll(b.profile); err == nil {
		if _, err := os.Stat(b.profile); errors.Is(err, os.ErrNotExist) {
			return
		}
	}

	time.Sleep(profileRemovalDelay)
	_ = os.RemoveAll(b.profile)
}
