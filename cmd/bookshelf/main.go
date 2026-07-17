package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/aloglu/bookshelf/internal/library"
	"github.com/aloglu/bookshelf/internal/tui"
)

var version = "dev"
var httpClient = &http.Client{Timeout: 30 * time.Second}

const defaultInstallerURL = "https://raw.githubusercontent.com/aloglu/bookshelf/main/install.sh"
const defaultLatestReleaseURL = "https://api.github.com/repos/aloglu/bookshelf/releases/latest"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) || errors.Is(err, tui.ErrInterrupted) {
			return
		}
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
	if command == "" {
		usage(os.Stdout)
		return nil
	}
	if command != "help" && command != "--help" && command != "-h" && wantsHelp(args) {
		if !commandUsage(os.Stdout, command) {
			return fmt.Errorf("unknown command %q", command)
		}
		return nil
	}
	switch command {
	case "help", "--help", "-h":
		if len(args) > 0 {
			if !commandUsage(os.Stdout, args[0]) {
				return fmt.Errorf("unknown command %q", args[0])
			}
			return nil
		}
		usage(os.Stdout)
		return nil
	case "version", "--version", "-v":
		fmt.Printf("bookshelf %s\n", version)
		return nil
	case "upgrade":
		return upgradeCommand(ctx, args)
	case "uninstall":
		return uninstallCommand(args)
	case "_init":
		return initializeCommand()
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
	case "list", "ls":
		return listCommand(ctx, paths, args)
	case "build":
		return buildCommand(ctx, paths, args)
	case "preview", "serve":
		return previewCommand(ctx, paths, args)
	case "validate":
		return validateCommand(paths)
	case "add":
		return addCommand(ctx, paths, args)
	case "import":
		return importCommand(ctx, paths, args)
	case "export":
		return exportCommand(paths, args)
	case "edit":
		return editCommand(ctx, paths, args)
	case "remove", "delete", "rm":
		return removeCommand(ctx, paths, args)
	case "covers", "cover":
		return coversCommand(ctx, paths, args)
	case "settings", "config":
		return settingsCommand(paths, args)
	case "_sync-data":
		return syncDataCommand(paths)
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", command)
	}
}

func syncDataCommand(paths library.Paths) error {
	books, err := library.Load(paths)
	if err != nil {
		return err
	}
	if problems := library.Validate(books); len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "- %v\n", problem)
		}
		return fmt.Errorf("library synchronization failed with %d problem(s)", len(problems))
	}
	return library.SaveGenerated(paths, books)
}

func initializeCommand() error {
	root := strings.TrimSpace(os.Getenv("BOOKSHELF_INSTALL_DIR"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		root = filepath.Join(home, ".local", "share", "bookshelf")
	}
	paths := library.NewPaths(root)
	if err := library.Initialize(paths); err != nil {
		return err
	}
	return syncDataCommand(paths)
}

func usage(output io.Writer) {
	fmt.Fprintln(output, `Bookshelf — manage and publish your book collection

Usage:
  bookshelf                         Show this help
  bookshelf list [--plain|--json]   Browse or print the library
  bookshelf add [fields]            Add a book
  bookshelf import FILE [options]   Import books from JSON or CSV
  bookshelf export FILE [options]   Export books as JSON or Excel-compatible CSV
  bookshelf edit --id-or-isbn ID    Edit an existing book
  bookshelf remove [IDs...]         Remove one or more books
  bookshelf build [options]         Generate published data and covers
  bookshelf preview [options]       Preview the generated website locally
  bookshelf covers [IDs...]         Fetch or apply book covers
  bookshelf validate                Validate source and published data
  bookshelf settings [options]      Configure the published website
  bookshelf upgrade                 Install the latest Bookshelf release
  bookshelf uninstall               Remove Bookshelf
  bookshelf version                 Print the installed version

Run a command without fields in a terminal to use its interactive workflow.`)
}

func wantsHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")
}

