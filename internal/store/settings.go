package store

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Settings is everything in the Settings drawer.
type Settings struct {
	// Site
	Name      string   `json:"name"`
	Private   bool     `json:"private"`
	Sort      string   `json:"sort"`      // author, title, year
	Theme     string   `json:"theme"`     // system, light, dark
	Stats     bool     `json:"stats"`     // visitors may see the stats page
	Permalink string   `json:"permalink"` // isbn, hyphens, title
	Sources   []string `json:"sources"`   // openlibrary, google, goodreads
	Social    Social   `json:"social"`

	// Look
	WallDay   string    `json:"wallDay"`
	WallNight string    `json:"wallNight"`
	WoodDay   string    `json:"woodDay"`
	WoodNight string    `json:"woodNight"`
	Bookcase  bool      `json:"bookcase"`
	CaseParts CaseParts `json:"caseParts"`
	Ribbon    string    `json:"ribbon"`
	Bindings  bool      `json:"bindings"`
	BookSize  int       `json:"bookSize"`  // percent, 70–130
	MaxHeight int       `json:"maxHeight"` // percent, 70–100 (100: no limit)

	// Library
	BackupEvery string `json:"backupEvery"` // day, week, never (only before imports and restores)
	BackupKeep  int    `json:"backupKeep"`
}

// Social links, stored as full profile URLs.
type Social struct {
	Goodreads  string `json:"goodreads"`
	StoryGraph string `json:"storygraph"`
	Instagram  string `json:"instagram"`
}

// CaseParts are the parts of the bookcase that are shown when it is on.
type CaseParts struct {
	Sides bool `json:"sides"`
	Top   bool `json:"top"`
	Back  bool `json:"back"`
}

// DefaultSettings are a new library's settings.
func DefaultSettings() Settings {
	return Settings{
		Name: "Bookshelf", Sort: "author", Theme: "system", Stats: true, Permalink: "isbn", Sources: []string{"openlibrary"},
		WallDay: "#e9e0d0", WallNight: "#17110d", WoodDay: "oak", WoodNight: "walnut",
		CaseParts: CaseParts{Sides: true, Top: true, Back: true}, Ribbon: "#8e2532", Bindings: true, BookSize: 100, MaxHeight: 100,
		BackupEvery: "day", BackupKeep: 14,
	}
}

var (
	hexColor = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	woods    = []string{"oak", "walnut", "maple", "ash", "pine", "cherry", "teak", "mahogany", "ebony"}
	sources  = []string{"openlibrary", "google", "goodreads"}
)

// Validate checks every value and tidies what can be tidied.
func (s *Settings) Validate() error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		s.Name = "Bookshelf"
	}
	if len([]rune(s.Name)) > 80 {
		return fmt.Errorf("the name is longer than 80 characters")
	}
	if !slices.Contains([]string{"author", "title", "year"}, s.Sort) {
		return fmt.Errorf("unknown sort %q", s.Sort)
	}
	if !slices.Contains([]string{"system", "light", "dark"}, s.Theme) {
		return fmt.Errorf("unknown theme %q", s.Theme)
	}
	if !slices.Contains([]string{"isbn", "hyphens", "title"}, s.Permalink) {
		return fmt.Errorf("unknown address style %q", s.Permalink)
	}
	var kept []string
	for _, src := range sources { // fixed order, no duplicates
		if slices.Contains(s.Sources, src) {
			kept = append(kept, src)
		}
	}
	if len(kept) != len(s.Sources) {
		return fmt.Errorf("unknown search source in %v", s.Sources)
	}
	s.Sources = kept
	for name, c := range map[string]*string{"day wall": &s.WallDay, "evening wall": &s.WallNight, "ribbon": &s.Ribbon} {
		*c = strings.ToLower(strings.TrimSpace(*c))
		if !hexColor.MatchString(*c) {
			return fmt.Errorf("the %s color must look like #a1b2c3", name)
		}
	}
	for _, l := range []struct {
		name, host string
		url        *string
	}{{"Goodreads", "goodreads.com", &s.Social.Goodreads}, {"The StoryGraph", "thestorygraph.com", &s.Social.StoryGraph}, {"Instagram", "instagram.com", &s.Social.Instagram}} {
		*l.url = strings.TrimSpace(*l.url)
		if *l.url == "" {
			continue
		}
		u, err := url.Parse(*l.url)
		if err != nil || u.Scheme != "https" || u.User != nil || (u.Hostname() != l.host && !strings.HasSuffix(u.Hostname(), "."+l.host)) {
			return fmt.Errorf("the %s link must be an https address on %s", l.name, l.host)
		}
	}
	if !slices.Contains(woods, s.WoodDay) || !slices.Contains(woods, s.WoodNight) {
		return fmt.Errorf("unknown wood")
	}
	if s.BookSize < 70 || s.BookSize > 130 {
		return fmt.Errorf("book size must be between 70%% and 130%%")
	}
	if s.MaxHeight < 70 || s.MaxHeight > 100 {
		return fmt.Errorf("tallest spine must be between 70%% and 100%%")
	}
	if !slices.Contains([]string{"day", "week", "never"}, s.BackupEvery) {
		return fmt.Errorf("unknown backup schedule %q", s.BackupEvery)
	}
	if s.BackupKeep < 1 || s.BackupKeep > 365 {
		return fmt.Errorf("keep between 1 and 365 backups")
	}
	return nil
}
