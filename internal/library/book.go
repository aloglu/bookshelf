package library

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"

	goslug "github.com/gosimple/slug"
)

var strictYearPattern = regexp.MustCompile(`^\d{4}$`)
var legacyYearPattern = regexp.MustCompile(`(?i)^published(?:\s+in)?\s+(\d{4})$`)

type WebsiteVisibility string

const (
	WebsiteVisible WebsiteVisibility = "visible"
	WebsiteHidden  WebsiteVisibility = "hidden"
)

type Book struct {
	ID                string            `json:"id"`
	Title             string            `json:"title"`
	Author            string            `json:"author,omitempty"`
	ISBN              string            `json:"isbn,omitempty"`
	Slug              string            `json:"slug,omitempty"`
	TitleSlug         string            `json:"titleSlug,omitempty"`
	Permalink         string            `json:"permalink,omitempty"`
	Translator        string            `json:"translator,omitempty"`
	Publisher         string            `json:"publisher,omitempty"`
	Binding           string            `json:"binding,omitempty"`
	Published         *int              `json:"published,omitempty"`
	WebsiteVisibility WebsiteVisibility `json:"websiteVisibility"`
	CoverFile         string            `json:"coverFile,omitempty"`
	Cover             string            `json:"cover,omitempty"`
	Thumbnail         string            `json:"thumbnail,omitempty"`
	SpineColor        string            `json:"spineColor,omitempty"`
	SpineTextColor    string            `json:"spineTextColor,omitempty"`
}

type BookInput struct {
	Title             string
	Author            string
	ISBN              string
	Slug              string
	Translator        string
	Publisher         string
	Binding           string
	Published         string
	WebsiteVisibility string
}

type BookPatch struct {
	Title             *string
	Author            *string
	ISBN              *string
	Slug              *string
	Translator        *string
	Publisher         *string
	Binding           *string
	Published         *string
	WebsiteVisibility *string
}

func (b *Book) UnmarshalJSON(data []byte) error {
	type wireBook struct {
		ID                json.RawMessage `json:"id"`
		Title             json.RawMessage `json:"title"`
		Author            json.RawMessage `json:"author"`
		ISBN              json.RawMessage `json:"isbn"`
		Slug              json.RawMessage `json:"slug"`
		TitleSlug         json.RawMessage `json:"titleSlug"`
		Permalink         json.RawMessage `json:"permalink"`
		Translator        json.RawMessage `json:"translator"`
		Publisher         json.RawMessage `json:"publisher"`
		Binding           json.RawMessage `json:"binding"`
		Published         json.RawMessage `json:"published"`
		WebsiteVisibility json.RawMessage `json:"websiteVisibility"`
		CoverFile         json.RawMessage `json:"coverFile"`
		Cover             json.RawMessage `json:"cover"`
		Thumbnail         json.RawMessage `json:"thumbnail"`
		SpineColor        json.RawMessage `json:"spineColor"`
		SpineTextColor    json.RawMessage `json:"spineTextColor"`
	}
	var wire wireBook
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	published, err := flexibleYear(wire.Published)
	if err != nil {
		return fmt.Errorf("invalid published year: %w", err)
	}
	*b = Book{
		ID:                flexibleString(wire.ID),
		Title:             flexibleString(wire.Title),
		Author:            flexibleString(wire.Author),
		ISBN:              flexibleString(wire.ISBN),
		Slug:              flexibleString(wire.Slug),
		TitleSlug:         flexibleString(wire.TitleSlug),
		Permalink:         flexibleString(wire.Permalink),
		Translator:        flexibleString(wire.Translator),
		Publisher:         flexibleString(wire.Publisher),
		Binding:           flexibleString(wire.Binding),
		Published:         published,
		WebsiteVisibility: WebsiteVisibility(flexibleString(wire.WebsiteVisibility)),
		CoverFile:         flexibleString(wire.CoverFile),
		Cover:             flexibleString(wire.Cover),
		Thumbnail:         flexibleString(wire.Thumbnail),
		SpineColor:        flexibleString(wire.SpineColor),
		SpineTextColor:    flexibleString(wire.SpineTextColor),
	}
	return nil
}

func flexibleString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	return ""
}

func flexibleYear(raw json.RawMessage) (*int, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	value := flexibleString(raw)
	if value != "" {
		if year, err := ParseYearInput(value); err == nil {
			return year, nil
		}
		if match := legacyYearPattern.FindStringSubmatch(strings.TrimSpace(value)); len(match) == 2 {
			return ParseYearInput(match[1])
		}
		return nil, fmt.Errorf("%q must be a four-digit year", value)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil && strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("must be a four-digit year")
}

func CleanISBN(value string) string {
	var out strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			out.WriteRune(r)
		} else if r == 'x' || r == 'X' {
			out.WriteByte('X')
		}
	}
	return out.String()
}

func Slugify(value string) string {
	value = strings.NewReplacer("'", "", "’", "", "‘", "", "ʼ", "").Replace(value)
	slug := goslug.Make(value)
	const maxSlugLength = 80
	if len(slug) > maxSlugLength {
		slug = strings.TrimRight(slug[:maxSlugLength], "-")
		if boundary := strings.LastIndexByte(slug, '-'); boundary >= maxSlugLength/2 {
			slug = slug[:boundary]
		}
	}
	if slug == "" {
		digest := sha256.Sum256([]byte(value))
		return fmt.Sprintf("book-%x", digest[:6])
	}
	return slug
}

