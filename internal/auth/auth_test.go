package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	a, err := New(st, "", "secret", proxies)
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
	if _, err := New(st, "admin", "", ""); err == nil {
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
