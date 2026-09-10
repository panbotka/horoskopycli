package seznam

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// loginURL is the Seznam login screen, told which service is asking and where
// to return afterwards. Landing back on horoskopy.cz is what mints the cookie
// for that domain.
const loginURL = "https://login.seznam.cz/?service=horoskopy" +
	"&return_url=https%3A%2F%2Fwww.horoskopy.cz%2Fp%2Fsnar-vyklad-snu"

// handoffURL is opened when the user is signed in to Seznam but has not been
// returned to horoskopy.cz, which happens when Seznam interrupts the flow with
// a screen of its own.
const handoffURL = "https://www.horoskopy.cz/"

// DefaultLoginTimeout is how long the browser stays open waiting for the user
// to finish signing in.
const DefaultLoginTimeout = 5 * time.Minute

// browserStartTimeout bounds how long the browser may take to open its
// debugging port.
const browserStartTimeout = 30 * time.Second

// cookiePollInterval is how often the browser cookie jar is inspected while
// waiting for the user.
const cookiePollInterval = time.Second

// ErrLoginTimeout is returned when the login was not completed in time.
var ErrLoginTimeout = errors.New("timed out waiting for the login to finish")

// LoginOptions tunes how Login runs.
type LoginOptions struct {
	// Browser is the browser executable to use. Empty means: find one.
	Browser string
	// Timeout is how long to wait for the user. Zero means DefaultLoginTimeout.
	Timeout time.Duration
	// Progress receives a line of reassurance while the browser is open. It
	// may be nil.
	Progress io.Writer
}

// Login opens a browser on the Seznam login screen, waits for the user to sign
// in, and returns the session cookie the browser was given. The browser runs
// on a throwaway profile which is deleted afterwards, so nothing but the
// cookie survives.
//
// It returns ErrNoBrowser when no Chrome-based browser is installed and
// ErrLoginTimeout when the user does not finish in time.
func Login(ctx context.Context, opts LoginOptions) (Session, error) {
	binary, err := findBrowser(opts.Browser)
	if err != nil {
		return Session{}, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultLoginTimeout
	}

	started, err := launchBrowser(ctx, binary, loginURL, browserStartTimeout)
	if err != nil {
		return Session{}, err
	}
	defer started.stop()

	conn, err := dialCDP(started.endpoint)
	if err != nil {
		return Session{}, err
	}
	defer func() {
		// Ask the browser to leave before killing it, so that it takes its
		// helper processes — and the profile holding the login — with it.
		//nolint:errcheck // best effort: the process is killed right after anyway.
		_ = conn.closeBrowser()
		_ = conn.close()
	}()

	if opts.Progress != nil {
		fmt.Fprintf(opts.Progress, "Waiting for the Seznam login in %s…\n", binary)
	}

	return waitForSession(ctx, conn, timeout, cookiePollInterval)
}

// waitForSession polls the browser cookie jar every interval until the
// horoskopy.cz session cookie appears, nudging a stalled login along the way.
func waitForSession(ctx context.Context, conn *cdpConn, timeout, interval time.Duration) (Session, error) {
	deadline := time.Now().Add(timeout)
	nudged := false

	for time.Now().Before(deadline) {
		jar, err := conn.cookies()
		if err != nil {
			return Session{}, err
		}

		if found, ok := sessionCookie(jar); ok {
			return Session{Cookie: found.Value, Expires: found.expiresAt()}, nil
		}

		// Signed in to Seznam, but not carried over to horoskopy.cz yet:
		// opening the site once is what hands the session over.
		if !nudged && signedIn(jar) {
			if err := conn.openTab(handoffURL); err != nil {
				return Session{}, err
			}
			nudged = true
		}

		if err := sleep(ctx, interval); err != nil {
			return Session{}, err
		}
	}

	return Session{}, fmt.Errorf("%w after %s", ErrLoginTimeout, timeout)
}

// sessionCookie picks the horoskopy.cz session cookie out of a cookie jar.
func sessionCookie(jar []cookie) (cookie, bool) {
	for _, candidate := range jar {
		if candidate.Name == CookieName && inDomain(candidate.Domain, cookieDomain) {
			return candidate, true
		}
	}

	return cookie{}, false
}

// signedIn reports whether the jar holds a Seznam session for any domain,
// which means the user is through the login screen.
func signedIn(jar []cookie) bool {
	for _, candidate := range jar {
		if candidate.Name == CookieName {
			return true
		}
	}

	return false
}

// inDomain reports whether a cookie domain is the given domain or one of its
// subdomains, without matching lookalikes such as nothoroskopy.cz.
func inDomain(candidate, domain string) bool {
	candidate = strings.TrimPrefix(candidate, ".")

	return candidate == domain || strings.HasSuffix(candidate, "."+domain)
}

// sleep waits for the given duration, returning early if the context is done.
func sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("login interrupted: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
