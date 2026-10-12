// Package updates finds out whether a newer Bookshelf has been released. It
// asks GitHub which release is the latest at most twice a day, while the
// owner leaves the check on; only the version is asked for, and nothing about
// the library is sent.
package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LatestURL is GitHub's answer to "which release is the latest" (drafts and
// pre-releases are never it).
const LatestURL = "https://api.github.com/repos/aloglu/bookshelf/releases/latest"

// Every is how often the check runs.
const Every = 12 * time.Hour

// Status is what the owner is shown.
type Status struct {
	Version   string    `json:"version"`            // the one running
	Latest    string    `json:"latest,omitempty"`   // the newest release, once known
	Newer     bool      `json:"newer"`              // Latest is newer than Version
	URL       string    `json:"url,omitempty"`      // the newest release's page, with what's new
	CheckedAt time.Time `json:"checkedAt,omitzero"` // the last answer from GitHub
	Problem   string    `json:"problem,omitempty"`  // why the last check failed, if it did
	Dev       bool      `json:"dev,omitempty"`      // a development build, never out of date
}

// Checker keeps the latest answer.
type Checker struct {
	version string
	enabled func() bool
	log     *slog.Logger
	URL     string
	HTTP    *http.Client

	mu     sync.Mutex
	status Status
	kick   chan struct{}
}

// New makes a Checker for the running version; enabled says whether the
// owner wants the check (Settings).
func New(version string, enabled func() bool, log *slog.Logger) *Checker {
	return &Checker{version: version, enabled: enabled, log: log, URL: LatestURL,
		HTTP: &http.Client{Timeout: 15 * time.Second}, status: Status{Version: version, Dev: !isRelease(version)},
		kick: make(chan struct{}, 1)}
}

// Status is the latest answer.
func (c *Checker) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// CheckSoon asks for a check now, for when the owner turns the check on.
func (c *Checker) CheckSoon() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// Run checks at start and every Every, and whenever CheckSoon asks, until ctx ends.
func (c *Checker) Run(ctx context.Context) {
	if !isRelease(c.version) {
		return // a development build has no release to compare with
	}
	t := time.NewTicker(Every)
	defer t.Stop()
	for {
		if c.enabled() {
			c.Check(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-c.kick:
		}
	}
}

// Check asks GitHub once.
func (c *Checker) Check(ctx context.Context) {
	latest, url, err := c.fetch(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.status.Problem = err.Error()
		if c.log != nil {
			c.log.Warn("update check failed", "err", err)
		}
		return
	}
	c.status = Status{Version: c.version, Latest: latest, Newer: Newer(latest, c.version), URL: url, CheckedAt: time.Now()}
}

func (c *Checker) fetch(ctx context.Context) (version, url string, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.URL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Bookshelf/"+c.version)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("GitHub couldn’t be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var rel struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", "", fmt.Errorf("GitHub’s answer couldn’t be read")
	}
	v := strings.TrimPrefix(rel.Tag, "v")
	if !isRelease(v) {
		return "", "", fmt.Errorf("the latest release has no version number (%q)", rel.Tag)
	}
	if !strings.HasPrefix(rel.URL, "https://github.com/") {
		rel.URL = "https://github.com/aloglu/bookshelf/releases/latest"
	}
	return v, rel.URL, nil
}

// version is major.minor.patch with an optional pre-release (2.1.0-rc.1).
type version struct {
	n   [3]int
	pre string
}

func parse(s string) (version, bool) {
	var v version
	s, v.pre, _ = strings.Cut(s, "-")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v.n[i] = n
	}
	return v, true
}

func isRelease(s string) bool { _, ok := parse(s); return ok }

// Newer reports whether release a is newer than b. A release is newer than
// its own pre-releases (2.1.0 after 2.1.0-rc.1).
func Newer(a, b string) bool {
	va, okA := parse(a)
	vb, okB := parse(b)
	if !okA || !okB {
		return false
	}
	for i := range va.n {
		if va.n[i] != vb.n[i] {
			return va.n[i] > vb.n[i]
		}
	}
	return va.pre == "" && vb.pre != ""
}
