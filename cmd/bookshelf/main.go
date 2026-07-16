package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aloglu/bookshelf/internal/library"
	"github.com/aloglu/bookshelf/internal/tui"
)

var version = "dev"

const defaultInstallerURL = "https://raw.githubusercontent.com/aloglu/bookshelf/main/install.sh"

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bookshelf:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	command := ""
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}
	switch command {
	case "help", "--help", "-h":
		usage(os.Stdout)
		return nil
	case "version", "--version", "-v":
		fmt.Printf("bookshelf %s\n", version)
		return nil
	case "upgrade":
		return upgrade(ctx)
	case "uninstall":
		return uninstall(args)
	}

	root, err := library.ResolveRoot()
	if err != nil {
		return err
	}
	paths := library.NewPaths(root)
	if err := library.Ensure(paths); err != nil {
		return err
	}

	switch command {
	case "":
		if !tui.IsTerminal() {
			usage(os.Stderr)
			return fmt.Errorf("interactive mode requires a terminal")
		}
		return runManager(ctx, paths)
	case "list", "ls":
		return listCommand(ctx, paths, args)
	case "build":
		return buildCommand(ctx, paths, args)
	case "validate":
		return validateCommand(paths)
	case "add":
		return addCommand(ctx, paths, args)
	case "update", "edit":
		return updateCommand(ctx, paths, args)
	case "remove", "delete", "rm":
		return removeCommand(ctx, paths, args)
	case "covers", "cover":
		return coversCommand(ctx, paths, args)
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", command)
	}
}

func usage(output io.Writer) {
	fmt.Fprintln(output, `Bookshelf — manage and publish your book collection

Usage:
  bookshelf                         Open the interactive manager
  bookshelf list [--plain|--json]   Browse or print the library
  bookshelf add [fields]            Add a book
  bookshelf update --id-or-isbn ID  Edit an existing book
  bookshelf remove [IDs...]         Remove one or more books
  bookshelf build [options]         Generate published data and covers
  bookshelf covers [IDs...]         Apply matching manual covers
  bookshelf validate                Validate source and published data
  bookshelf upgrade                 Install the latest Bookshelf release
  bookshelf uninstall               Remove Bookshelf
  bookshelf version                 Print the installed version

Run a command without fields in a terminal to use its interactive workflow.`)
}

func buildCommand(ctx context.Context, paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	fetch := flags.Bool("fetch-covers", false, "fetch missing covers")
	recompute := flags.Bool("recompute-colors", false, "recompute spine colors")
	if err := flags.Parse(args); err != nil {
		return err
	}
	stats, err := library.Build(ctx, paths, library.BuildOptions{
		FetchCovers:     *fetch,
		RecomputeColors: *recompute,
		OnFetch: func(book library.Book, state string) {
			fmt.Printf("%s: %s\n", state, book.Title)
		},
	})
	if err != nil {
		return err
	}
	printStats(stats)
	return nil
}

func validateCommand(paths library.Paths) error {
	books, err := library.Load(paths)
	if err != nil {
		return err
	}
	problems := library.Validate(books)
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "- %v\n", problem)
		}
		return fmt.Errorf("library validation failed with %d problem(s)", len(problems))
	}
	generated, err := library.LoadGenerated(paths)
	if err != nil {
		return fmt.Errorf("source library is valid, but published data is missing or invalid: %w; run `bookshelf build`", err)
	}
	if !library.GeneratedMatches(books, generated) {
		return fmt.Errorf("source library is valid, but published data is stale; run `bookshelf build`")
	}
	fmt.Printf("Library is valid and published data is current. Books: %d\n", len(books))
	return nil
}

