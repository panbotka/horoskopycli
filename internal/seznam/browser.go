package seznam

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// engine is the automation protocol a browser speaks. Chrome and its
// relatives speak the DevTools protocol; Firefox speaks WebDriver BiDi.
type engine int

const (
	engineChromium engine = iota
	engineFirefox
)

// engineNames maps a substring of an executable name to the protocol its
// browser speaks. Everything Chromium-based answers to the DevTools protocol,
// and every Firefox fork to BiDi.
var engineNames = map[string]engine{
	"firefox":   engineFirefox,
	"waterfox":  engineFirefox,
	"floorp":    engineFirefox,
	"librewolf": engineFirefox,
	"chrome":    engineChromium,
	"chromium":  engineChromium,
	"brave":     engineChromium,
	"edge":      engineChromium,
	"vivaldi":   engineChromium,
	"opera":     engineChromium,
}

// browserCandidates are the browsers looked for in PATH, in order of
// preference. Firefox comes first only because a Firefox user rarely has
// Chrome installed as well, while the reverse is common.
var browserCandidates = []string{
	"firefox",
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
	"brave-browser",
	"microsoft-edge",
}

// portFilePollInterval is how often the profile directory is checked for the
// file a Chromium-based browser writes once its debugging port is open.
const portFilePollInterval = 100 * time.Millisecond

// browserExitTimeout is how long a browser that has been asked to close is
// given to do so before it is killed.
const browserExitTimeout = 5 * time.Second

// profileRemovalDelay is how long to wait before another attempt at deleting
// the throwaway profile, giving a browser's helpers time to finish writing.
const profileRemovalDelay = 500 * time.Millisecond

// profileRemovalAttempts is how many times deleting the profile is retried
// before giving up and leaving it to the operating system's temporary
// directory cleanup.
const profileRemovalAttempts = 3

// bidiAnnouncement is how Firefox says where its remote agent is listening.
const bidiAnnouncement = "WebDriver BiDi listening"

// webSocketURL picks the endpoint out of that announcement.
var webSocketURL = regexp.MustCompile(`ws://\S+`)

// ErrNoBrowser is returned when no supported browser can be found to run the
// login in.
var ErrNoBrowser = errors.New("no supported browser found")

// ErrNoDebugPort is returned when a browser starts but never opens its remote
// control port, so its cookies cannot be read.
var ErrNoDebugPort = errors.New("browser did not open a debugging port")

// foundBrowser is a browser executable and the protocol it speaks.
type foundBrowser struct {
	path   string
	engine engine
}

// browser is a browser process started by this CLI, together with the
// throwaway profile it was given and the endpoint it is driven through.
type browser struct {
	cmd      *exec.Cmd
	profile  string
	endpoint string
	engine   engine
}

// engineFor works out which protocol the executable at path speaks, and
// reports false for a browser this CLI cannot drive, such as Safari.
func engineFor(path string) (engine, bool) {
	name := strings.ToLower(filepath.Base(path))
	name = strings.TrimSuffix(name, ".exe")

	for fragment, spoken := range engineNames {
		if strings.Contains(name, fragment) {
			return spoken, true
		}
	}

	return engineChromium, false
}

// findBrowser returns the browser to run the login in: the one asked for, else
// the user's default browser, else the first supported one installed.
//
// It returns ErrNoBrowser when nothing usable is found.
func findBrowser(preferred string) (foundBrowser, error) {
	if preferred != "" {
		path, err := exec.LookPath(preferred)
		if err != nil {
			return foundBrowser{}, fmt.Errorf("could not use browser %q: %w", preferred, err)
		}

		return describeBrowser(path), nil
	}

	if path, err := defaultBrowserPath(); err == nil {
		if _, supported := engineFor(path); supported {
			return describeBrowser(path), nil
		}
	}

	for _, candidate := range browserCandidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return describeBrowser(path), nil
		}
	}

	for _, candidate := range installedBrowserPaths() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return describeBrowser(candidate), nil
		}
	}

	return foundBrowser{}, fmt.Errorf("%w: tried %s", ErrNoBrowser, strings.Join(browserCandidates, ", "))
}

// describeBrowser pairs an executable with the protocol it speaks. An unknown
// browser is assumed to be Chromium-based, which is the safer guess: every
// mainstream browser except Firefox and Safari is.
func describeBrowser(path string) foundBrowser {
	spoken, _ := engineFor(path)

	return foundBrowser{path: path, engine: spoken}
}

// launchBrowser starts a browser on a fresh throwaway profile with remote
// control enabled, and waits until it can be driven. The caller must call stop
// to shut it down and delete the profile.
func launchBrowser(ctx context.Context, found foundBrowser, url string, timeout time.Duration) (*browser, error) {
	profile, err := newProfile(found)
	if err != nil {
		return nil, err
	}

	//nolint:gosec // the executable is one the user chose or one of the known browsers.
	cmd := exec.CommandContext(ctx, found.path, browserArgs(found.engine, profile, url)...)

	// Firefox announces its endpoint on stderr; Chromium writes it into the
	// profile, so only the former needs the pipe.
	var announcements io.Reader
	if found.engine == engineFirefox {
		announcements, err = cmd.StderrPipe()
		if err != nil {
			_ = os.RemoveAll(profile)

			return nil, fmt.Errorf("could not watch %s: %w", found.path, err)
		}
	}

	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(profile)

		return nil, fmt.Errorf("could not start %s: %w", found.path, err)
	}

	started := &browser{cmd: cmd, profile: profile, engine: found.engine}

	started.endpoint, err = findEndpoint(found.engine, profile, announcements, timeout)
	if err != nil {
		started.stop()

		return nil, err
	}

	return started, nil
}

