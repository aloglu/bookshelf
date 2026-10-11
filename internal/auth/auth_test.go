package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aloglu/bookshelf/internal/store"
)

func setup(t *testing.T, proxies string) (*Auth, *time.Time) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a, err := New(st, "", Plain("secret"), proxies)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	a.sleep = func(time.Duration) {}
	return a, &now
}

func TestNeedsAPassword(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	defer st.Close()
	if _, err := New(st, "admin", nil, ""); err == nil {
		t.Fatal("started without a password")
	}
}

func TestSignInAndSession(t *testing.T) {
	a, _ := setup(t, "")
	ctx := context.Background()
	if _, err := a.SignIn(ctx, "1.2.3.4", "admin", "nope", ""); !errors.Is(err, ErrWrong) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := a.SignIn(ctx, "1.2.3.4", "root", "secret", ""); !errors.Is(err, ErrWrong) {
		t.Fatalf("wrong user: %v", err)
	}
	token, err := a.SignIn(ctx, "1.2.3.4", "admin", "secret", "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	h, ok := a.Session(r)
	if !ok {
		t.Fatal("a fresh session was refused")
	}
	if err := a.SignOut(ctx, h); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Session(r); ok {
		t.Fatal("a signed-out session still works")
	}
}

func TestLockoutIsPerAddressAndExpires(t *testing.T) {
	a, now := setup(t, "")
	ctx := context.Background()
	for range failuresAllowed {
		a.SignIn(ctx, "6.6.6.6", "admin", "guess", "")
	}
	var locked *LockedError
	if _, err := a.SignIn(ctx, "6.6.6.6", "admin", "secret", ""); !errors.As(err, &locked) {
		t.Fatalf("not locked after %d wrong passwords: %v", failuresAllowed, err)
	}
	// Someone else is not locked out.
	if _, err := a.SignIn(ctx, "1.2.3.4", "admin", "secret", ""); err != nil {
		t.Fatalf("the owner was locked out by another address: %v", err)
	}
	*now = now.Add(failureWindow + time.Second)
	if _, err := a.SignIn(ctx, "6.6.6.6", "admin", "secret", ""); err != nil {
		t.Fatalf("still locked after the window: %v", err)
	}
	if len(a.failures) != 0 {
		t.Fatal("old failures were not pruned")
	}
}

func TestClientBehindATrustedProxy(t *testing.T) {
	a, _ := setup(t, "172.17.0.0/16")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.17.0.5:4000"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 172.17.0.9")
	if got := a.Client(r); got != "203.0.113.9" {
		t.Fatalf("client behind proxy = %s", got)
	}
	// An untrusted sender can't pick its own address.
	r.RemoteAddr = "198.51.100.7:4000"
	if got := a.Client(r); got != "198.51.100.7" {
		t.Fatalf("spoofed client = %s", got)
	}
}

func TestAttemptsAtOnceAreCountedToo(t *testing.T) {
	a, now := setup(t, "")
	// Many wrong passwords from elsewhere switch on the global slow-down, so
	// each attempt pauses before its password is checked: the moment in
	// which attempts sent together could all get past an uncounted check.
	for range globalSlowAfter {
		a.recent = append(a.recent, *now)
	}
	a.sleep = func(time.Duration) { time.Sleep(20 * time.Millisecond) }
	var wg sync.WaitGroup
	var mu sync.Mutex
	tried := 0
	for range 50 {
		wg.Go(func() {
			_, err := a.SignIn(context.Background(), "203.0.113.9", "admin", "wrong", "")
			if errors.Is(err, ErrWrong) {
				mu.Lock()
				tried++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if tried != failuresAllowed {
		t.Fatalf("%d passwords were tried at once, want %d", tried, failuresAllowed)
	}
}

func TestASetPasswordIsKeptHashed(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadPassword(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no password yet: %v", err)
	}
	if err := SavePassword(dir, "short"); err == nil {
		t.Fatal("a 5-character password was taken")
	}
	if err := SavePassword(dir, "a long password"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, PasswordFile))
	if strings.Contains(string(raw), "a long password") || !strings.HasPrefix(string(raw), "pbkdf2-sha256$") {
		t.Fatalf("the file holds %q", raw)
	}
	if info, _ := os.Stat(filepath.Join(dir, PasswordFile)); info.Mode().Perm() != 0o600 {
		t.Fatalf("the file is %v", info.Mode().Perm())
	}
	pw, err := LoadPassword(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !pw.Match("a long password") || pw.Match("a long passworD") || pw.Match("") {
		t.Fatal("the stored password matches the wrong things")
	}
	os.WriteFile(filepath.Join(dir, PasswordFile), []byte("hunter2\n"), 0o600)
	if _, err := LoadPassword(dir); err == nil {
		t.Fatal("a file Bookshelf didn't write was taken as a password")
	}
}