func commandUsage(output io.Writer, command string) bool {
	command = strings.TrimSpace(command)
	switch command {
	case "add":
		fmt.Fprintln(output, `Usage:
  bookshelf add                         Open the add-book form
  bookshelf add --title TITLE [fields]  Add one book non-interactively
  bookshelf add --from FILE [options]   Import books from JSON or CSV

Single-book fields:
  --title, --author, --isbn, --slug, --translator, --publisher, --binding, --published
  --no-build        Save without refreshing published data

Batch options:
  --from FILE          Read JSON or CSV; use - for standard input
  --format json|csv    Override format detection (required with standard input)
  --skip-duplicates    Skip existing or repeated IDs/ISBNs
  --no-build           Import without refreshing published data
  --dry-run            Parse and validate without saving

JSON may be an array of book objects or {"books":[...]}. CSV requires a title
column and supports id, author, isbn, slug, translator, publisher, binding, and published.`)
	case "import":
		fmt.Fprintln(output, `Usage:
  bookshelf import FILE [options]
  bookshelf import - --format json|csv [options]

Options:
  --format json|csv    Override format detection (required with standard input)
  --skip-duplicates    Skip existing or repeated IDs/ISBNs
  --no-build           Import without refreshing published data
  --dry-run            Parse and validate without saving`)
	case "export":
		fmt.Fprintln(output, `Usage:
  bookshelf export FILE [--format json|csv] [--force]
  bookshelf export - --format json|csv

The format is inferred from a .json or .csv filename. CSV output is UTF-8,
uses Excel-friendly column headings and line endings, and preserves non-ASCII
book metadata. Existing files are not replaced unless --force is supplied.`)
	case "list", "ls":
		fmt.Fprintln(output, "Usage:\n  bookshelf list [--plain|--json]\n\nWithout an output flag, opens the paginated interactive library in a terminal.")
	case "build":
		fmt.Fprintln(output, "Usage:\n  bookshelf build [--recompute-colors]")
	case "preview", "serve":
		fmt.Fprintln(output, "Usage:\n  bookshelf preview [--port PORT] [--no-open]\n\nServes the generated website on localhost and opens it in the default browser.\nPress Ctrl+C to stop the preview server.")
	case "validate":
		fmt.Fprintln(output, "Usage:\n  bookshelf validate")
	case "settings", "config":
		fmt.Fprintln(output, `Usage:
  bookshelf settings
  bookshelf settings [options] [PERMALINK_STYLE]

Options:
  --permalink-style STYLE  formatted-isbn, compact-isbn, or title-slug
  --statistics VALUE      show or hide public library statistics
  --default-view VIEW     shelf, stack, or coverflow on desktop
  --default-sort SORT     title, author, or year
  --sort-direction ORDER  ascending or descending
  --site-title TEXT       public website heading
  --site-subtitle TEXT    public website subtitle
  --hide-subtitle         hide the public website subtitle
  --random VALUE          show or hide the random-book button
  --isbn-links SOURCES    both, wikipedia, or goodreads
  --footer VALUE          show or hide the footer
  --footer-text MARKDOWN  replace the built-in footer attribution

With no options in a terminal, opens the Settings screen.
Mobile visitors always use the stacks view.`)
	case "edit":
		fmt.Fprintln(output, "Usage:\n  bookshelf edit [--id-or-isbn ID] [fields] [--no-build]\n\nWithout an ID in a terminal, opens the book picker.\nThe editable fields are the same as for `bookshelf add`.")
	case "remove", "delete", "rm":
		fmt.Fprintln(output, "Usage:\n  bookshelf remove [--yes] [--remove-covers] ID [ID...]\n  bookshelf remove --id-or-isbn ID [--id-or-isbn ID...]\n\nWith no IDs in a terminal, opens the interactive manager for selection.")
	case "covers", "cover":
		fmt.Fprintln(output, `Usage:
  bookshelf covers                              Select books and a source
  bookshelf covers [IDs...]                     Fetch covers for specific books
  bookshelf covers --all                        Fetch missing covers for every book
  bookshelf covers --missing                    Retry every book without a stored cover
  bookshelf covers --all --source goodreads     Choose a source non-interactively

Options:
  --all                  Target every book
  --missing              Target only books without a stored cover
  --source SOURCE        automatic, goodreads, openlibrary, google, manual, or url
  --url URL              Custom image URL for exactly one book
  --replace              Replace existing stored covers
  --recompute-colors     Recompute colors when applying manual covers
  --id-or-isbn ID        Target a book by ID or ISBN; repeatable`)
	case "upgrade":
		fmt.Fprintln(output, "Usage:\n  bookshelf upgrade [--check] [--force] [--yes]\n\n  --check  Report whether an upgrade is available without installing\n  --force  Reinstall even when the installed version is current\n  --yes    Skip confirmation; required when no terminal is available")
	case "uninstall":
		fmt.Fprintln(output, "Usage:\n  bookshelf uninstall [--purge|--delete-data] [--yes]\n\nBy default, removes the command and generated website while preserving everything under data/.\n\n  --purge        Also permanently delete all Bookshelf user data\n  --delete-data  Alias for --purge\n  --yes          Skip confirmation; required when no terminal is available")
	case "version":
		fmt.Fprintln(output, "Usage:\n  bookshelf version")
	default:
		return false
	}
	return true
}

func buildCommand(ctx context.Context, paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recompute := flags.Bool("recompute-colors", false, "recompute spine colors")
	if err := flags.Parse(args); err != nil {
		return err
	}
	stats, err := library.Build(ctx, paths, library.BuildOptions{
		RecomputeColors: *recompute,
	})
	if err != nil {
		return err
	}
	printStats(stats)
	fmt.Printf("Website files: %s\n", paths.PublicDir)
	fmt.Println("Preview locally: bookshelf preview")
	return nil
}

