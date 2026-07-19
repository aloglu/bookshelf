package tui

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/aloglu/bookshelf/internal/library"
)

type accessiblePrompter struct {
	reader *bufio.Reader
	output io.Writer
}

var accessibleInput io.Reader = os.Stdin
var accessibleOutput io.Writer = os.Stdout

func AccessibleMode() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BOOKSHELF_ACCESSIBLE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func newAccessiblePrompter() *accessiblePrompter {
	return &accessiblePrompter{reader: bufio.NewReader(accessibleInput), output: accessibleOutput}
}

func (p *accessiblePrompter) line(prompt string) (string, error) {
	fmt.Fprint(p.output, prompt)
	value, err := p.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF && value == "" {
		return "", io.EOF
	}
	return strings.TrimSpace(value), nil
}

func (p *accessiblePrompter) text(label, current string, required bool) (string, error) {
	for {
		suffix := ": "
		if current != "" {
			suffix = fmt.Sprintf(" [%s] (Enter keeps it; - clears it): ", current)
		}
		value, err := p.line(label + suffix)
		if err != nil {
			return "", err
		}
		if value == "" && current != "" {
			return current, nil
		}
		if value == "-" {
			value = ""
		}
		if required && value == "" {
			fmt.Fprintln(p.output, label+" is required.")
			continue
		}
		return value, nil
	}
}

func (p *accessiblePrompter) yesNo(label string, current bool) (bool, error) {
	hint := "y/N"
	if current {
		hint = "Y/n"
	}
	for {
		value, err := p.line(fmt.Sprintf("%s [%s]: ", label, hint))
		if err != nil {
			return false, err
		}
		switch strings.ToLower(value) {
		case "":
			return current, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(p.output, "Enter yes or no.")
		}
	}
}

func (p *accessiblePrompter) year(current string) (string, error) {
	for {
		value, err := p.text("Published year", current, false)
		if err != nil {
			return "", err
		}
		if _, err := library.ParseYearInput(value); err != nil {
			fmt.Fprintln(p.output, "Published year must be exactly four digits.")
			continue
		}
		return value, nil
	}
}

func (p *accessiblePrompter) choice(label string, values []string, current string) (string, error) {
	for {
		fmt.Fprintln(p.output, label)
		defaultIndex := 0
		for index, value := range values {
			marker := ""
			if value == current {
				defaultIndex = index
				marker = " (current)"
			}
			fmt.Fprintf(p.output, "  %d. %s%s\n", index+1, value, marker)
		}
		input, err := p.line(fmt.Sprintf("Choice [%d]: ", defaultIndex+1))
		if err != nil {
			return "", err
		}
		if input == "" {
			return values[defaultIndex], nil
		}
		index, err := strconv.Atoi(input)
		if err == nil && index >= 1 && index <= len(values) {
			return values[index-1], nil
		}
		fmt.Fprintln(p.output, "Enter one of the listed numbers.")
	}
}

