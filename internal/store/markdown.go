package store

import (
	"regexp"
	"strings"
)

// The library's name and its footer may hold inline Markdown: links, bold,
// italic and code. The browser draws it; where only text will do (the page
// title, a phone's home screen), PlainText is used.
var (
	mdLink     = regexp.MustCompile(`\[([^\]]+)\]\([^)\s]+\)`)
	mdAutolink = regexp.MustCompile(`<((?:https?://|mailto:)[^>\s]+)>`)
	mdCode     = regexp.MustCompile("`([^`]+)`")
	// Emphasis neither opens nor closes beside a space, so "2 * 3 * 4" stays as typed.
	mdStrong = regexp.MustCompile(`\*\*([^\s*](?:.*?[^\s*])?)\*\*|__([^\s_](?:.*?[^\s_])?)__`)
	mdEm     = regexp.MustCompile(`\*([^\s*](?:.*?[^\s*])?)\*|(^|[^\p{L}\p{N}_])_([^\s_](?:.*?[^\s_])?)_([^\p{L}\p{N}_]|$)`)
	spaces   = regexp.MustCompile(`\s+`)
)

// PlainText is inline Markdown as plain words: links become their text and
// emphasis marks go.
func PlainText(md string) string {
	s := mdCode.ReplaceAllString(md, "$1")
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdAutolink.ReplaceAllString(s, "$1")
	s = mdStrong.ReplaceAllString(s, "$1$2")
	s = mdEm.ReplaceAllString(s, "$1$2$3$4")
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}
