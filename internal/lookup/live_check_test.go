package lookup

import (
	"context"
	"os"
	"testing"
)

// Run with BOOKSHELF_LIVE=1 to ask the real services.
func TestLive(t *testing.T) {
	if os.Getenv("BOOKSHELF_LIVE") == "" {
		t.Skip("set BOOKSHELF_LIVE=1")
	}
	c := New("", "dev")
	for _, isbn := range []string{"9780679723424", "9780141439549", "9780060883287"} {
		d, err := c.Find(context.Background(), isbn)
		t.Logf("%s: %+v err=%v", isbn, d, err)
		if d.CoverURL != "" {
			body, err := c.FetchCover(context.Background(), d.CoverURL)
			if err != nil {
				t.Errorf("cover: %v", err)
				continue
			}
			body.Close()
		}
	}
}