func (p *accessiblePrompter) pickBooks(books []library.Book, multi bool, title string) ([]string, bool, error) {
	if len(books) == 0 {
		fmt.Fprintln(p.output, "No books are available.")
		return nil, false, nil
	}
	fmt.Fprintln(p.output, title)
	for index, book := range books {
		description := book.Author
		if description != "" {
			description = " — " + description
		}
		fmt.Fprintf(p.output, "  %d. %s%s\n", index+1, book.Title, description)
	}
	prompt := "Book number (blank cancels): "
	if multi {
		prompt = "Book numbers separated by spaces or commas (blank cancels): "
	}
	for {
		value, err := p.line(prompt)
		if err != nil {
			return nil, false, err
		}
		if value == "" {
			return nil, false, nil
		}
		fields := strings.Fields(strings.ReplaceAll(value, ",", " "))
		if !multi && len(fields) != 1 {
			fmt.Fprintln(p.output, "Choose exactly one book.")
			continue
		}
		ids := make([]string, 0, len(fields))
		seen := make(map[string]bool)
		valid := true
		for _, field := range fields {
			index, parseErr := strconv.Atoi(field)
			if parseErr != nil || index < 1 || index > len(books) {
				valid = false
				break
			}
			id := books[index-1].ID
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		if valid && len(ids) > 0 {
			return ids, true, nil
		}
		fmt.Fprintln(p.output, "Enter valid book numbers from the list.")
	}
}

func (p *accessiblePrompter) bookForm(existing *library.Book) (BookFormResult, error) {
	current := library.Book{}
	if existing != nil {
		current = *existing
		fmt.Fprintln(p.output, "Edit a Book")
	} else {
		fmt.Fprintln(p.output, "Add a Book")
	}
	title, err := p.text("Title", current.Title, true)
	if err != nil {
		return BookFormResult{}, err
	}
	author, err := p.text("Author", current.Author, false)
	if err != nil {
		return BookFormResult{}, err
	}
	isbn, err := p.text("ISBN", current.ISBN, false)
	if err != nil {
		return BookFormResult{}, err
	}
	slug, err := p.text("URL slug", current.Slug, false)
	if err != nil {
		return BookFormResult{}, err
	}
	translator, err := p.text("Translator", current.Translator, false)
	if err != nil {
		return BookFormResult{}, err
	}
	publisher, err := p.text("Publisher", current.Publisher, false)
	if err != nil {
		return BookFormResult{}, err
	}
	binding, err := p.text("Binding", current.Binding, false)
	if err != nil {
		return BookFormResult{}, err
	}
	published, err := p.year(current.Year())
	if err != nil {
		return BookFormResult{}, err
	}
	currentVisibility := library.NormalizeWebsiteVisibility(current.WebsiteVisibility)
	visibilityLabel, err := p.choice(
		"Website visibility",
		[]string{"Visible", "Hidden"},
		map[library.WebsiteVisibility]string{
			library.WebsiteVisible: "Visible",
			library.WebsiteHidden:  "Hidden",
		}[currentVisibility],
	)
	if err != nil {
		return BookFormResult{}, err
	}
	visibility := library.WebsiteVisible
	if visibilityLabel == "Hidden" {
		visibility = library.WebsiteHidden
	}
	build := true
	if visibility == currentVisibility {
		build, err = p.yesNo("Update published website data after saving?", true)
		if err != nil {
			return BookFormResult{}, err
		}
	} else {
		fmt.Fprintln(p.output, "The website will be updated because visibility changed.")
	}
	book := library.FromInput(library.BookInput{
		Title: title, Author: author, ISBN: isbn, Slug: slug, Translator: translator,
		Publisher: publisher, Binding: binding, Published: published,
		WebsiteVisibility: string(visibility),
	})
	if existing != nil {
		book.ID = existing.ID
		book.CoverFile = existing.CoverFile
		book.Cover = existing.Cover
		book.SpineColor = existing.SpineColor
		book.SpineTextColor = existing.SpineTextColor
		book = library.Normalize(book)
	}
	return BookFormResult{Book: book, Build: build}, nil
}

func (p *accessiblePrompter) coverSource(books []library.Book) (CoverSourceResult, error) {
	values := []string{"automatic", "goodreads", "openlibrary", "google"}
	if len(books) == 1 {
		values = append(values, "url")
	}
	values = append(values, "manual", "cancel")
	value, err := p.choice("Choose a Cover Source", values, "automatic")
	if err != nil || value == "cancel" {
		return CoverSourceResult{}, err
	}
	source, err := library.ParseCoverSource(value)
	if err != nil {
		return CoverSourceResult{}, err
	}
	result := CoverSourceResult{Source: source, Confirmed: true}
	if source == library.CoverSourceURL {
		for {
			input, inputErr := p.line("Custom HTTP or HTTPS image URL: ")
			if inputErr != nil {
				return CoverSourceResult{}, inputErr
			}
			parsed, parseErr := url.ParseRequestURI(input)
			if parseErr == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
				result.URL = input
				break
			}
			fmt.Fprintln(p.output, "Enter a valid HTTP or HTTPS URL.")
		}
	}
	return result, nil
}

