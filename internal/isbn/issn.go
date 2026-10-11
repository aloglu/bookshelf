package isbn

import (
	"errors"
	"strings"
)

// Periodicals, including the book-like issues some of them publish, carry an
// ISSN instead of an ISBN: eight characters, the last a check digit that may
// be X, shown as 1300-2880. Their barcodes are EAN-13 codes starting 977:
// 977, the ISSN's first seven digits, two variant digits, a check digit.

// ErrInvalidISSN means the digits don't make an ISSN.
var ErrInvalidISSN = errors.New("that isn’t a valid ISSN")

func issnCheck(seven string) byte {
	sum := 0
	for i, r := range seven {
		sum += int(r-'0') * (8 - i)
	}
	switch c := (11 - sum%11) % 11; c {
	case 10:
		return 'X'
	default:
		return byte('0' + c)
	}
}

func validISSN(d string) bool {
	if len(d) != 8 {
		return false
	}
	for _, r := range d[:7] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return d[7] == issnCheck(d[:7])
}

func validEAN(d string) bool {
	if len(d) != 13 {
		return false
	}
	sum := 0
	for i, r := range d {
		if r < '0' || r > '9' {
			return false
		}
		w := 1
		if i%2 == 1 {
			w = 3
		}
		sum += int(r-'0') * w
	}
	return sum%10 == 0
}

// NormalizeISSN turns an ISSN as typed ("ISSN 1300-2880", "13002880") or a
// periodical's 977 barcode into the bare eight characters.
func NormalizeISSN(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.TrimPrefix(strings.ToUpper(s), "ISSN"), ":")
	d := digits(s)
	switch {
	case d == "":
		return "", nil
	case len(d) == 8 && validISSN(d):
		return d, nil
	case len(d) == 13 && strings.HasPrefix(d, "977") && validEAN(d):
		seven := d[3:10]
		return seven + string(issnCheck(seven)), nil
	default:
		return "", ErrInvalidISSN
	}
}

// HyphenateISSN shows an ISSN as 1300-2880.
func HyphenateISSN(issn string) string {
	if len(issn) != 8 {
		return issn
	}
	return issn[:4] + "-" + issn[4:]
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= '0' && r <= '9') || r == 'X' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ID is a book's identifier: an ISBN, or a periodical's ISSN.
type ID struct {
	ISBN string // bare ISBN-13, or ""
	ISSN string // bare eight characters, or ""
}

// ErrInvalidID means the identifier is neither a valid ISBN nor a valid ISSN.
var ErrInvalidID = errors.New("that isn’t a valid ISBN or ISSN")

// Parse reads the one "ISBN or ISSN" field: an ISBN-13 or ISBN-10, an ISSN,
// or a periodical's 977 barcode. An ISSN is recognized by its length (eight
// characters) or by a 977 barcode; an explicit "ISSN" label also counts.
func Parse(raw string) (ID, error) {
	d := digits(label.ReplaceAllString(raw, ""))
	explicit := strings.Contains(strings.ToUpper(raw), "ISSN")
	switch {
	case d == "":
		return ID{}, nil
	case explicit || len(d) == 8 || (len(d) == 13 && strings.HasPrefix(d, "977")):
		issn, err := NormalizeISSN(raw)
		if err != nil {
			return ID{}, ErrInvalidID
		}
		return ID{ISSN: issn}, nil
	default:
		n, err := Normalize(raw)
		if err != nil {
			return ID{}, ErrInvalidID
		}
		return ID{ISBN: n}, nil
	}
}

// Display shows an identifier hyphenated, with "ISSN" before an ISSN.
func (id ID) Display() string {
	switch {
	case id.ISBN != "":
		return Hyphenate(id.ISBN)
	case id.ISSN != "":
		return "ISSN " + HyphenateISSN(id.ISSN)
	}
	return ""
}
