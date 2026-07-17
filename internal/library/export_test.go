package library

import (
	"bytes"
	"strings"
	"testing"
)

func TestCSVExportIsExcelCompatibleAndImportable(t *testing.T) {
	year := 2024
	book := Normalize(Book{
		ID:             "turkish-book",
		Title:          "100. Yılında Cumhuriyet’in Haritası",
		Author:         "Çağdaş Yazar",
		ISBN:           "978-0-00-000000-0",
		Slug:           "cumhuriyet-haritasi",
		Translator:     "Şule Çevirmen",
		Publisher:      "Örnek Yayınları",
		Binding:        "Kâğıt kapak",
		Published:      &year,
		CoverFile:      "9780000000000.jpg",
		SpineColor:     "#123456",
		SpineTextColor: "#FFFFFF",
		Cover:          "data/covers/9780000000000.jpg",
	})
	var output bytes.Buffer
	if err := EncodeExport(&output, []Book{book}, "csv"); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(output.Bytes(), []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("CSV export does not start with a UTF-8 byte-order mark")
	}
	if !strings.Contains(output.String(), "\r\n") {
		t.Fatal("CSV export does not use Excel-compatible CRLF line endings")
	}

	decoded, err := DecodeImport(bytes.NewReader(output.Bytes()), "csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded books = %d", len(decoded))
	}
	got := decoded[0]
	if got.Title != book.Title ||
		got.Author != book.Author ||
		got.CoverFile != book.CoverFile ||
		got.SpineColor != book.SpineColor ||
		got.SpineTextColor != book.SpineTextColor {
		t.Fatalf("CSV round trip = %#v", got)
	}
}

func TestJSONExportContainsDurableButNotGeneratedFields(t *testing.T) {
	book := Normalize(Book{
		Title:     "Dune",
		ISBN:      "978-0-441-17271-9",
		CoverFile: "9780441172719.jpg",
		Cover:     "data/covers/9780441172719.jpg",
		Permalink: "978-0-441-17271-9",
	})
	var output bytes.Buffer
	if err := EncodeExport(&output, []Book{book}, "json"); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, `"coverFile": "9780441172719.jpg"`) {
		t.Fatalf("JSON export is missing the durable cover reference:\n%s", text)
	}
	for _, generated := range []string{`"cover":`, `"permalink":`, `"titleSlug":`} {
		if strings.Contains(text, generated) {
			t.Fatalf("JSON export contains generated field %s:\n%s", generated, text)
		}
	}
	decoded, err := DecodeImport(bytes.NewReader(output.Bytes()), "json")
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 || decoded[0].CoverFile != book.CoverFile {
		t.Fatalf("JSON round trip = %#v", decoded)
	}
}
