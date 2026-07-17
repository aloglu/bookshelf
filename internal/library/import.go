package library

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type ImportOptions struct {
	SkipDuplicates bool
	FetchCovers    bool
	Build          bool
	DryRun         bool
}

type ImportResult struct {
	Imported int
	Skipped  int
	Books    []Book
	Build    BuildStats
}

func DecodeImport(reader io.Reader, format string) ([]Book, error) {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), ".")) {
	case "json":
		return decodeJSONImport(reader)
	case "csv":
		return decodeCSVImport(reader)
	default:
		return nil, fmt.Errorf("unsupported import format %q; use json or csv", format)
	}
}

func Import(ctx context.Context, paths Paths, candidates []Book, options ImportOptions) (ImportResult, error) {
	var result ImportResult
	existing, err := Load(paths)
	if err != nil {
		return result, err
	}
	ids := make(map[string]bool, len(existing)+len(candidates))
	isbns := make(map[string]bool, len(existing)+len(candidates))
	for _, book := range existing {
		ids[book.ID] = true
		if isbn := CleanISBN(book.ISBN); isbn != "" {
			isbns[isbn] = true
		}
	}

	for index, candidate := range candidates {
		candidate = Normalize(candidate)
		if candidate.Title == "" {
			return result, fmt.Errorf("import row %d: title is required", index+1)
		}
		isbn := CleanISBN(candidate.ISBN)
		duplicate := ids[candidate.ID] || (isbn != "" && isbns[isbn])
		if duplicate {
			if options.SkipDuplicates {
				result.Skipped++
				continue
			}
			return result, fmt.Errorf("import row %d (%q): duplicate id or ISBN", index+1, candidate.Title)
		}
		ids[candidate.ID] = true
		if isbn != "" {
			isbns[isbn] = true
		}
		result.Books = append(result.Books, candidate)
	}
	result.Imported = len(result.Books)
	if result.Imported == 0 {
		return result, nil
	}

	allBooks := make([]Book, 0, len(existing)+len(result.Books))
	allBooks = append(allBooks, existing...)
	allBooks = append(allBooks, result.Books...)
	if problems := Validate(allBooks); len(problems) > 0 {
		return ImportResult{}, validationError(problems)
	}
	if options.DryRun {
		return result, nil
	}
	if err := Save(paths, allBooks); err != nil {
		return ImportResult{}, err
	}
	if !options.Build {
		return result, nil
	}
	only := make(map[string]bool, len(result.Books))
	for _, book := range result.Books {
		only[book.Key()] = true
	}
	stats, err := Build(ctx, paths, BuildOptions{
		FetchCovers: options.FetchCovers,
		ProcessOnly: only,
		FetchOnly:   only,
	})
	result.Build = stats
	return result, err
}

func decodeJSONImport(reader io.Reader) ([]Book, error) {
	decoder := json.NewDecoder(reader)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode JSON import: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode JSON import: unexpected data after the first value")
		}
		return nil, fmt.Errorf("decode JSON import: %w", err)
	}
	var books []Book
	if err := json.Unmarshal(raw, &books); err == nil {
		return books, nil
	}
	var envelope struct {
		Books []Book `json:"books"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Books == nil {
		return nil, fmt.Errorf("JSON import must be an array of books or an object containing a books array")
	}
	return envelope.Books, nil
}

func decodeCSVImport(reader io.Reader) ([]Book, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true
	csvReader.FieldsPerRecord = -1
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("decode CSV import: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("CSV import is empty")
	}
	headers := make([]string, len(records[0]))
	for i, header := range records[0] {
		headers[i] = normalizeHeader(strings.TrimPrefix(header, "\ufeff"))
	}
	if !contains(headers, "title") {
		return nil, fmt.Errorf("CSV import requires a title column")
	}
	books := make([]Book, 0, len(records)-1)
	for rowIndex, record := range records[1:] {
		if blankRecord(record) {
			continue
		}
		var input BookInput
		var id string
		var coverFile string
		var spineColor string
		var spineTextColor string
		for column, value := range record {
			if column >= len(headers) {
				continue
			}
			switch headers[column] {
			case "id":
				id = value
			case "title":
				input.Title = value
			case "author":
				input.Author = value
			case "isbn", "isbn10", "isbn13":
				input.ISBN = value
			case "slug", "urlslug", "permalink":
				input.Slug = value
			case "translator":
				input.Translator = value
			case "publisher":
				input.Publisher = value
			case "binding", "format":
				input.Binding = value
			case "published", "year", "publishedyear":
				input.Published = value
			case "coverfile":
				coverFile = value
			case "spinecolor":
				spineColor = value
			case "spinetextcolor":
				spineTextColor = value
			}
		}
		book := FromInput(input)
		book.CoverFile = strings.TrimSpace(coverFile)
		book.SpineColor = strings.TrimSpace(spineColor)
		book.SpineTextColor = strings.TrimSpace(spineTextColor)
		if strings.TrimSpace(id) != "" {
			book.ID = strings.TrimSpace(id)
			book = Normalize(book)
		}
		if book.Title == "" {
			return nil, fmt.Errorf("CSV row %d: title is required", rowIndex+2)
		}
		books = append(books, book)
	}
	return books, nil
}

func normalizeHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "")
	return replacer.Replace(value)
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func blankRecord(record []string) bool {
	for _, value := range record {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
