// Package isbn normalizes, checks and hyphenates ISBNs.
//
// Bookshelf stores an ISBN as its bare 13 digits and shows it hyphenated.
// Where the hyphens fall depends on how many digits each registration group
// and publisher were given, which comes from the International ISBN Agency's
// range table (RangeMessage.xml, embedded here; refresh it from
// https://www.isbn-international.org/range_file_generation now and then).
package isbn

import (
	_ "embed"
	"encoding/xml"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

//go:embed RangeMessage.xml
var rangeMessage []byte

// ErrInvalid means the digits don't make an ISBN.
var ErrInvalid = errors.New("that isn’t a valid ISBN")

var label = regexp.MustCompile(`(?i)^\s*isbn(?:[-\s]?1[03])?\s*:?\s*`)

// Normalize turns whatever was typed or pasted ("ISBN-10: 0-679-72342-0",
// "978 0679 723424", stray dots) into bare ISBN-13 digits, converting a valid
// ISBN-10. An empty input gives "" and no error.
func Normalize(raw string) (string, error) {
	s := label.ReplaceAllString(raw, "")
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= '0' && r <= '9') || r == 'X' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case d == "":
		return "", nil
	case len(d) == 10 && valid10(d):
		return To13(d), nil
	case len(d) == 13 && valid13(d):
		return d, nil
	default:
		return "", ErrInvalid
	}
}

func valid10(d string) bool {
	sum := 0
	for i, r := range d {
		var v int
		switch {
		case r == 'X' && i == 9:
			v = 10
		case r >= '0' && r <= '9':
			v = int(r - '0')
		default:
			return false
		}
		sum += v * (10 - i)
	}
	return sum%11 == 0
}

func valid13(d string) bool {
	if !strings.HasPrefix(d, "978") && !strings.HasPrefix(d, "979") {
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

// To13 converts a valid ISBN-10 to its ISBN-13.
func To13(isbn10 string) string {
	d := "978" + isbn10[:9]
	sum := 0
	for i, r := range d {
		w := 1
		if i%2 == 1 {
			w = 3
		}
		sum += int(r-'0') * w
	}
	return d + strconv.Itoa((10-sum%10)%10)
}

type rule struct {
	lo, hi string // 7-digit bounds
	length int
}

var (
	once   sync.Once
	prefix map[string][]rule // "978": group rules
	groups map[string][]rule // "978-0": publisher rules
)

func loadRanges() {
	var msg struct {
		Prefixes []struct {
			Prefix string `xml:"Prefix"`
			Rules  []struct {
				Range  string `xml:"Range"`
				Length int    `xml:"Length"`
			} `xml:"Rules>Rule"`
		} `xml:"EAN.UCCPrefixes>EAN.UCC"`
		Groups []struct {
			Prefix string `xml:"Prefix"`
			Rules  []struct {
				Range  string `xml:"Range"`
				Length int    `xml:"Length"`
			} `xml:"Rules>Rule"`
		} `xml:"RegistrationGroups>Group"`
	}
	prefix, groups = map[string][]rule{}, map[string][]rule{}
	if err := xml.Unmarshal(rangeMessage, &msg); err != nil {
		panic("isbn: bad embedded RangeMessage.xml: " + err.Error())
	}
	toRule := func(rng string, length int) rule {
		lo, hi, _ := strings.Cut(rng, "-")
		return rule{lo: lo, hi: hi, length: length}
	}
	for _, p := range msg.Prefixes {
		for _, r := range p.Rules {
			prefix[p.Prefix] = append(prefix[p.Prefix], toRule(r.Range, r.Length))
		}
	}
	for _, g := range msg.Groups {
		for _, r := range g.Rules {
			groups[g.Prefix] = append(groups[g.Prefix], toRule(r.Range, r.Length))
		}
	}
}

func lengthFor(rules []rule, seven string) int {
	for _, r := range rules {
		if seven >= r.lo && seven <= r.hi {
			return r.length
		}
	}
	return 0
}

// Hyphenate shows bare ISBN-13 digits as prefix-group-publisher-title-check
// (978-0-679-72342-4). Digits it can't place (an unassigned range) are
// returned as they are.
func Hyphenate(isbn13 string) string {
	if len(isbn13) != 13 {
		return isbn13
	}
	once.Do(loadRanges)
	ean, rest := isbn13[:3], isbn13[3:12]
	gl := lengthFor(prefix[ean], rest[:7])
	if gl == 0 {
		return isbn13
	}
	group := rest[:gl]
	pl := lengthFor(groups[ean+"-"+group], (rest[gl:] + "0000000")[:7])
	if pl == 0 {
		return isbn13
	}
	return strings.Join([]string{ean, group, rest[gl : gl+pl], rest[gl+pl:], isbn13[12:]}, "-")
}