func previewCommand(ctx context.Context, paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("preview", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	port := flags.Int("port", 8000, "local port; use 0 to choose an available port")
	noOpen := flags.Bool("no-open", false, "do not open the default browser")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return fmt.Errorf("start preview server: %w; try `bookshelf preview --port 0`", err)
	}
	defer listener.Close()
	address := listener.Addr().String()
	url := "http://" + address + "/"
	server := &http.Server{
		Handler:           previewHandler(paths),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()

	fmt.Printf("Previewing Bookshelf at %s\n", url)
	fmt.Printf("Serving %s\n", paths.PublicDir)
	fmt.Println("Press Ctrl+C to stop.")
	if !*noOpen {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "Could not open a browser automatically: %v\n", err)
		}
	}

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func previewHandler(paths library.Paths) http.Handler {
	return http.FileServer(http.Dir(paths.PublicDir))
}

func openBrowser(url string) error {
	type browserCommand struct {
		name string
		args []string
	}
	commands := []browserCommand{
		{name: "xdg-open", args: []string{url}},
		{name: "gio", args: []string{"open", url}},
	}
	for _, candidate := range commands {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			continue
		}
		command := exec.Command(path, candidate.args...)
		if err := command.Start(); err == nil {
			go func() { _ = command.Wait() }()
			return nil
		}
	}
	return fmt.Errorf("no supported browser opener was found; open %s manually", url)
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

func settingsCommand(paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("settings", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	style := flags.String("permalink-style", "", "formatted-isbn, compact-isbn, or title-slug")
	statistics := flags.String("statistics", "", "show or hide")
	defaultView := flags.String("default-view", "", "shelf, stack, or coverflow")
	defaultSort := flags.String("default-sort", "", "title, author, or year")
	sortDirection := flags.String("sort-direction", "", "ascending or descending")
	siteTitle := flags.String("site-title", "", "public website heading")
	siteSubtitle := flags.String("site-subtitle", "", "public website subtitle")
	hideSubtitle := flags.Bool("hide-subtitle", false, "hide the public website subtitle")
	randomButton := flags.String("random", "", "show or hide")
	isbnLinks := flags.String("isbn-links", "", "both, wikipedia, or goodreads")
	footer := flags.String("footer", "", "show or hide")
	footerText := flags.String("footer-text", "", "replace the built-in footer attribution")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("provide at most one permalink style")
	}
	if flags.NArg() == 1 {
		if *style != "" {
			return fmt.Errorf("choose a positional style or --permalink-style, not both")
		}
		*style = flags.Arg(0)
	}
	config, err := library.LoadConfig(paths)
	if err != nil {
		return err
	}
	hasOptions := *style != "" || *statistics != "" || *defaultView != "" || *defaultSort != "" ||
		*sortDirection != "" || *siteTitle != "" || *siteSubtitle != "" || *hideSubtitle ||
		*randomButton != "" || *isbnLinks != "" || *footer != "" || *footerText != ""
	if !hasOptions {
		if !tui.IsTerminal() {
			fmt.Printf("Website title: %s\n", config.SiteTitle)
			fmt.Printf("Website subtitle: %s\n", config.SiteSubtitle)
			fmt.Printf("Statistics: %s\n", map[bool]string{true: "shown", false: "hidden"}[config.ShowStatistics])
			fmt.Printf("Random book button: %s\n", map[bool]string{true: "shown", false: "hidden"}[config.ShowRandom])
			fmt.Printf("Default desktop view: %s\n", config.DefaultView)
			fmt.Printf("Default sort: %s\n", config.DefaultSort)
			fmt.Printf("Sort direction: %s\n", config.DefaultSortOrder)
			fmt.Printf("ISBN link sources: %s\n", config.ISBNLinkSources)
			fmt.Printf("Permalink style: %s\n", config.PermalinkStyle)
			fmt.Printf("Footer: %s\n", map[bool]string{true: "shown", false: "hidden"}[config.ShowFooter])
			fmt.Printf("Footer text: %s\n", config.FooterText)
			return nil
		}
		updated, confirmed, err := tui.RunSettingsForm(config)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
		config = updated
	} else if *style != "" {
		parsed, err := library.ParsePermalinkStyle(*style)
		if err != nil {
			return err
		}
		config.PermalinkStyle = parsed
	}
	if *statistics != "" {
		config.ShowStatistics, err = parseVisibility(*statistics, "statistics")
		if err != nil {
			return err
		}
	}
	if *defaultView != "" {
		config.DefaultView, err = library.ParseWebsiteView(*defaultView)
		if err != nil {
			return err
		}
	}
	if *defaultSort != "" {
		config.DefaultSort, err = library.ParseWebsiteSort(*defaultSort)
		if err != nil {
			return err
		}
	}
	if *sortDirection != "" {
		config.DefaultSortOrder, err = library.ParseSortDirection(*sortDirection)
		if err != nil {
			return err
		}
	}
	if *siteTitle != "" {
		config.SiteTitle = *siteTitle
	}
	if *hideSubtitle {
		if *siteSubtitle != "" {
			return fmt.Errorf("choose --site-subtitle or --hide-subtitle, not both")
		}
		config.SiteSubtitle = ""
	} else if *siteSubtitle != "" {
		config.SiteSubtitle = *siteSubtitle
	}
	if *randomButton != "" {
		config.ShowRandom, err = parseVisibility(*randomButton, "random button")
		if err != nil {
			return err
		}
	}
	if *isbnLinks != "" {
		config.ISBNLinkSources, err = library.ParseISBNLinkSources(*isbnLinks)
		if err != nil {
			return err
		}
	}
	if *footer != "" {
		config.ShowFooter, err = parseVisibility(*footer, "footer")
		if err != nil {
			return err
		}
	}
	if *footerText != "" {
		config.FooterText = *footerText
	}
	books, err := library.Load(paths)
	if err != nil {
		return err
	}
	if problems := library.Validate(books); len(problems) > 0 {
		return fmt.Errorf("cannot publish permalink setting: %v", problems[0])
	}
	if err := library.SaveConfig(paths, config); err != nil {
		return err
	}
	if err := library.SaveGenerated(paths, books); err != nil {
		return err
	}
	fmt.Println("Settings saved and published website data updated.")
	return nil
}

