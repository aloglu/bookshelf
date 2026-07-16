package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

var yearPattern = regexp.MustCompile(`\d{4}`)

type Book struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Author         string `json:"author,omitempty"`
	ISBN           string `json:"isbn,omitempty"`
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
	Translator string
	Publisher  string
	Binding    string
	Published  string
}

type BookPatch struct {
	Title      *string
	Author     *string
	ISBN       *string
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
	value = strings.ToLower(value)
	var out strings.Builder
	dash := false
	for _, r := range value {
		isASCIIAlpha := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if isASCIIAlpha || isDigit {
			out.WriteRune(r)
			dash = false
		} else if out.Len() > 0 && !dash {
			out.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(out.String(), "-")
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
	input.Translator = strings.TrimSpace(input.Translator)
	input.Publisher = strings.TrimSpace(input.Publisher)
	input.Binding = strings.TrimSpace(input.Binding)
	input.Cover = strings.TrimSpace(input.Cover)
	input.SpineColor = strings.TrimSpace(input.SpineColor)
	input.SpineTextColor = strings.TrimSpace(input.SpineTextColor)
	if input.ID == "" {
		input.ID = CleanISBN(input.ISBN)
	}
	if input.ID == "" {
		input.ID = Slugify(input.Title + "-" + input.Author)
	}
	return input
}

func FromInput(input BookInput) Book {
	return Normalize(Book{
		Title:      input.Title,
		Author:     input.Author,
		ISBN:       input.ISBN,
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