func ParseYear(value string) *int {
	year, _ := ParseYearInput(value)
	return year
}

func ParseYearInput(value string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if !strictYearPattern.MatchString(value) {
		return nil, fmt.Errorf("published year must be exactly four digits")
	}
	year, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("parse published year: %w", err)
	}
	return &year, nil
}

func Normalize(input Book) Book {
	input.ID = strings.TrimSpace(input.ID)
	input.Title = NormalizeTypography(strings.TrimSpace(input.Title))
	input.Author = NormalizeTypography(strings.TrimSpace(input.Author))
	input.ISBN = strings.TrimSpace(input.ISBN)
	input.Slug = strings.TrimSpace(input.Slug)
	input.Translator = NormalizeTypography(strings.TrimSpace(input.Translator))
	input.Publisher = NormalizeTypography(strings.TrimSpace(input.Publisher))
	input.Binding = NormalizeTypography(strings.TrimSpace(input.Binding))
	input.WebsiteVisibility = WebsiteVisibility(strings.ToLower(strings.TrimSpace(string(input.WebsiteVisibility))))
	if input.WebsiteVisibility == "" {
		input.WebsiteVisibility = WebsiteVisible
	}
	input.CoverFile = strings.TrimSpace(input.CoverFile)
	input.Cover = strings.TrimSpace(input.Cover)
	input.Thumbnail = strings.TrimSpace(input.Thumbnail)
	input.SpineColor = strings.TrimSpace(input.SpineColor)
	input.SpineTextColor = strings.TrimSpace(input.SpineTextColor)
	input.Permalink = strings.TrimSpace(input.Permalink)
	input.TitleSlug = Slugify(input.Title)
	if input.Slug != "" {
		input.Slug = Slugify(input.Slug)
	}
	if input.ID == "" {
		input.ID = CleanISBN(input.ISBN)
	}
	if input.ID == "" && input.Slug != "" {
		input.ID = input.Slug
	}
	if input.ID == "" {
		identity := input.Title
		if input.Author != "" {
			identity += "-" + input.Author
		}
		input.ID = Slugify(identity)
	}
	return input
}

func PreferredPermalink(book Book, style PermalinkStyle) string {
	if book.Slug != "" {
		return book.Slug
	}
	if style == PermalinkTitleSlug && book.TitleSlug != "" {
		return book.TitleSlug
	}
	if book.ISBN != "" {
		if style == PermalinkCompactISBN {
			return CleanISBN(book.ISBN)
		}
		return book.ISBN
	}
	if book.TitleSlug != "" {
		return book.TitleSlug
	}
	return book.ID
}

func FromInput(input BookInput) Book {
	return Normalize(Book{
		Title:             input.Title,
		Author:            input.Author,
		ISBN:              input.ISBN,
		Slug:              input.Slug,
		Translator:        input.Translator,
		Publisher:         input.Publisher,
		Binding:           input.Binding,
		Published:         ParseYear(input.Published),
		WebsiteVisibility: WebsiteVisibility(input.WebsiteVisibility),
	})
}

func (b Book) VisibleOnWebsite() bool {
	return NormalizeWebsiteVisibility(b.WebsiteVisibility) == WebsiteVisible
}

func NormalizeWebsiteVisibility(value WebsiteVisibility) WebsiteVisibility {
	value = WebsiteVisibility(strings.ToLower(strings.TrimSpace(string(value))))
	if value == "" {
		return WebsiteVisible
	}
	return value
}

func ParseWebsiteVisibility(value string) (WebsiteVisibility, error) {
	visibility := NormalizeWebsiteVisibility(WebsiteVisibility(value))
	switch visibility {
	case WebsiteVisible, WebsiteHidden:
		return visibility, nil
	default:
		return "", fmt.Errorf("website visibility must be visible or hidden")
	}
}

func (b Book) Key() string {
	if isbn := CleanISBN(b.ISBN); isbn != "" {
		return isbn
	}
	return b.ID
}

func (b Book) Year() string {
	if b.Published == nil {
		return ""
	}
	return strconv.Itoa(*b.Published)
}

func AssignTitleSlugs(books []Book) {
	counts := make(map[string]int, len(books))
	reserved := make(map[string]int, len(books)*3)
	for index := range books {
		books[index].TitleSlug = Slugify(books[index].Title)
		counts[books[index].TitleSlug]++
		for _, token := range []string{books[index].Slug, books[index].ID, books[index].ISBN, CleanISBN(books[index].ISBN)} {
			if token != "" {
				reserved[strings.ToLower(token)] = index
			}
		}
	}
	for index := range books {
		owner, reservedCollision := reserved[strings.ToLower(books[index].TitleSlug)]
		if counts[books[index].TitleSlug] < 2 && (!reservedCollision || owner == index) {
			continue
		}
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(books[index].ID))
		base := books[index].TitleSlug
		if len(base) > 71 {
			base = strings.TrimRight(base[:71], "-")
		}
		books[index].TitleSlug = fmt.Sprintf("%s-%08x", base, hash.Sum32())
	}
}