func addCommand(ctx context.Context, paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	input, fetch, noBuild := bookFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	var book library.Book
	if strings.TrimSpace(input.Title) == "" && tui.IsTerminal() {
		result, err := tui.RunBookForm(nil)
		if err != nil {
			return err
		}
		book = result.Book
		*fetch = result.FetchCover
		*noBuild = !result.Build
	} else {
		book = library.FromInput(*input)
	}
	added, stats, err := library.Add(ctx, paths, book, library.ChangeOptions{
		FetchCover: *fetch,
		Build:      !*noBuild,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Added %q.\n", added.Title)
	if !*noBuild {
		printStats(stats)
	}
	return nil
}

func updateCommand(ctx context.Context, paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	id := flags.String("id-or-isbn", "", "book id or ISBN")
	patch, fetch, noBuild := updateFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		if tui.IsTerminal() {
			return runManager(ctx, paths)
		}
		return fmt.Errorf("--id-or-isbn is required")
	}
	updated, stats, err := library.Update(ctx, paths, *id, *patch, library.ChangeOptions{
		FetchCover: *fetch,
		Build:      !*noBuild,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Updated %q.\n", updated.Title)
	if !*noBuild {
		printStats(stats)
	}
	return nil
}

func removeCommand(ctx context.Context, paths library.Paths, args []string) error {
	ids, yes, removeCovers, err := parseRemoveArgs(args)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		if tui.IsTerminal() {
			return runManager(ctx, paths)
		}
		return fmt.Errorf("provide at least one id or ISBN")
	}
	books, err := booksForIDs(paths, ids)
	if err != nil {
		return err
	}
	if !yes && tui.IsTerminal() {
		confirmed, covers, err := tui.ConfirmRemoval(books)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
		removeCovers = covers
	} else if !yes {
		return fmt.Errorf("--yes is required for non-interactive removal")
	}
	removed, err := library.Remove(paths, ids, removeCovers)
	if err != nil {
		return err
	}
	fmt.Printf("Removed %d book(s).\n", len(removed))
	return nil
}

func coversCommand(ctx context.Context, paths library.Paths, args []string) error {
	ids, all, recompute, err := parseCoversArgs(args)
	if err != nil {
		return err
	}
	if all {
		ids = nil
	}
	if len(ids) == 0 && !all && tui.IsTerminal() {
		return runManager(ctx, paths)
	}
	stats, err := library.ApplyManualCovers(ctx, paths, ids, recompute)
	if err != nil {
		return err
	}
	printStats(stats)
	return nil
}

func listCommand(ctx context.Context, paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	plain := flags.Bool("plain", false, "print a non-interactive table")
	jsonOutput := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	books, err := library.Load(paths)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(books)
	}
	if tui.IsTerminal() && !*plain {
		return runManager(ctx, paths)
	}
	statuses := library.PublicationStatuses(paths, books)
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "TITLE\tAUTHOR\tYEAR\tISBN\tCOVER\tSTATUS")
	for _, book := range books {
		cover := "no"
		if book.Cover != "" {
			cover = "yes"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			book.Title, book.Author, book.Year(), book.ISBN, cover, statuses[book.ID])
	}
	return writer.Flush()
}

func runManager(ctx context.Context, paths library.Paths) error {
	for {
		books, err := library.Load(paths)
		if err != nil {
			return err
		}
		result, err := tui.RunBrowser(books, library.PublicationStatuses(paths, books))
		if err != nil {
			return err
		}
		switch result.Action {
		case tui.ActionQuit:
			return nil
		case tui.ActionAdd:
			form, err := tui.RunBookForm(nil)
			if err != nil {
				return err
			}
			if _, _, err := library.Add(ctx, paths, form.Book, library.ChangeOptions{FetchCover: form.FetchCover, Build: form.Build}); err != nil {
				return err
			}
		case tui.ActionEdit:
			selected, err := booksForIDs(paths, result.IDs)
			if err != nil {
				return err
			}
			form, err := tui.RunBookForm(&selected[0])
			if err != nil {
				return err
			}
			if _, _, err := library.Replace(ctx, paths, selected[0].ID, form.Book, library.ChangeOptions{FetchCover: form.FetchCover, Build: form.Build}); err != nil {
				return err
			}
		case tui.ActionRemove:
			selected, err := booksForIDs(paths, result.IDs)
			if err != nil {
				return err
			}
			confirmed, removeCovers, err := tui.ConfirmRemoval(selected)
			if err != nil {
				return err
			}
			if confirmed {
				if _, err := library.Remove(paths, result.IDs, removeCovers); err != nil {
					return err
				}
			}
		case tui.ActionBuild:
			options, err := tui.RunBuildForm()
			if err != nil {
				return err
			}
			if options.Confirmed {
				if _, err := library.Build(ctx, paths, library.BuildOptions{
					FetchCovers:     options.FetchCovers,
					RecomputeColors: options.RecomputeColors,
				}); err != nil {
					return err
				}
			}
		case tui.ActionValidate:
			if err := validateCommand(paths); err != nil {
				return err
			}
		case tui.ActionCovers:
			if _, err := library.ApplyManualCovers(ctx, paths, result.IDs, true); err != nil {
				return err
			}
		}
	}
}

func bookFlags(flags *flag.FlagSet) (*library.BookInput, *bool, *bool) {
	input := &library.BookInput{}
	flags.StringVar(&input.Title, "title", "", "title")
	flags.StringVar(&input.Author, "author", "", "author")
	flags.StringVar(&input.ISBN, "isbn", "", "ISBN")
	flags.StringVar(&input.Translator, "translator", "", "translator")
	flags.StringVar(&input.Publisher, "publisher", "", "publisher")
	flags.StringVar(&input.Binding, "binding", "", "binding")
	flags.StringVar(&input.Published, "published", "", "published year")
	fetch := flags.Bool("fetch-covers", false, "fetch a missing cover")
	noBuild := flags.Bool("no-build", false, "save without refreshing published data")
	return input, fetch, noBuild
}

