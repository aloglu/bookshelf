// Command bookshelf serves a Bookshelf library: the shelf for visitors and
// the admin for its owner, in the browser, from one address.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aloglu/bookshelf/internal/auth"
	"github.com/aloglu/bookshelf/internal/backup"
	"github.com/aloglu/bookshelf/internal/lookup"
	"github.com/aloglu/bookshelf/internal/server"
	"github.com/aloglu/bookshelf/internal/store"
	"golang.org/x/term"
)

// version is set when releases are built (-ldflags "-X main.version=2.0.0").
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bookshelf:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "health" {
		return health()
	}
	setPassword := len(args) > 0 && args[0] == "set-password"
	if setPassword {
		args = args[1:]
	}
	flags := flag.NewFlagSet("bookshelf", flag.ContinueOnError)
	dataDir := flags.String("data-dir", env("BOOKSHELF_DATA_DIR", defaultDataDir()), "the folder Bookshelf keeps your library in")
	host := flags.String("host", env("BOOKSHELF_HOST", ""), "the address to listen on (empty: every address of this computer)")
	port := flags.Int("port", envInt("PORT", 8080), "the port to listen on")
	showVersion := flags.Bool("version", false, "print the version and stop")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), `Bookshelf %s

Usage: bookshelf [options]

Settings come from these environment variables (or the options below):
  BOOKSHELF_ADMIN_PASSWORD  the password to sign in with (or set one once with
                            bookshelf set-password)
  BOOKSHELF_ADMIN_USER      the user name (default: admin)
  BOOKSHELF_DATA_DIR        where the library is kept
  PORT                      the port (default: 8080)
  BOOKSHELF_TRUSTED_PROXIES reverse proxies to believe about visitors' addresses,
                            for example "172.17.0.0/16"
  BOOKSHELF_GOOGLE_API_KEY  an optional Google Books key for ISBN lookups
  PUID, PGID                in Docker, the user and group to run as (default 1000)

Commands:
  bookshelf set-password    choose the password once; it is kept, hashed, in
                            the data folder (BOOKSHELF_ADMIN_PASSWORD wins if set)
  bookshelf health          check that Bookshelf answers (for health checks)

Options:
`, version)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Println("bookshelf", version)
		return nil
	}

	if setPassword {
		return choosePassword(*dataDir)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	// Refuse before touching anything: no data folder is created without a password.
	password, err := signInPassword(*dataDir)
	if err != nil {
		return err
	}

	if err := dropPrivileges(*dataDir); err != nil {
		return err
	}
	st, err := store.Open(*dataDir)
	if err != nil {
		if errors.Is(err, store.ErrInUse) {
			return fmt.Errorf("%w. Stop the other one (an older container, a second copy of the program, or a test pointed at the same folder) and start again", err)
		}
		return fmt.Errorf("open the library in %s: %w", *dataDir, err)
	}
	defer st.Close()

	a, err := auth.New(st, os.Getenv("BOOKSHELF_ADMIN_USER"), password, os.Getenv("BOOKSHELF_TRUSTED_PROXIES"))
	if err != nil {
		return err
	}
	backups := backup.New(*dataDir, st, version, log)
	srv := &server.Server{Store: st, Auth: a, Lookup: lookup.New(os.Getenv("BOOKSHELF_GOOGLE_API_KEY"), version), Backups: backups,
		DataDir: *dataDir, Version: version, Web: webFS(), Log: log}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go housekeeping(ctx, st, log)
	go backups.Run(ctx) // scheduled backups and the unused-cover cleanup

	httpServer := &http.Server{
		Addr:              net.JoinHostPort(*host, strconv.Itoa(*port)),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute, // cover uploads and imports
		WriteTimeout:      5 * time.Minute, // exports and backups
		IdleTimeout:       2 * time.Minute,
	}
	listener, err := net.Listen("tcp", httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w (is another program using port %d?)", httpServer.Addr, err, *port)
	}
	log.Info("Bookshelf is running", "version", version, "address", "http://"+displayHost(*host)+":"+strconv.Itoa(*port), "data", *dataDir, "user", a.User())

	errs := make(chan error, 1)
	go func() { errs <- httpServer.Serve(listener) }()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	log.Info("stopping")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdown)
}

// health checks that a Bookshelf on this machine answers (the Docker
// image's health check), using the same port setting as the server.
// signInPassword is BOOKSHELF_ADMIN_PASSWORD if it is set, otherwise the one
// set with set-password.
func signInPassword(dataDir string) (auth.Password, error) {
	if pw := os.Getenv("BOOKSHELF_ADMIN_PASSWORD"); pw != "" {
		return auth.Plain(pw), nil
	}
	pw, err := auth.LoadPassword(dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("set BOOKSHELF_ADMIN_PASSWORD to the password you want to sign in with, or run bookshelf set-password once")
	}
	return pw, err
}

// choosePassword asks for a password twice (without showing it) and keeps it,
// hashed, in the data folder. From a pipe it reads one line instead.
func choosePassword(dataDir string) error {
	read := func(prompt string) (string, error) {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && line == "" {
				return "", errors.New("no password given")
			}
			return strings.TrimRight(line, "\r\n"), nil
		}
		fmt.Fprint(os.Stderr, prompt)
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(raw), err
	}
	first, err := read(fmt.Sprintf("New password (at least %d characters, not shown as you type): ", auth.MinPassword))
	if err != nil {
		return err
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		again, err := read("Type it again: ")
		if err != nil {
			return err
		}
		if again != first {
			return errors.New("the two passwords don’t match; nothing was changed")
		}
	}
	if err := auth.SavePassword(dataDir, first); err != nil {
		return err
	}
	fmt.Printf("The password is set (kept, hashed, in %s). Sign in as %s.\n", dataDir, env("BOOKSHELF_ADMIN_USER", "admin"))
	if os.Getenv("BOOKSHELF_ADMIN_PASSWORD") != "" {
		fmt.Println("BOOKSHELF_ADMIN_PASSWORD is set too, and wins while it is.")
	}
	fmt.Println("If Bookshelf is running, restart it to use the new password.")
	return nil
}

func health() error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(envInt("PORT", 8080)) + "/healthz")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("answered %s", resp.Status)
	}
	return nil
}

// housekeeping removes expired sessions now and every hour.
func housekeeping(ctx context.Context, st *store.Store, log *slog.Logger) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if err := st.PruneSessions(ctx); err != nil && ctx.Err() == nil {
			log.Warn("pruning sessions", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func displayHost(host string) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "localhost"
	}
	return host
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}

// defaultDataDir is the usual place for a program's data on each system. It
// is deliberately not Bookshelf 1.x's folder (~/.local/share/bookshelf).
func defaultDataDir() string {
	const name = "com.aloglu.bookshelf"
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, name)
		}
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", name)
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, name)
	}
	return filepath.Join(home, ".local", "share", name)
}