func parseVisibility(value, name string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "show", "shown", "on", "true":
		return true, nil
	case "hide", "hidden", "off", "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid %s setting %q; use show or hide", name, value)
	}
}

func addCommand(ctx context.Context, paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	input, noBuild := bookFlags(flags)
	batch := addBatchFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q; run `bookshelf help add`", flags.Arg(0))
	}
	batch.noBuild = *noBuild
	if batch.from != "" {
		if hasBookInput(*input) {
			return fmt.Errorf("--from cannot be combined with single-book fields")
		}
		return runBatchImport(ctx, paths, batch)
	}
	var book library.Book
	if strings.TrimSpace(input.Title) == "" && tui.IsTerminal() {
		result, err := tui.RunBookForm(nil)
		if err != nil {
			return err
		}
		if result.Cancelled {
			return nil
		}
		book = result.Book
		*noBuild = !result.Build
	} else {
		book = library.FromInput(*input)
	}
	added, stats, err := library.Add(ctx, paths, book, library.ChangeOptions{
		Build: !*noBuild,
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

type batchImportFlags struct {
	from           string
	format         string
	skipDuplicates bool
	noBuild        bool
	dryRun         bool
}

func addBatchFlags(flags *flag.FlagSet) *batchImportFlags {
	options := &batchImportFlags{}
	flags.StringVar(&options.from, "from", "", "import books from a JSON or CSV file; use - for stdin")
	flags.StringVar(&options.format, "format", "", "import format: json or csv")
	flags.BoolVar(&options.skipDuplicates, "skip-duplicates", false, "skip existing or repeated IDs/ISBNs")
	flags.BoolVar(&options.dryRun, "dry-run", false, "parse and validate without saving")
	return options
}

func importCommand(ctx context.Context, paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := &batchImportFlags{}
	flags.StringVar(&options.format, "format", "", "import format: json or csv")
	flags.BoolVar(&options.skipDuplicates, "skip-duplicates", false, "skip existing or repeated IDs/ISBNs")
	flags.BoolVar(&options.noBuild, "no-build", false, "save without refreshing published data")
	flags.BoolVar(&options.dryRun, "dry-run", false, "parse and validate without saving")
	if len(args) > 0 && (args[0] == "-" || !strings.HasPrefix(args[0], "-")) {
		options.from = args[0]
		args = args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if options.from == "" && flags.NArg() == 1 {
		options.from = flags.Arg(0)
	} else if flags.NArg() != 0 {
		return fmt.Errorf("provide exactly one import file")
	}
	if options.from == "" {
		return fmt.Errorf("import file is required; run `bookshelf help import`")
	}
	return runBatchImport(ctx, paths, options)
}

func exportCommand(paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "", "export format: json or csv")
	force := flags.Bool("force", false, "replace an existing export file")
	destination := ""
	if len(args) > 0 && (args[0] == "-" || !strings.HasPrefix(args[0], "-")) {
		destination = args[0]
		args = args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if destination == "" && flags.NArg() == 1 {
		destination = flags.Arg(0)
	} else if flags.NArg() != 0 {
		return fmt.Errorf("provide exactly one export file")
	}
	if destination == "" {
		return fmt.Errorf("export file is required; run `bookshelf help export`")
	}

	selectedFormat := strings.TrimSpace(*format)
	if selectedFormat == "" && destination != "-" {
		selectedFormat = strings.TrimPrefix(filepath.Ext(destination), ".")
	}
	if selectedFormat == "" {
		return fmt.Errorf("--format is required when exporting to standard output")
	}
	selectedFormat = strings.ToLower(strings.TrimPrefix(selectedFormat, "."))
	if selectedFormat != "json" && selectedFormat != "csv" {
		return fmt.Errorf("unsupported export format %q; use json or csv", selectedFormat)
	}

	books, err := library.Load(paths)
	if err != nil {
		return err
	}
	if destination == "-" {
		return library.EncodeExport(os.Stdout, books, selectedFormat)
	}
	if !*force {
		if _, err := os.Stat(destination); err == nil {
			return fmt.Errorf("%s already exists; use --force to replace it", destination)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := writeExportFile(destination, books, selectedFormat); err != nil {
		return err
	}
	fmt.Printf("Exported %d book(s) to %s (%s).\n", len(books), destination, strings.ToUpper(selectedFormat))
	return nil
}

func writeExportFile(destination string, books []library.Book, format string) error {
	directory := filepath.Dir(destination)
	temp, err := os.CreateTemp(directory, "."+filepath.Base(destination)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := library.EncodeExport(temp, books, format); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, destination)
}

func runBatchImport(ctx context.Context, paths library.Paths, options *batchImportFlags) error {
	var reader io.Reader
	var file *os.File
	format := strings.TrimSpace(options.format)
	if options.from == "-" {
		reader = os.Stdin
		if format == "" {
			return fmt.Errorf("--format is required when importing from standard input")
		}
	} else {
		opened, err := os.Open(options.from)
		if err != nil {
			return err
		}
		file = opened
		defer file.Close()
		reader = file
		if format == "" {
			format = strings.TrimPrefix(filepath.Ext(options.from), ".")
		}
	}
	books, err := library.DecodeImport(io.LimitReader(reader, 64<<20), format)
	if err != nil {
		return err
	}
	result, err := library.Import(ctx, paths, books, library.ImportOptions{
		SkipDuplicates: options.skipDuplicates,
		Build:          !options.noBuild,
		DryRun:         options.dryRun,
	})
	if err != nil {
		return err
	}
	verb := "Imported"
	if options.dryRun {
		verb = "Would import"
	}
	fmt.Printf("%s %d book(s). Skipped: %d.\n", verb, result.Imported, result.Skipped)
	if !options.noBuild && !options.dryRun && result.Imported > 0 {
		printStats(result.Build)
	}
	return nil
}

func hasBookInput(input library.BookInput) bool {
	return input.Title != "" || input.Author != "" || input.ISBN != "" || input.Translator != "" ||
		input.Slug != "" || input.Publisher != "" || input.Binding != "" || input.Published != ""
}

func editCommand(ctx context.Context, paths library.Paths, args []string) error {
	flags := flag.NewFlagSet("edit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	id := flags.String("id-or-isbn", "", "book id or ISBN")
	patch, noBuild := updateFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		if tui.IsTerminal() {
			books, err := library.Load(paths)
			if err != nil {
				return err
			}
			workflow, err := tui.RunEditWorkflow(books)
			if err != nil || !workflow.Confirmed {
				return err
			}
			edited, stats, err := library.Replace(ctx, paths, workflow.Original.ID, workflow.Form.Book, library.ChangeOptions{
				Build: workflow.Form.Build,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Edited %q.\n", edited.Title)
			if workflow.Form.Build {
				printStats(stats)
			}
			return nil
		}
		return fmt.Errorf("--id-or-isbn is required")
	}
	if tui.IsTerminal() && emptyBookPatch(*patch) {
		return runInteractiveEdit(ctx, paths, *id)
	}
	updated, stats, err := library.Update(ctx, paths, *id, *patch, library.ChangeOptions{
		Build: !*noBuild,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Edited %q.\n", updated.Title)
	if !*noBuild {
		printStats(stats)
	}
	return nil
}

func runInteractiveEdit(ctx context.Context, paths library.Paths, id string) error {
	selected, err := booksForIDs(paths, []string{id})
	if err != nil {
		return err
	}
	form, err := tui.RunBookForm(&selected[0])
	if err != nil || form.Cancelled {
		return err
	}
	edited, stats, err := library.Replace(ctx, paths, selected[0].ID, form.Book, library.ChangeOptions{
		Build: form.Build,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Edited %q.\n", edited.Title)
	if form.Build {
		printStats(stats)
	}
	return nil
}

func emptyBookPatch(patch library.BookPatch) bool {
	return patch.Title == nil && patch.Author == nil && patch.ISBN == nil && patch.Slug == nil &&
		patch.Translator == nil && patch.Publisher == nil && patch.Binding == nil && patch.Published == nil
}

func removeCommand(ctx context.Context, paths library.Paths, args []string) error {
	ids, yes, removeCovers, err := parseRemoveArgs(args)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		if tui.IsTerminal() {
			books, loadErr := library.Load(paths)
			if loadErr != nil {
				return loadErr
			}
			workflow, workflowErr := tui.RunRemoveWorkflow(books)
			if workflowErr != nil || !workflow.Confirmed {
				return workflowErr
			}
			ids = workflow.IDs
			removeCovers = workflow.RemoveCovers
			yes = true
			if len(ids) == 0 {
				return err
			}
		} else {
			return fmt.Errorf("provide at least one id or ISBN")
		}
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
	options, err := parseCoversArgs(args)
	if err != nil {
		return err
	}
	books, err := library.Load(paths)
	if err != nil {
		return err
	}
	var selected []library.Book
	interactiveSelection := !options.all && !options.missing && len(options.ids) == 0 && tui.IsTerminal()
	if !options.all && !options.missing && len(options.ids) == 0 && !tui.IsTerminal() {
		return fmt.Errorf("provide book IDs, --all, or --missing")
	}
	source := options.source
	startedInteractively := interactiveSelection
	if interactiveSelection {
		workflow, workflowErr := tui.RunCoverWorkflow(books, nil)
		if workflowErr != nil {
			return workflowErr
		}
		if !workflow.Confirmed {
			return nil
		}
		options.ids = workflow.IDs
		options.url = workflow.URL
		source = workflow.Source
		interactiveSelection = false
	}
	var previousSelection []string
selectionLoop:
	for {
		switch {
		case options.all:
			selected = books
		case options.missing:
			selected = booksMissingCovers(books)
			if len(selected) == 0 {
				fmt.Println("Every book already has a stored cover.")
				return nil
			}
		case len(options.ids) > 0:
			selected, err = booksForIDs(paths, options.ids)
			if err != nil {
				return err
			}
		default:
			ids, confirmed, selectErr := tui.RunBookSelector(books, nil, previousSelection, "Bookshelf · Covers", true, true)
			if selectErr != nil {
				return selectErr
			}
			if !confirmed {
				return nil
			}
			previousSelection = append(previousSelection[:0], ids...)
			selected, err = booksForIDs(paths, ids)
			if err != nil {
				return err
			}
		}
		if len(selected) == 0 {
			fmt.Println("No books selected.")
			return nil
		}
		for source == "" {
			if !tui.IsTerminal() {
				return fmt.Errorf("--source is required when no terminal is available")
			}
			choice, chooseErr := tui.ChooseCoverSource(selected)
			err = chooseErr
			if err != nil {
				return err
			}
			if !choice.Confirmed {
				if interactiveSelection {
					selected = nil
					continue selectionLoop
				}
				return nil
			}
			source = choice.Source
			options.url = choice.URL
		}
		break
	}
	if source == library.CoverSourceManual {
		ids := make([]string, 0, len(selected))
		for _, book := range selected {
			ids = append(ids, book.ID)
		}
		stats, applyErr := library.ApplyManualCovers(ctx, paths, ids, options.recompute)
		if applyErr != nil {
			return applyErr
		}
		printStats(stats)
		return nil
	}

	session, err := library.NewCoverFetchSession(paths, selected, options.replace)
	if err != nil {
		return err
	}
	if source == library.CoverSourceURL {
		if len(selected) != 1 {
			_ = session.Discard()
			return fmt.Errorf("a custom cover URL requires exactly one book")
		}
		if options.url == "" {
			_ = session.Discard()
			return fmt.Errorf("--url is required with --source url")
		}
		session.SetCustomURL(options.url)
	}
	if tui.IsTerminal() {
		summary, kept, back, progressErr := tui.RunCoverProgress(ctx, session, source)
		if progressErr != nil {
			return progressErr
		}
		if !kept {
			fmt.Println("Cover fetching cancelled; fetched covers were discarded.")
			return nil
		}
		if back {
			if startedInteractively {
				return coversCommand(ctx, paths, nil)
			}
			return nil
		}
		if summary.Downloaded > 0 {
			if len(selected) == 1 && summary.Downloaded == 1 {
				return tui.OfferCoverPreview(library.CoverPath(paths, selected[0]), false)
			}
			return tui.OfferCoverPreview(paths.CoversDir, true)
		}
		return nil
	}
	for index, book := range session.Books() {
		if err := ctx.Err(); err != nil {
			_ = session.Discard()
			return err
		}
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", index+1, len(selected), book.Title)
		session.Record(session.Fetch(ctx, index, source))
	}
	summary, err := session.Commit()
	if err != nil {
		_ = session.Discard()
		return err
	}
	return printCoverSummary(paths, session, summary)
}

func booksMissingCovers(books []library.Book) []library.Book {
	missing := make([]library.Book, 0, len(books))
	for _, book := range books {
		if book.Cover == "" {
			missing = append(missing, book)
		}
	}
	return missing
}

func printCoverSummary(paths library.Paths, session *library.CoverFetchSession, summary library.CoverFetchSummary) error {
	fmt.Printf("Covers complete. Downloaded: %d. Skipped: %d. Not found: %d. Failed: %d. Colors: %d.\n",
		summary.Downloaded, summary.Skipped, summary.NotFound, summary.Failed, summary.Colored)
	if summary.Downloaded > 0 {
		fmt.Printf("Covers saved in: %s\n", paths.CoversDir)
	}
	reportPath, count, err := session.WriteReport()
	if err != nil {
		return err
	}
	if count > 0 {
		fmt.Printf("%d book(s) need attention. Report: %s\n", count, reportPath)
	}
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
		_, err := tui.RunBrowser(books, library.PublicationStatuses(paths, books))
		return err
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

func bookFlags(flags *flag.FlagSet) (*library.BookInput, *bool) {
	input := &library.BookInput{}
	flags.StringVar(&input.Title, "title", "", "title")
	flags.StringVar(&input.Author, "author", "", "author")
	flags.StringVar(&input.ISBN, "isbn", "", "ISBN")
	flags.StringVar(&input.Slug, "slug", "", "optional website URL slug")
	flags.StringVar(&input.Translator, "translator", "", "translator")
	flags.StringVar(&input.Publisher, "publisher", "", "publisher")
	flags.StringVar(&input.Binding, "binding", "", "binding")
	flags.StringVar(&input.Published, "published", "", "published year")
	noBuild := flags.Bool("no-build", false, "save without refreshing published data")
	return input, noBuild
}

func updateFlags(flags *flag.FlagSet) (*library.BookPatch, *bool) {
	patch := &library.BookPatch{}
	optionalStringFlag(flags, "title", "title; pass an empty value only to receive a validation error", &patch.Title)
	optionalStringFlag(flags, "author", "author; pass an empty value to clear", &patch.Author)
	optionalStringFlag(flags, "isbn", "ISBN; pass an empty value to clear", &patch.ISBN)
	optionalStringFlag(flags, "slug", "website URL slug; pass an empty value to use the ISBN or book id", &patch.Slug)
	optionalStringFlag(flags, "translator", "translator; pass an empty value to clear", &patch.Translator)
	optionalStringFlag(flags, "publisher", "publisher; pass an empty value to clear", &patch.Publisher)
	optionalStringFlag(flags, "binding", "binding; pass an empty value to clear", &patch.Binding)
	optionalStringFlag(flags, "published", "published year; pass an empty value to clear", &patch.Published)
	noBuild := flags.Bool("no-build", false, "save without refreshing published data")
	return patch, noBuild
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

type coversOptions struct {
	ids       []string
	all       bool
	missing   bool
	recompute bool
	replace   bool
	source    library.CoverSource
	confirmed bool
	url       string
}

func parseCoversArgs(args []string) (coversOptions, error) {
	var options coversOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--all":
			options.all = true
		case arg == "--missing":
			options.missing = true
		case arg == "--recompute-colors":
			options.recompute = true
		case arg == "--replace":
			options.replace = true
		case arg == "--url":
			if i+1 >= len(args) {
				return coversOptions{}, fmt.Errorf("--url requires a value")
			}
			i++
			options.url = strings.TrimSpace(args[i])
			options.source = library.CoverSourceURL
		case strings.HasPrefix(arg, "--url="):
			options.url = strings.TrimSpace(strings.TrimPrefix(arg, "--url="))
			options.source = library.CoverSourceURL
		case arg == "--source":
			if i+1 >= len(args) {
				return coversOptions{}, fmt.Errorf("--source requires a value")
			}
			i++
			source, err := library.ParseCoverSource(args[i])
			if err != nil {
				return coversOptions{}, err
			}
			options.source = source
		case strings.HasPrefix(arg, "--source="):
			source, err := library.ParseCoverSource(strings.TrimPrefix(arg, "--source="))
			if err != nil {
				return coversOptions{}, err
			}
			options.source = source
		case arg == "--id-or-isbn":
			if i+1 >= len(args) {
				return coversOptions{}, fmt.Errorf("--id-or-isbn requires a value")
			}
			i++
			options.ids = append(options.ids, args[i])
		case strings.HasPrefix(arg, "--id-or-isbn="):
			options.ids = append(options.ids, strings.TrimPrefix(arg, "--id-or-isbn="))
		case strings.HasPrefix(arg, "-"):
			return coversOptions{}, fmt.Errorf("unknown covers option %q", arg)
		default:
			options.ids = append(options.ids, arg)
		}
	}
	if options.all && len(options.ids) > 0 {
		return coversOptions{}, fmt.Errorf("--all cannot be combined with book IDs")
	}
	if options.missing && options.all {
		return coversOptions{}, fmt.Errorf("--missing cannot be combined with --all")
	}
	if options.missing && len(options.ids) > 0 {
		return coversOptions{}, fmt.Errorf("--missing cannot be combined with book IDs")
	}
	if options.missing && options.replace {
		return coversOptions{}, fmt.Errorf("--missing cannot be combined with --replace")
	}
	if options.missing && (options.url != "" || options.source == library.CoverSourceURL) {
		return coversOptions{}, fmt.Errorf("--missing cannot be combined with a custom URL")
	}
	if options.url != "" && options.source != library.CoverSourceURL {
		return coversOptions{}, fmt.Errorf("--url cannot be combined with a different --source")
	}
	if options.url != "" && (options.all || len(options.ids) != 1) {
		return coversOptions{}, fmt.Errorf("--url requires exactly one book ID")
	}
	return options, nil
}

func printStats(stats library.BuildStats) {
	fmt.Printf("Done. Books: %d. Processed: %d. Manual covers: %d. Colors: %d. Missing covers: %d.\n",
		stats.Books, stats.Processed, stats.Manuals, stats.Colored, stats.Missing)
}

func upgradeCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	check := flags.Bool("check", false, "check for an available upgrade without installing")
	force := flags.Bool("force", false, "reinstall even when already current")
	yes := flags.Bool("yes", false, "skip confirmation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	latest, err := latestRelease(ctx)
	if err != nil {
		return fmt.Errorf("check latest release: %w", err)
	}
	if sameVersion(version, latest) {
		if *check || !*force {
			fmt.Printf("Bookshelf %s is already the latest version.\n", displayVersion(version))
			return nil
		}
	} else if *check {
		fmt.Printf("Upgrade available: %s → %s.\n", displayVersion(version), displayVersion(latest))
		return nil
	}
	if !*yes {
		if !tui.IsTerminal() {
			return fmt.Errorf("--yes is required when no terminal is available")
		}
		choice, chosen, decisionErr := tui.RunDecision(tui.DecisionRequest{
			Title:       "Upgrade Bookshelf?",
			Description: fmt.Sprintf("Replace Bookshelf %s with %s.", displayVersion(version), displayVersion(latest)),
			Options: []tui.DecisionOption{
				{ID: "upgrade", Label: "Upgrade"},
				{ID: "cancel", Label: "Cancel"},
			},
			Default: 1,
		})
		if decisionErr != nil {
			return decisionErr
		}
		if !chosen || choice != "upgrade" {
			return nil
		}
	}

	url := strings.TrimSpace(os.Getenv("BOOKSHELF_INSTALLER_URL"))
	if url == "" {
		url = defaultInstallerURL
	}
	fmt.Printf("Upgrading Bookshelf %s → %s for %s/%s...\n", displayVersion(version), displayVersion(latest), runtime.GOOS, runtime.GOARCH)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := httpClient.Do(request)
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
	command.Env = environmentWith("BOOKSHELF_VERSION", latest)
	return command.Run()
}

func environmentWith(name, value string) []string {
	prefix := name + "="
	environment := os.Environ()
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}

func latestRelease(ctx context.Context) (string, error) {
	url := strings.TrimSpace(os.Getenv("BOOKSHELF_LATEST_RELEASE_URL"))
	if url == "" {
		url = defaultLatestReleaseURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "bookshelf/"+version)
	response, err := httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub Releases API: %s", response.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return "", err
	}
	release.TagName = strings.TrimSpace(release.TagName)
	if release.TagName == "" {
		return "", fmt.Errorf("latest release has no tag")
	}
	return release.TagName, nil
}

func sameVersion(left, right string) bool {
	normalize := func(value string) string {
		return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "v")
	}
	return normalize(left) == normalize(right)
}

func displayVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "dev"
	}
	if value != "dev" && !strings.HasPrefix(strings.ToLower(value), "v") {
		return "v" + value
	}
	return value
}

func uninstallCommand(args []string) error {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	force := flags.Bool("force", false, "allow uninstall outside the installed binary")
	purge := flags.Bool("purge", false, "also permanently delete all Bookshelf user data")
	deleteData := flags.Bool("delete-data", false, "alias for --purge")
	yes := flags.Bool("yes", false, "skip confirmation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	shouldPurge := *purge || *deleteData
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
	if !*yes {
		if !tui.IsTerminal() {
			return fmt.Errorf("confirmation requires a terminal; rerun with --yes after reviewing `bookshelf help uninstall`")
		}
		confirmed, err := tui.ConfirmUninstall(binPath, installDir, shouldPurge)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}
	if err := os.Remove(binPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, completionPath := range completionPaths(home) {
		if err := os.Remove(completionPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if shouldPurge {
		if err := os.RemoveAll(installDir); err != nil {
			return err
		}
		fmt.Printf("Removed Bookshelf command and all data:\n%s\n%s\n", binPath, installDir)
		return nil
	}
	publicDir := filepath.Join(installDir, "public")
	if err := os.RemoveAll(publicDir); err != nil {
		return err
	}
	_ = os.Remove(installDir)
	fmt.Printf("Removed Bookshelf command and generated website:\n%s\n%s\n", binPath, publicDir)
	fmt.Printf("Preserved books, covers, and settings in %s\n", filepath.Join(installDir, "data"))
	fmt.Println("To delete them later, remove that directory or reinstall and run `bookshelf uninstall --purge`.")
	return nil
}

func completionPaths(home string) []string {
	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	return []string{
		filepath.Join(dataHome, "bash-completion", "completions", "bookshelf"),
		filepath.Join(dataHome, "zsh", "site-functions", "_bookshelf"),
		filepath.Join(configHome, "fish", "completions", "bookshelf.fish"),
	}
}
