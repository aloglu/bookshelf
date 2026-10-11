package store

import (
	"fmt"
	"strconv"
	"time"
)

// Bindings a book can have. Mass market counts as paperback.
const (
	Paperback = "paperback"
	Hardcover = "hardcover"
	Cloth     = "cloth"
	Leather   = "leather"
)

// Book is one book on the shelf.
type Book struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Author             string    `json:"author"`
	Translator         string    `json:"translator"`
	Publisher          string    `json:"publisher"`
	Published          *int      `json:"published"`
	Pages              *int      `json:"pages"`
	Binding            string    `json:"binding"`
	ISBN               string    `json:"isbn"`  // bare ISBN-13 digits, or ""
	ISSN               string    `json:"issn"`  // a periodical's ISSN, bare eight characters, or ""
	Issue              string    `json:"issue"` // the periodical's issue, as printed ("70–71"), or ""
	Slug               string    `json:"slug"`
	Hidden             bool      `json:"hidden"`
	Cover              string    `json:"cover"` // file name in the covers folder, or "" for a plain cloth cover
	SpineColor         string    `json:"spineColor"`
	SpineTextColor     string    `json:"spineTextColor"`
	SpineColorOverride string    `json:"spineColorOverride"`
	AddedAt            time.Time `json:"addedAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// Precision says how much of a reading's date is known.
type Precision string

const (
	Instant Precision = "instant" // recorded by Bookshelf at the moment
	Day     Precision = "day"     // a date the owner gave
	Year    Precision = "year"    // only a year
	Unknown Precision = "unknown" // "I don't remember"
)

// When is a point in time known to some precision.
type When struct {
	Time      time.Time `json:"time"`
	Precision Precision `json:"precision"`
}

// Known reports whether there is any date at all.
func (w When) Known() bool { return w.Precision != Unknown && w.Precision != "" }

func (w When) encode() string {
	switch w.Precision {
	case Instant:
		return stamp(w.Time)
	case Day:
		return w.Time.Format("2006-01-02")
	case Year:
		return strconv.Itoa(w.Time.Year())
	default:
		return ""
	}
}

func decodeWhen(value string, p Precision) (When, error) {
	w := When{Precision: p}
	var err error
	switch p {
	case Instant:
		w.Time, err = time.Parse(time.RFC3339Nano, value)
	case Day:
		w.Time, err = time.Parse("2006-01-02", value)
	case Year:
		var y int
		y, err = strconv.Atoi(value)
		w.Time = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
	case Unknown:
	default:
		err = fmt.Errorf("unknown date precision %q", p)
	}
	return w, err
}

// Outcomes of a reading.
const (
	OutcomeReading  = "reading"
	OutcomeFinished = "finished"
	OutcomeSetAside = "set_aside"
)

// Reading is one time a book was read, tracked or marked as read.
type Reading struct {
	ID       string     `json:"id"`
	BookID   string     `json:"bookId"`
	Started  When       `json:"started"`
	Ended    When       `json:"ended"`
	Outcome  string     `json:"outcome"`
	Progress []Progress `json:"progress"` // oldest first; empty for a reading marked as read
	Note     *Note      `json:"note"`
}

// Page is the latest page recorded, or 0.
func (r Reading) Page() int {
	if n := len(r.Progress); n > 0 {
		return r.Progress[n-1].Page
	}
	return 0
}

// Progress is one page update.
type Progress struct {
	Page int       `json:"page"`
	At   time.Time `json:"at"`
}

// Note is the owner's thoughts on a finished reading.
type Note struct {
	Text      string    `json:"text"`
	Public    bool      `json:"public"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Session is a signed-in device.
type Session struct {
	TokenHash string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	LastSeen  time.Time `json:"lastSeen"`
	UserAgent string    `json:"userAgent"`
}
