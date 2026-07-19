package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type PermalinkStyle string
type WebsiteView string
type WebsiteSort string
type SortDirection string
type ISBNLinkSources string
type ScrollSpeed string

const (
	PermalinkFormattedISBN PermalinkStyle = "formatted-isbn"
	PermalinkCompactISBN   PermalinkStyle = "compact-isbn"
	PermalinkTitleSlug     PermalinkStyle = "title-slug"

	WebsiteViewShelf     WebsiteView = "shelf"
	WebsiteViewStack     WebsiteView = "stack"
	WebsiteViewCoverflow WebsiteView = "coverflow"

	WebsiteSortTitle  WebsiteSort = "title"
	WebsiteSortAuthor WebsiteSort = "author"
	WebsiteSortYear   WebsiteSort = "year"

	SortAscending  SortDirection = "ascending"
	SortDescending SortDirection = "descending"

	ISBNLinksBoth      ISBNLinkSources = "both"
	ISBNLinksWikipedia ISBNLinkSources = "wikipedia"
	ISBNLinksGoodreads ISBNLinkSources = "goodreads"

	ScrollSpeedSlow   ScrollSpeed = "slow"
	ScrollSpeedNormal ScrollSpeed = "normal"
	ScrollSpeedFast   ScrollSpeed = "fast"
)