// browserArgs builds the command line that puts a browser under remote control
// on a profile of its own.
func browserArgs(spoken engine, profile, url string) []string {
	if spoken == engineFirefox {
		// -no-remote implies a new instance, so an already running Firefox
		// neither swallows this window nor is disturbed by it.
		return []string{
			"--profile", profile,
			"--no-remote",
			"--remote-debugging-port=0",
			url,
		}
	}

	return []string{
		"--user-data-dir=" + profile,
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
		"--window-size=520,780",
		url,
	}
}

// newProfile creates the throwaway profile directory.
//
// Snap-packaged browsers cannot see the system /tmp — each snap gets one of
// its own — nor hidden directories in the home directory, so for those the
// profile has to live inside the snap's own directory.
func newProfile(found foundBrowser) (string, error) {
	parent := ""
	if snapDir, ok := snapProfileDir(found.path); ok {
		parent = snapDir
	}

	profile, err := os.MkdirTemp(parent, "horoskopycli-login-")
	if err != nil {
		return "", fmt.Errorf("could not create a browser profile: %w", err)
	}

	return profile, nil
}

// snapProfileDir returns the directory a snap-confined browser can read, if
// the browser looks like a snap.
func snapProfileDir(path string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}

	name := strings.ToLower(filepath.Base(path))

	for _, snap := range []string{"firefox", "chromium"} {
		if !strings.Contains(name, snap) {
			continue
		}

		dir := filepath.Join(home, "snap", snap, "common")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, true
		}
	}

	return "", false
}

// findEndpoint waits for the browser to say where it can be driven: Firefox
// announces it on stderr, Chromium writes it into the profile directory.
func findEndpoint(spoken engine, profile string, announcements io.Reader, timeout time.Duration) (string, error) {
	if spoken == engineFirefox {
		return waitForAnnouncement(announcements, timeout)
	}

	return waitForEndpoint(profile, timeout)
}

// waitForAnnouncement watches a Firefox process's output for the line naming
// its remote agent. It keeps reading afterwards so that the browser never
// blocks on a full pipe.
func waitForAnnouncement(announcements io.Reader, timeout time.Duration) (string, error) {
	announced := make(chan string, 1)

	go func() {
		scanner := bufio.NewScanner(announcements)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.Contains(line, bidiAnnouncement) {
				continue
			}

			select {
			case announced <- webSocketURL.FindString(line):
			default:
			}
		}
	}()

	select {
	case endpoint := <-announced:
		if endpoint == "" {
			return "", fmt.Errorf("%w: could not read the announced address", ErrNoDebugPort)
		}

		return endpoint, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("%w within %s", ErrNoDebugPort, timeout)
	}
}

// waitForEndpoint waits for a Chromium-based browser to write its
// DevToolsActivePort file and turns it into a WebSocket URL.
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

// connect opens a driving session with the browser, whichever protocol it
// speaks.
func (b *browser) connect() (cookieSource, error) {
	if b.engine == engineFirefox {
		return dialBiDi(b.endpoint)
	}

	return dialCDP(b.endpoint)
}

// stop shuts the browser down and removes its throwaway profile, taking the
// login session with it: the cookie the CLI kept is the only thing that
// survives.
func (b *browser) stop() {
	b.waitOrKill(browserExitTimeout)
	b.removeProfile()
}

// waitOrKill gives a browser that has been asked to close time to leave on its
// own, and kills it if it will not.
//
// The wait matters: a browser that exits normally takes its helper processes
// with it, while a killed one leaves them running — and they go on writing to
// the profile directory, which is how a deleted profile comes back.
//
//nolint:errcheck // the browser is being torn down, so a failed kill or wait leaves nothing to act on.
func (b *browser) waitOrKill(timeout time.Duration) {
	if b.cmd == nil || b.cmd.Process == nil {
		return
	}

	exited := make(chan struct{})
	go func() {
		b.cmd.Process.Wait()
		close(exited)
	}()

	select {
	case <-exited:
	case <-time.After(timeout):
		b.cmd.Process.Kill()
		<-exited
	}
}

// removeProfile deletes the throwaway profile, retrying while a straggling
// helper process writes it back out.
//
//nolint:errcheck // a profile that cannot be removed is left to the temporary directory cleanup.
func (b *browser) removeProfile() {
	if b.profile == "" {
		return
	}

	for attempt := range profileRemovalAttempts {
		os.RemoveAll(b.profile)

		if _, err := os.Stat(b.profile); errors.Is(err, os.ErrNotExist) {
			return
		}

		if attempt < profileRemovalAttempts-1 {
			time.Sleep(profileRemovalDelay)
		}
	}
}
