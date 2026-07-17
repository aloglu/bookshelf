package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"

	goslug "github.com/gosimple/slug"
)

var yearPattern = regexp.MustCompile(`\d{4}`)

type Book struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Author         string `json:"author,omitempty"`
	ISBN           string `json:"isbn,omitempty"`
	Slug           string `json:"slug,omitempty"`
	TitleSlug      string `json:"titleSlug,omitempty"`
	Permalink      string `json:"permalink,omitempty"`
	Translator     string `json:"translator,omitempty"`
	Publisher      string `json:"publisher,omitempty"`
	Binding        string `json:"binding,omitempty"`
	Published      *int   `json:"published,omitempty"`
	Cover          string `json:"cover,omitempty"`
	SpineColor     string `json:"spineColor,omitempty"`
	SpineTextColor string `json:"spineTextColor,omitempty"`
}

type BookInput struct {
	Title      string
	Author     string
	ISBN       string
	Slug       string
	Translator string
	Publisher  string
	Binding    string
	Published  string
}

type BookPatch struct {
	Title      *string
	Author     *string
	ISBN       *string
	Slug       *string
	Translator *string
	Publisher  *string
	Binding    *string
	Published  *string
}

func (b *Book) UnmarshalJSON(data []byte) error {
	type wireBook struct {
		ID             json.RawMessage `json:"id"`
		Title          json.RawMessage `json:"title"`
		Author         json.RawMessage `json:"author"`
		ISBN           json.RawMessage `json:"isbn"`
		Slug           json.RawMessage `json:"slug"`
		TitleSlug      json.RawMessage `json:"titleSlug"`
		Permalink      json.RawMessage `json:"permalink"`
		Translator     json.RawMessage `json:"translator"`
		Publisher      json.RawMessage `json:"publisher"`
		Binding        json.RawMessage `json:"binding"`
		Published      json.RawMessage `json:"published"`
		Cover          json.RawMessage `json:"cover"`
		SpineColor     json.RawMessage `json:"spineColor"`
		SpineTextColor json.RawMessage `json:"spineTextColor"`
	}
	var wire wireBook
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*b = Book{
		ID:             flexibleString(wire.ID),
		Title:          flexibleString(wire.Title),
		Author:         flexibleString(wire.Author),
		ISBN:           flexibleString(wire.ISBN),
		Slug:           flexibleString(wire.Slug),
		TitleSlug:      flexibleString(wire.TitleSlug),
		Permalink:      flexibleString(wire.Permalink),
		Translator:     flexibleString(wire.Translator),
		Publisher:      flexibleString(wire.Publisher),
		Binding:        flexibleString(wire.Binding),
		Published:      flexibleYear(wire.Published),
		Cover:          flexibleString(wire.Cover),
		SpineColor:     flexibleString(wire.SpineColor),
		SpineTextColor: flexibleString(wire.SpineTextColor),
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

func flexibleYear(raw json.RawMessage) *int {
	value := flexibleString(raw)
	if value != "" {
		return ParseYear(value)
	}
	var year int
	if err := json.Unmarshal(raw, &year); err == nil {
		return &year
	}
	return nil
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
		return fmt.Sprintf("book-%012x", rand.Uint64()&0xffffffffffff)
	}
	return slug
}

func ParseYear(value string) *int {
	match := yearPattern.FindString(strings.TrimSpace(value))
	if match == "" {
		return nil
	}
	year, err := strconv.Atoi(match)
	if err != nil {
		return nil
	}
	return &year
}

func Normalize(input Book) Book {
	input.ID = strings.TrimSpace(input.ID)
	input.Title = strings.TrimSpace(input.Title)
	input.Author = strings.TrimSpace(input.Author)
	input.ISBN = strings.TrimSpace(input.ISBN)
	input.Slug = strings.TrimSpace(input.Slug)
	input.Translator = strings.TrimSpace(input.Translator)
	input.Publisher = strings.TrimSpace(input.Publisher)
	input.Binding = strings.TrimSpace(input.Binding)
	input.Cover = strings.TrimSpace(input.Cover)
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
		input.ID = Slugify(input.Title + "-" + input.Author)
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
		Title:      input.Title,
		Author:     input.Author,
		ISBN:       input.ISBN,
		Slug:       input.Slug,
		Translator: input.Translator,
		Publisher:  input.Publisher,
		Binding:    input.Binding,
		Published:  ParseYear(input.Published),
	})
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
