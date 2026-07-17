package library

import (
	"strings"
	"unicode"
)

// NormalizeTypography converts straight English quotation marks to contextual
// typographic marks. Existing typographic punctuation is left unchanged.
func NormalizeTypography(value string) string {
	runes := []rune(value)
	var output strings.Builder
	output.Grow(len(value))
	for index, current := range runes {
		if current != '\'' && current != '"' {
			output.WriteRune(current)
			continue
		}
		previous := adjacentRune(runes, index-1)
		next := adjacentRune(runes, index+1)
		nextVisible := nextVisibleRune(runes, index)
		if current == '\'' {
			if isWordRune(previous) && isWordRune(next) {
				output.WriteRune('’')
			} else if isOpeningQuoteContext(previous) && nextVisible != 0 {
				output.WriteRune('‘')
			} else {
				output.WriteRune('’')
			}
			continue
		}
		if isOpeningQuoteContext(previous) && nextVisible != 0 {
			output.WriteRune('“')
		} else {
			output.WriteRune('”')
		}
	}
	return output.String()
}

// NormalizeFooterMarkdown normalizes prose and link labels without changing
// inline code or link destinations.
func NormalizeFooterMarkdown(value string) string {
	var output strings.Builder
	for offset := 0; offset < len(value); {
		switch value[offset] {
		case '`':
			end := strings.IndexByte(value[offset+1:], '`')
			if end < 0 {
				output.WriteString(value[offset:])
				return output.String()
			}
			end += offset + 2
			output.WriteString(value[offset:end])
			offset = end
		case '[':
			labelEnd := strings.Index(value[offset+1:], "](")
			if labelEnd < 0 {
				next := nextMarkdownBoundary(value, offset+1)
				output.WriteString(NormalizeTypography(value[offset:next]))
				offset = next
				continue
			}
			labelEnd += offset + 1
			destinationStart := labelEnd + 2
			destinationEnd := strings.IndexByte(value[destinationStart:], ')')
			if destinationEnd < 0 {
				next := nextMarkdownBoundary(value, offset+1)
				output.WriteString(NormalizeTypography(value[offset:next]))
				offset = next
				continue
			}
			destinationEnd += destinationStart
			output.WriteByte('[')
			output.WriteString(NormalizeTypography(value[offset+1 : labelEnd]))
			output.WriteString("](")
			output.WriteString(value[destinationStart:destinationEnd])
			output.WriteByte(')')
			offset = destinationEnd + 1
		default:
			next := nextMarkdownBoundary(value, offset)
			output.WriteString(NormalizeTypography(value[offset:next]))
			offset = next
		}
	}
	return output.String()
}

func nextMarkdownBoundary(value string, start int) int {
	next := len(value)
	if index := strings.IndexByte(value[start:], '`'); index >= 0 && start+index < next {
		next = start + index
	}
	if index := strings.IndexByte(value[start:], '['); index >= 0 && start+index < next {
		next = start + index
	}
	if next == start {
		return start + 1
	}
	return next
}

func adjacentRune(runes []rune, index int) rune {
	if index < 0 || index >= len(runes) {
		return 0
	}
	return runes[index]
}

func nextVisibleRune(runes []rune, index int) rune {
	for position := index + 1; position < len(runes); position++ {
		if !unicode.IsSpace(runes[position]) {
			return runes[position]
		}
	}
	return 0
}

func isWordRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsNumber(value)
}

func isOpeningQuoteContext(value rune) bool {
	if value == 0 || unicode.IsSpace(value) {
		return true
	}
	return strings.ContainsRune("([{<—–-", value)
}
