package library

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var exportCSVHeaders = []string{
	"ID",
	"Title",
	"Author",
	"ISBN",
	"URL Slug",
	"Translator",
	"Publisher",
	"Binding",
	"Published",
	"Cover File",
	"Spine Color",
	"Spine Text Color",
}

func EncodeExport(writer io.Writer, books []Book, format string) error {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), ".")) {
	case "json":
		return encodeJSONExport(writer, books)
	case "csv":
		return encodeCSVExport(writer, books)
	default:
		return fmt.Errorf("unsupported export format %q; use json or csv", format)
	}
}

func encodeJSONExport(writer io.Writer, books []Book) error {
	exported := exportBooks(books)
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "    ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(exported); err != nil {
		return fmt.Errorf("encode JSON export: %w", err)
	}
	return nil
}

func encodeCSVExport(writer io.Writer, books []Book) error {
	if _, err := io.WriteString(writer, "\ufeff"); err != nil {
		return fmt.Errorf("write CSV byte-order mark: %w", err)
	}
	csvWriter := csv.NewWriter(writer)
	csvWriter.UseCRLF = true
	if err := csvWriter.Write(exportCSVHeaders); err != nil {
		return fmt.Errorf("encode CSV export: %w", err)
	}
	for _, book := range exportBooks(books) {
		published := ""
		if book.Published != nil {
			published = strconv.Itoa(*book.Published)
		}
		record := []string{
			book.ID,
			book.Title,
			book.Author,
			book.ISBN,
			book.Slug,
			book.Translator,
			book.Publisher,
			book.Binding,
			published,
			book.CoverFile,
			book.SpineColor,
			book.SpineTextColor,
		}
		for index := range record {
			record[index] = protectSpreadsheetCell(record[index])
		}
		if err := csvWriter.Write(record); err != nil {
			return fmt.Errorf("encode CSV export: %w", err)
		}
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("encode CSV export: %w", err)
	}
	return nil
}

func protectSpreadsheetCell(value string) string {
	if value == "" || !strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return value
	}
	return "'" + value
}

func exportBooks(books []Book) []Book {
	exported := append([]Book(nil), books...)
	for index := range exported {
		exported[index].TitleSlug = ""
		exported[index].Permalink = ""
		exported[index].Cover = ""
		exported[index].Thumbnail = ""
	}
	return exported
}
