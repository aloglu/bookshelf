// Package auth signs the owner in: one user name and a password (from the
// environment, or set once with `bookshelf set-password`), week-long sessions
// per device, and a lockout after repeated wrong passwords.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/aloglu/bookshelf/internal/store"
)

const (
	CookieName = "bookshelf_session"
	SessionAge = store.SessionAge

	failureWindow   = 15 * time.Minute
	failuresAllowed = 5 // per address within the window, then locked for the rest of it
	globalSlowAfter = 20
)

// ErrWrong is a wrong user name or password.
var ErrWrong = errors.New("that user name and password don’t match")

// LockedError says how long an address must wait.
type LockedError struct{ Wait time.Duration }

func (e *LockedError) Error() string { return "too many wrong passwords; try again later" }

// Auth checks sign-ins and sessions.
type Auth struct {
	user     [32]byte // SHA-256, so comparisons take the same time whatever the input
	password Password
	userName string
	store    *store.Store
	trusted  []netip.Prefix // proxies whose X-Forwarded-For is believed

	mu       sync.Mutex
	failures map[string][]time.Time // by client address
	recent   []time.Time            // every failure, for the global slow-down
	now      func() time.Time
	sleep    func(time.Duration)
}

// New makes an Auth. trustedProxies is a comma-separated list of addresses
// or networks (for example "172.17.0.0/16, 10.0.0.2") of reverse proxies.
func New(st *store.Store, user string, password Password, trustedProxies string) (*Auth, error) {
	if password == nil {
		return nil, errors.New("no password")
	}
	if user == "" {
		user = "admin"
	}
	a := &Auth{store: st, userName: user, user: sha256.Sum256([]byte(user)), password: password,
		failures: map[string][]time.Time{}, now: time.Now, sleep: time.Sleep}
	for _, part := range strings.Split(trustedProxies, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p, err := netip.ParsePrefix(part)
		if err != nil {
			addr, aerr := netip.ParseAddr(part)
			if aerr != nil {
				return nil, errors.New("BOOKSHELF_TRUSTED_PROXIES: " + part + " is not an address or network")
			}
			p = netip.PrefixFrom(addr, addr.BitLen())
		}
		a.trusted = append(a.trusted, p.Masked())
	}
	return a, nil
}

// User is the configured user name.
func (a *Auth) User() string { return a.userName }

// Client is the address a request came from: the connecting address, or, if
// that is a trusted proxy, the address the proxy says it is forwarding.
func (a *Auth) Client(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	addr = addr.Unmap()
	if !a.isTrusted(addr) {
		return addr.String()
	}
	// The rightmost address not belonging to a trusted proxy is the client.
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		h, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		if h = h.Unmap(); !a.isTrusted(h) {
			return h.String()
		}
	}
	return addr.String()
}

func (a *Auth) isTrusted(addr netip.Addr) bool {
	for _, p := range a.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// Trusted reports whether the request came straight from a trusted proxy, so
// its X-Forwarded-* headers may be believed.
func (a *Auth) Trusted(r *http.Request) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	addr, err := netip.ParseAddr(host)
	return err == nil && a.isTrusted(addr.Unmap())
}

// SignIn checks a user name and password from client and, if they match,
// returns a new session token.
func (a *Auth) SignIn(ctx context.Context, client, user, password, userAgent string) (string, error) {
	if wait := a.attempt(client); wait > 0 {
		return "", &LockedError{Wait: wait}
	}
	a.slowDown()
	u := sha256.Sum256([]byte(user))
	userOK := subtle.ConstantTimeCompare(u[:], a.user[:]) == 1
	passOK := a.password.Match(password) // checked whatever the user name, so a wrong name takes as long
	if !userOK || !passOK {
		return "", ErrWrong
	}
	a.mu.Lock()
	delete(a.failures, client)
	a.mu.Unlock()

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	raw := hex.EncodeToString(token)
	now := a.now()
	err := a.store.AddSession(ctx, store.Session{TokenHash: hash(raw), CreatedAt: now, ExpiresAt: now.Add(SessionAge), LastSeen: now, UserAgent: trimUA(userAgent)})
	return raw, err
}

func trimUA(ua string) string {
	if len(ua) > 200 {
		return ua[:200]
	}
	return ua
}

func hash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// attempt counts a sign-in from client before its password is checked, or
// says how long the client must wait. Counting first, in one step, means
// many attempts sent at once can't all slip in before the first is counted;
// a right password then clears the count.
func (a *Auth) attempt(client string) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked()
	list := a.failures[client]
	if len(list) >= failuresAllowed {
		return list[len(list)-failuresAllowed].Add(failureWindow).Sub(a.now())
	}
	now := a.now()
	a.failures[client] = append(list, now)
	a.recent = append(a.recent, now)
	return 0
}

// slowDown makes every attempt wait a moment while many wrong passwords are
// arriving from anywhere. Unlike a lockout, it never shuts the owner out.
func (a *Auth) slowDown() {
	a.mu.Lock()
	a.pruneLocked()
	busy := len(a.recent) >= globalSlowAfter
	a.mu.Unlock()
	if busy {
		a.sleep(2 * time.Second)
	}
}

func (a *Auth) pruneLocked() {
	cutoff := a.now().Add(-failureWindow)
	for k, list := range a.failures {
		i := 0
		for i < len(list) && list[i].Before(cutoff) {
			i++
		}
		if i == len(list) {
			delete(a.failures, k)
		} else {
			a.failures[k] = list[i:]
		}
	}
	i := 0
	for i < len(a.recent) && a.recent[i].Before(cutoff) {
		i++
	}
	a.recent = a.recent[i:]
}

// Session reports whether a request carries a valid session, and keeps it
// alive: each week of use extends it.
func (a *Auth) Session(r *http.Request) (string, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	h := hash(c.Value)
	ss, ok := a.store.Snapshot().Sessions[h]
	if !ok || a.now().After(ss.ExpiresAt) {
		return "", false
	}
	a.store.TouchSession(r.Context(), h)
	return h, true
}

// SignOut ends a session.
func (a *Auth) SignOut(ctx context.Context, tokenHash string) error {
	return a.store.RemoveSession(ctx, tokenHash)
}
