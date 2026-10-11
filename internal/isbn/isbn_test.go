package isbn

import (
	"errors"
	"testing"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"ISBN-10: 0-679-72342-0":     "9780679723424",
		"isbn 978 0679 723424":       "9780679723424",
		"0.679.72342.0":              "9780679723424",
		"  9780679723424 ":           "9780679723424",
		"ISBN-13: 978-0-679-72342-4": "9780679723424",
		"080442957X":                 "9780804429573",
		"":                           "",
	} {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"123", "9780679723425", "0679723421", "1234567890123"} {
		if _, err := Normalize(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("Normalize(%q) accepted", in)
		}
	}
}

func TestHyphenate(t *testing.T) {
	for in, want := range map[string]string{
		"9780679723424": "978-0-679-72342-4", // English, 3-digit publisher
		"9780156453806": "978-0-15-645380-6",
		"9780805209990": "978-0-8052-0999-0",
		"9781590171998": "978-1-59017-199-8",
		"9781400033416": "978-1-4000-3341-6",
		"9783161484100": "978-3-16-148410-0", // German
		"9782070360024": "978-2-07-036002-4", // French
		"9786052957066": "978-605-295-706-6", // Turkey: a 3-digit group
		"9791032305683": "979-10-323-0568-3", // France, 979 prefix
	} {
		if got := Hyphenate(in); got != want {
			t.Errorf("Hyphenate(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestISSN(t *testing.T) {
	for in, want := range map[string]string{
		"1300-2880":      "13002880",
		"ISSN 1300-2880": "13002880",
		"977-2146023007": "2146023X", // a periodical's barcode
		"9772146023007":  "2146023X",
		"0378-5955":      "03785955",
	} {
		got, err := NormalizeISSN(in)
		if err != nil || got != want {
			t.Errorf("NormalizeISSN(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"1300-2881", "9772146023008", "123"} {
		if _, err := NormalizeISSN(in); err == nil {
			t.Errorf("NormalizeISSN(%q) accepted", in)
		}
	}
	if HyphenateISSN("2146023X") != "2146-023X" {
		t.Error("hyphenation")
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]ID{
		"978-0-679-72342-4": {ISBN: "9780679723424"},
		"0679723420":        {ISBN: "9780679723424"},
		"1300-2880":         {ISSN: "13002880"},
		"ISSN 0378-5955":    {ISSN: "03785955"},
		"977-2146023007":    {ISSN: "2146023X"},
		"":                  {},
	} {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"978-024198081", "978-9750807049", "1300-2881"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) accepted", in)
		}
	}
	if (ID{ISSN: "13002880"}).Display() != "ISSN 1300-2880" || (ID{ISBN: "9780679723424"}).Display() != "978-0-679-72342-4" {
		t.Error("display")
	}
}