func updateFlags(flags *flag.FlagSet) (*library.BookPatch, *bool, *bool) {
	patch := &library.BookPatch{}
	optionalStringFlag(flags, "title", "title; pass an empty value only to receive a validation error", &patch.Title)
	optionalStringFlag(flags, "author", "author; pass an empty value to clear", &patch.Author)
	optionalStringFlag(flags, "isbn", "ISBN; pass an empty value to clear", &patch.ISBN)
	optionalStringFlag(flags, "translator", "translator; pass an empty value to clear", &patch.Translator)
	optionalStringFlag(flags, "publisher", "publisher; pass an empty value to clear", &patch.Publisher)
	optionalStringFlag(flags, "binding", "binding; pass an empty value to clear", &patch.Binding)
	optionalStringFlag(flags, "published", "published year; pass an empty value to clear", &patch.Published)
	fetch := flags.Bool("fetch-covers", false, "fetch a missing cover")
	noBuild := flags.Bool("no-build", false, "save without refreshing published data")
	return patch, fetch, noBuild
}

func optionalStringFlag(flags *flag.FlagSet, name, usage string, destination **string) {
	flags.Func(name, usage, func(value string) error {
		copy := value
		*destination = &copy
		return nil
	})
}

func booksForIDs(paths library.Paths, ids []string) ([]library.Book, error) {
	books, err := library.Load(paths)
	if err != nil {
		return nil, err
	}
	selected := make([]library.Book, 0, len(ids))
	seen := make(map[string]bool)
	for _, id := range ids {
		index := library.FindIndex(books, id)
		if index < 0 {
			return nil, fmt.Errorf("no book found for %q", id)
		}
		if !seen[books[index].ID] {
			selected = append(selected, books[index])
			seen[books[index].ID] = true
		}
	}
	return selected, nil
}

func parseRemoveArgs(args []string) (ids []string, yes, removeCovers bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--yes":
			yes = true
		case arg == "--remove-covers":
			removeCovers = true
		case arg == "--id-or-isbn":
			if i+1 >= len(args) {
				return nil, false, false, fmt.Errorf("--id-or-isbn requires a value")
			}
			i++
			ids = append(ids, args[i])
		case strings.HasPrefix(arg, "--id-or-isbn="):
			ids = append(ids, strings.TrimPrefix(arg, "--id-or-isbn="))
		case strings.HasPrefix(arg, "-"):
			return nil, false, false, fmt.Errorf("unknown remove option %q", arg)
		default:
			ids = append(ids, arg)
		}
	}
	return ids, yes, removeCovers, nil
}

func parseCoversArgs(args []string) (ids []string, all, recompute bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--all":
			all = true
		case arg == "--recompute-colors":
			recompute = true
		case arg == "--id-or-isbn":
			if i+1 >= len(args) {
				return nil, false, false, fmt.Errorf("--id-or-isbn requires a value")
			}
			i++
			ids = append(ids, args[i])
		case strings.HasPrefix(arg, "--id-or-isbn="):
			ids = append(ids, strings.TrimPrefix(arg, "--id-or-isbn="))
		case strings.HasPrefix(arg, "-"):
			return nil, false, false, fmt.Errorf("unknown covers option %q", arg)
		default:
			ids = append(ids, arg)
		}
	}
	return ids, all, recompute, nil
}

func printStats(stats library.BuildStats) {
	fmt.Printf("Done. Books: %d. Processed: %d. Manual covers: %d. Downloaded: %d. Colors: %d. Missing covers: %d.\n",
		stats.Books, stats.Processed, stats.Manuals, stats.Downloads, stats.Colored, stats.Missing)
}

func upgrade(ctx context.Context) error {
	url := strings.TrimSpace(os.Getenv("BOOKSHELF_INSTALLER_URL"))
	if url == "" {
		url = defaultInstallerURL
	}
	fmt.Printf("Upgrading Bookshelf %s for %s/%s...\n", version, runtime.GOOS, runtime.GOARCH)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download installer: %s", response.Status)
	}
	temp, err := os.CreateTemp("", "bookshelf-install-*.sh")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err := io.Copy(temp, io.LimitReader(response.Body, 2<<20)); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "sh", name, "--upgrade")
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func uninstall(args []string) error {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	force := flags.Bool("force", false, "allow uninstall outside the installed binary")
	if err := flags.Parse(args); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	installDir := os.Getenv("BOOKSHELF_INSTALL_DIR")
	if installDir == "" {
		installDir = filepath.Join(home, ".local", "share", "bookshelf")
	}
	binPath := os.Getenv("BOOKSHELF_BIN_PATH")
	if binPath == "" {
		binPath = filepath.Join(home, ".local", "bin", "bookshelf")
	}
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	expected, _ := filepath.EvalSymlinks(binPath)
	if executable != expected && !*force {
		return fmt.Errorf("refusing to uninstall from development binary %s; run the installed command or pass --force", executable)
	}
	if err := os.Remove(binPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.RemoveAll(installDir); err != nil {
		return err
	}
	fmt.Printf("Removed %s and %s\n", binPath, installDir)
	return nil
}