func runAccessibleDecision(request DecisionRequest) (string, bool, error) {
	p := newAccessiblePrompter()
	fmt.Fprintln(p.output, request.Title)
	if request.Description != "" {
		fmt.Fprintln(p.output, request.Description)
	}
	values := make([]string, len(request.Options))
	current := ""
	for index, option := range request.Options {
		values[index] = option.Label
		if index == request.Default {
			current = option.Label
		}
	}
	choice, err := p.choice("Choose an action", values, current)
	if err != nil {
		return "", false, err
	}
	for index, label := range values {
		if label == choice {
			return request.Options[index].ID, true, nil
		}
	}
	return "", false, nil
}

func runAccessibleSettingsForm(config library.Config) (library.Config, bool, error) {
	p := newAccessiblePrompter()
	fmt.Fprintln(p.output, "Settings")
	var err error
	if config.SiteTitle, err = p.text("Website title", config.SiteTitle, true); err != nil {
		return library.Config{}, false, err
	}
	if config.SiteSubtitle, err = p.text("Subtitle", config.SiteSubtitle, false); err != nil {
		return library.Config{}, false, err
	}
	if config.ShowStatistics, err = p.yesNo("Show library statistics?", config.ShowStatistics); err != nil {
		return library.Config{}, false, err
	}
	if config.ShowRandom, err = p.yesNo("Show random book button?", config.ShowRandom); err != nil {
		return library.Config{}, false, err
	}
	value, err := p.choice("Default desktop view", []string{"shelf", "stack", "coverflow"}, string(config.DefaultView))
	if err != nil {
		return library.Config{}, false, err
	}
	config.DefaultView = library.WebsiteView(value)
	value, err = p.choice("Shelf scroll speed", []string{"slow", "normal", "fast"}, string(config.ShelfScrollSpeed))
	if err != nil {
		return library.Config{}, false, err
	}
	config.ShelfScrollSpeed = library.ScrollSpeed(value)
	value, err = p.choice("Coverflow scroll speed", []string{"slow", "normal", "fast"}, string(config.CoverflowSpeed))
	if err != nil {
		return library.Config{}, false, err
	}
	config.CoverflowSpeed = library.ScrollSpeed(value)
	value, err = p.choice("Default sort", []string{"title", "author", "year"}, string(config.DefaultSort))
	if err != nil {
		return library.Config{}, false, err
	}
	config.DefaultSort = library.WebsiteSort(value)
	value, err = p.choice("Sort direction", []string{"ascending", "descending"}, string(config.DefaultSortOrder))
	if err != nil {
		return library.Config{}, false, err
	}
	config.DefaultSortOrder = library.SortDirection(value)
	value, err = p.choice("ISBN link sources", []string{"both", "wikipedia", "goodreads"}, string(config.ISBNLinkSources))
	if err != nil {
		return library.Config{}, false, err
	}
	config.ISBNLinkSources = library.ISBNLinkSources(value)
	value, err = p.choice("Default permalink", []string{"formatted-isbn", "compact-isbn", "title-slug"}, string(config.PermalinkStyle))
	if err != nil {
		return library.Config{}, false, err
	}
	config.PermalinkStyle = library.PermalinkStyle(value)
	if config.ShowFooter, err = p.yesNo("Show footer?", config.ShowFooter); err != nil {
		return library.Config{}, false, err
	}
	if config.FooterText, err = p.text("Footer text", config.FooterText, false); err != nil {
		return library.Config{}, false, err
	}
	save, err := p.yesNo("Save settings?", true)
	return config, save, err
}
