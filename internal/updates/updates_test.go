package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"2.0.1", "2.0.0", true}, {"2.1.0", "2.0.9", true}, {"3.0.0", "2.9.9", true},
		{"2.0.0", "2.0.0", false}, {"2.0.0", "2.0.1", false}, {"2.10.0", "2.9.0", true},
		{"2.1.0", "2.1.0-rc.1", true}, {"2.1.0-rc.1", "2.1.0", false},
		{"2.0.0", "dev", false}, {"nonsense", "2.0.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestCheckAsksGitHubForTheVersionOnly(t *testing.T) {
	var asked *http.Request
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r
		w.Write([]byte(`{"tag_name":"v2.1.0","html_url":"https://github.com/aloglu/bookshelf/releases/tag/v2.1.0","body":"notes"}`))
	}))
	defer gh.Close()

	c := New("2.0.0", func() bool { return true }, nil)
	c.URL = gh.URL
	c.Check(context.Background())
	st := c.Status()
	if !st.Newer || st.Latest != "2.1.0" || st.URL != "https://github.com/aloglu/bookshelf/releases/tag/v2.1.0" || st.CheckedAt.IsZero() {
		t.Fatalf("status: %+v", st)
	}
	if asked.Method != "GET" || asked.URL.RawQuery != "" || asked.Header.Get("User-Agent") != "Bookshelf/2.0.0" || asked.Header.Get("Cookie") != "" {
		t.Fatalf("the request carried more than the version: %v %v", asked.URL, asked.Header)
	}

	// A failed check keeps the last answer and says why.
	gh.Close()
	c.Check(context.Background())
	if st := c.Status(); st.Latest != "2.1.0" || st.Problem == "" {
		t.Fatalf("after a failure: %+v", st)
	}
}

func TestADevelopmentBuildIsNeverOutOfDate(t *testing.T) {
	c := New("dev", func() bool { return true }, nil)
	if st := c.Status(); !st.Dev || st.Newer {
		t.Fatalf("dev: %+v", st)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Run(ctx) // returns at once, without asking anyone
}