type Config struct {
	PermalinkStyle   PermalinkStyle  `json:"permalinkStyle"`
	ShowStatistics   bool            `json:"showStatistics"`
	DefaultView      WebsiteView     `json:"defaultView"`
	DefaultSort      WebsiteSort     `json:"defaultSort"`
	DefaultSortOrder SortDirection   `json:"defaultSortOrder"`
	SiteTitle        string          `json:"siteTitle"`
	SiteSubtitle     string          `json:"siteSubtitle"`
	ShowRandom       bool            `json:"showRandom"`
	ShelfScrollSpeed ScrollSpeed     `json:"shelfScrollSpeed"`
	CoverflowSpeed   ScrollSpeed     `json:"coverflowScrollSpeed"`
	ISBNLinkSources  ISBNLinkSources `json:"isbnLinkSources"`
	ShowFooter       bool            `json:"showFooter"`
	FooterText       string          `json:"footerText,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		PermalinkStyle:   PermalinkFormattedISBN,
		ShowStatistics:   true,
		DefaultView:      WebsiteViewShelf,
		DefaultSort:      WebsiteSortTitle,
		DefaultSortOrder: SortAscending,
		SiteTitle:        "Bookshelf",
		SiteSubtitle:     "Click on a book spine to see its details",
		ShowRandom:       true,
		ShelfScrollSpeed: ScrollSpeedNormal,
		CoverflowSpeed:   ScrollSpeedNormal,
		ISBNLinkSources:  ISBNLinksBoth,
		ShowFooter:       true,
	}
}

func LoadConfig(paths Paths) (Config, error) {
	config := DefaultConfig()
	raw, err := os.ReadFile(paths.ConfigJSON)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", paths.ConfigJSON, err)
	}
	config = NormalizeConfig(config)
	if err := ValidateConfig(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func SaveConfig(paths Paths, config Config) error {
	config = NormalizeConfig(config)
	if err := ValidateConfig(config); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return err
	}
	return atomicWrite(paths.ConfigJSON, append(raw, '\n'), 0o644)
}

func NormalizeConfig(config Config) Config {
	config.SiteTitle = NormalizeTypography(config.SiteTitle)
	config.SiteSubtitle = NormalizeTypography(config.SiteSubtitle)
	config.FooterText = NormalizeFooterMarkdown(config.FooterText)
	return config
}

func ValidateConfig(config Config) error {
	if err := ValidatePermalinkStyle(config.PermalinkStyle); err != nil {
		return err
	}
	if err := ValidateWebsiteView(config.DefaultView); err != nil {
		return err
	}
	if err := ValidateWebsiteSort(config.DefaultSort); err != nil {
		return err
	}
	if err := ValidateSortDirection(config.DefaultSortOrder); err != nil {
		return err
	}
	if err := ValidateISBNLinkSources(config.ISBNLinkSources); err != nil {
		return err
	}
	if err := ValidateScrollSpeed(config.ShelfScrollSpeed); err != nil {
		return fmt.Errorf("invalid shelf scroll speed: %w", err)
	}
	if err := ValidateScrollSpeed(config.CoverflowSpeed); err != nil {
		return fmt.Errorf("invalid Coverflow scroll speed: %w", err)
	}
	if strings.TrimSpace(config.SiteTitle) == "" {
		return fmt.Errorf("website title cannot be empty")
	}
	return nil
}

func ValidatePermalinkStyle(style PermalinkStyle) error {
	switch style {
	case PermalinkFormattedISBN, PermalinkCompactISBN, PermalinkTitleSlug:
		return nil
	default:
		return fmt.Errorf("invalid permalink style %q; use formatted-isbn, compact-isbn, or title-slug", style)
	}
}

func ParsePermalinkStyle(value string) (PermalinkStyle, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "formatted-isbn", "formatted", "isbn":
		return PermalinkFormattedISBN, nil
	case "compact-isbn", "compact":
		return PermalinkCompactISBN, nil
	case "title-slug", "title":
		return PermalinkTitleSlug, nil
	default:
		return "", fmt.Errorf("invalid permalink style %q; use formatted-isbn, compact-isbn, or title-slug", value)
	}
}

func ValidateWebsiteView(view WebsiteView) error {
	switch view {
	case WebsiteViewShelf, WebsiteViewStack, WebsiteViewCoverflow:
		return nil
	default:
		return fmt.Errorf("invalid default website view %q; use shelf, stack, or coverflow", view)
	}
}

func ParseWebsiteView(value string) (WebsiteView, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "shelf":
		return WebsiteViewShelf, nil
	case "stack", "stacks":
		return WebsiteViewStack, nil
	case "coverflow", "covers":
		return WebsiteViewCoverflow, nil
	default:
		return "", fmt.Errorf("invalid default website view %q; use shelf, stack, or coverflow", value)
	}
}

func ValidateWebsiteSort(sort WebsiteSort) error {
	switch sort {
	case WebsiteSortTitle, WebsiteSortAuthor, WebsiteSortYear:
		return nil
	default:
		return fmt.Errorf("invalid default website sort %q; use title, author, or year", sort)
	}
}

func ParseWebsiteSort(value string) (WebsiteSort, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "title":
		return WebsiteSortTitle, nil
	case "author":
		return WebsiteSortAuthor, nil
	case "year", "published":
		return WebsiteSortYear, nil
	default:
		return "", fmt.Errorf("invalid default website sort %q; use title, author, or year", value)
	}
}

func ValidateSortDirection(direction SortDirection) error {
	switch direction {
	case SortAscending, SortDescending:
		return nil
	default:
		return fmt.Errorf("invalid default sort direction %q; use ascending or descending", direction)
	}
}

func ParseSortDirection(value string) (SortDirection, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ascending", "asc":
		return SortAscending, nil
	case "descending", "desc":
		return SortDescending, nil
	default:
		return "", fmt.Errorf("invalid default sort direction %q; use ascending or descending", value)
	}
}

func ValidateISBNLinkSources(sources ISBNLinkSources) error {
	switch sources {
	case ISBNLinksBoth, ISBNLinksWikipedia, ISBNLinksGoodreads:
		return nil
	default:
		return fmt.Errorf("invalid ISBN link sources %q; use both, wikipedia, or goodreads", sources)
	}
}

func ParseISBNLinkSources(value string) (ISBNLinkSources, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "both":
		return ISBNLinksBoth, nil
	case "wikipedia", "wiki":
		return ISBNLinksWikipedia, nil
	case "goodreads":
		return ISBNLinksGoodreads, nil
	default:
		return "", fmt.Errorf("invalid ISBN link sources %q; use both, wikipedia, or goodreads", value)
	}
}

func ValidateScrollSpeed(speed ScrollSpeed) error {
	switch speed {
	case ScrollSpeedSlow, ScrollSpeedNormal, ScrollSpeedFast:
		return nil
	default:
		return fmt.Errorf("%q; use slow, normal, or fast", speed)
	}
}

func ParseScrollSpeed(value string) (ScrollSpeed, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "slow":
		return ScrollSpeedSlow, nil
	case "normal", "default":
		return ScrollSpeedNormal, nil
	case "fast":
		return ScrollSpeedFast, nil
	default:
		return "", fmt.Errorf("invalid scroll speed %q; use slow, normal, or fast", value)
	}
}
