package store

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Settings is everything in Settings.
type Settings struct {
	// Site
	Name       string   `json:"name"` // may hold inline Markdown (a link, emphasis)
	Private    bool     `json:"private"`
	Sort       string   `json:"sort"`       // author, title, year
	SortDesc   bool     `json:"sortDesc"`   // Z to A, newest first
	Theme      string   `json:"theme"`      // system, light, dark
	Footer     bool     `json:"footer"`     // the line under the shelves is shown
	FooterText string   `json:"footerText"` // what it says, in inline Markdown
	Stats      bool     `json:"stats"`      // visitors may see the stats page
	Permalink  string   `json:"permalink"`  // isbn, hyphens, title
	Sources    []string `json:"sources"`    // openlibrary, google, goodreads
	Icon       string   `json:"icon"`       // the owner's own icon (its name in the data folder), or "" for Bookshelf's
	IconShown  bool     `json:"iconShown"`  // the icon stands beside the library's name in the header
	Social     Social   `json:"social"`

	// Look
	WallDay     string    `json:"wallDay"`
	WallNight   string    `json:"wallNight"`
	WoodDay     string    `json:"woodDay"`
	WoodNight   string    `json:"woodNight"`
	RibbonDay   string    `json:"ribbonDay"`
	RibbonNight string    `json:"ribbonNight"`
	CaseParts   CaseParts `json:"caseParts"`
	// Bookcase and Ribbon are how Bookshelf before 2.0.5 stored the bookcase (one switch over its parts) and
	// the ribbon (one color for day and evening). They are only ever read, from settings and backups saved
	// by those versions (see upgrade).
	Bookcase  *bool  `json:"bookcase,omitempty"`
	Ribbon    string `json:"ribbon,omitempty"`
	Bindings  bool   `json:"bindings"`
	BookSize  int    `json:"bookSize"`  // percent, 70–130
	MaxHeight int    `json:"maxHeight"` // percent, 70–100 (100: no limit)

	// Library
	BackupEvery string `json:"backupEvery"` // day, week, never (only before imports and restores)
	BackupKeep  int    `json:"backupKeep"`

	// About
	CheckUpdates bool `json:"checkUpdates"` // ask GitHub twice a day whether a newer Bookshelf is out
}

// Social links, stored as full profile URLs.
type Social struct {
	Goodreads  string `json:"goodreads"`
	StoryGraph string `json:"storygraph"`
	Instagram  string `json:"instagram"`
}

// CaseParts are the parts of the bookcase, each shown on its own. With none of them, the books stand in
// rows with nothing around or under them.
type CaseParts struct {
	Shelves bool `json:"shelves"` // the planks the books stand on
	Sides   bool `json:"sides"`
	Top     bool `json:"top"`
	Back    bool `json:"back"`
}

// DefaultSettings are a new library's settings.
func DefaultSettings() Settings {
	return Settings{
		Name: "Bookshelf", Sort: "author", Theme: "system", Stats: true, Permalink: "isbn", Sources: []string{"openlibrary"}, IconShown: true,
		WallDay: "#e9e0d0", WallNight: "#17110d", WoodDay: "oak", WoodNight: "walnut",
		CaseParts: CaseParts{Shelves: true}, RibbonDay: "#8e2532", RibbonNight: "#8e2532", Bindings: true, BookSize: 100, MaxHeight: 100,
		BackupEvery: "day", BackupKeep: 14, CheckUpdates: true,
	}
}

var (
	hexColor = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	woods    = []string{"oak", "walnut", "maple", "ash", "pine", "cherry", "teak", "mahogany", "ebony"}
	sources  = []string{"openlibrary", "google", "goodreads"}
	iconName = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// upgrade brings settings saved before 2.0.5 up to date: there the bookcase's parts only showed while its
// one switch was on, so with it off they all go off. Shelves were always shown then, and a setting missing
// from what was saved keeps its default (on). The one ribbon color becomes the day's and the evening's.
func (s *Settings) upgrade() {
	if s.Ribbon != "" {
		s.RibbonDay, s.RibbonNight = s.Ribbon, s.Ribbon
	}
	s.Ribbon = ""
	if s.Bookcase != nil && !*s.Bookcase {
		s.CaseParts.Sides, s.CaseParts.Top, s.CaseParts.Back = false, false, false
	}
	s.Bookcase = nil
}

// Validate checks every value and tidies what can be tidied.
func (s *Settings) Validate() error {
	s.upgrade()
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		s.Name = "Bookshelf"
	}
	if len([]rune(s.Name)) > 200 {
		return fmt.Errorf("the name is longer than 200 characters")
	}
	if PlainText(s.Name) == "" {
		return fmt.Errorf("the name needs some words, not only formatting")
	}
	s.FooterText = strings.TrimSpace(s.FooterText)
	if len([]rune(s.FooterText)) > 1000 {
		return fmt.Errorf("the footer is longer than 1,000 characters")
	}
	if s.Icon != "" && !iconName.MatchString(s.Icon) {
		return fmt.Errorf("unknown icon %q", s.Icon)
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
	for name, c := range map[string]*string{"day wall": &s.WallDay, "evening wall": &s.WallNight, "day ribbon": &s.RibbonDay, "evening ribbon": &s.RibbonNight} {
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
