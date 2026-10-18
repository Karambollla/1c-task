package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"edition/internal/application"
	"edition/internal/edition"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "init":
		initDB(os.Args[2:])
	case "serve":
		serve(os.Args[2:])
	case "client":
		clientAdmin(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func base(fs *flag.FlagSet) *string {
	return fs.String("db", "edition.db", "SQLite database path")
}

func open(path string) (*edition.Store, error) {
	return edition.Open(path, edition.DefaultConfig())
}

func initDB(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	db := base(fs)
	id := fs.String("editor-id", "editor", "editor client ID")
	secret := fs.String("editor-secret", "", "editor secret")
	fs.Parse(args)
	if *secret == "" {
		fatal("editor secret is required")
	}
	s, err := open(*db)
	if err != nil {
		fatal("open database", "error", err)
	}
	defer s.Close()
	if err = s.CreateUser(context.Background(), *id, edition.RoleEditor, *secret, 0); err != nil {
		fatal("create editor", "error", err)
	}
	fmt.Println("database initialized")
}
func clientAdmin(args []string) {
	if len(args) < 1 {
		fatal("expected client add or client trust")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("client add", flag.ExitOnError)
		db := base(fs)
		id := fs.String("id", "", "client ID")
		secret := fs.String("secret", "", "client secret")
		trust := fs.Float64("trust", 0, "initial trust")
		fs.Parse(args[1:])
		s, err := open(*db)
		if err != nil {
			fatal("open database", "error", err)
		}
		defer s.Close()
		if err = s.CreateUser(context.Background(), *id, edition.RoleReporter, *secret, *trust); err != nil {
			fatal("create reporter", "error", err)
		}
		fmt.Println("reporter added")
	case "trust":
		fs := flag.NewFlagSet("client trust", flag.ExitOnError)
		db := base(fs)
		id := fs.String("id", "", "client ID")
		trust := fs.Float64("trust", -1, "new trust")
		fs.Parse(args[1:])
		s, err := open(*db)
		if err != nil {
			fatal("open database", "error", err)
		}
		defer s.Close()
		if err = s.SetTrust(context.Background(), *id, *trust); err != nil {
			fatal("update reporter trust", "error", err)
		}
		fmt.Println("trust updated")
	default:
		fatal("expected client add or client trust")
	}
}
func serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	db := base(fs)
	addr := fs.String("listen", ":9000", "TCP listen address")
	threshold := fs.Float64("publish-threshold", 1.05, "score needed to publish")
	half := fs.Duration("half-life", 6*time.Hour, "trust half-life")
	congrats := fs.Int("congratulations", 3, "number of congratulated sources")
	fs.Parse(args)
	cfg := edition.Config{Threshold: *threshold, HalfLife: *half, Congratulations: *congrats}
	if cfg.HalfLife <= 0 || cfg.Threshold < 0 || cfg.Congratulations < 0 {
		fatal("invalid publishing configuration")
	}
	s, err := edition.Open(*db, cfg)
	if err != nil {
		fatal("open database", "error", err)
	}
	defer s.Close()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fatal("listen", "address", *addr, "error", err)
	}
	slog.Info("edition server listening", "address", ln.Addr(), "threshold", cfg.Threshold, "half_life", cfg.HalfLife, "congratulations", cfg.Congratulations)
	if err := (&edition.Server{Application: application.New(s)}).Serve(ln); err != nil {
		fatal("serve", "error", err)
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "usage: edition-server <init|serve|client>")
}

func fatal(message string, args ...any) {
	slog.Error(message, args...)
	os.Exit(1)
}
