//go:build js && wasm

package wasmsqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Options represents configuration options for opening a wasmsqlite database.
//
// Only the OO direct route is supported. SQLite runs inside the same Worker
// as the Go WASM binary via sqlite3.oo1.OpfsDb / sqlite3.oo1.DB, so there is
// no nested Worker bridge.
type Options struct {
	// File path for the database (default: "/app.db")
	File string

	// VFS to use (default: "opfs")
	VFS string

	// Busy timeout in milliseconds (default: 5000)
	BusyTimeout int

	// Whether to parse time strings as time.Time (default: false)
	ParseTime bool

	// Journal mode (default: not set, uses SQLite default)
	JournalMode string

	// Custom pragma statements to execute on open
	Pragma []string
}

// DefaultOptions returns default options for opening a database
func DefaultOptions() *Options {
	return &Options{
		File:        "/app.db",
		VFS:         "opfs",
		BusyTimeout: 5000,
		ParseTime:   false,
	}
}

// Open opens a database with the given options
func Open(opts *Options) (*sql.DB, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	// Build DSN from options
	dsn := buildDSN(opts)

	return sql.Open("wasmsqlite", dsn)
}

// buildDSN builds a DSN string from options
func buildDSN(opts *Options) string {
	values := url.Values{}

	if opts.File != "" && opts.File != "/app.db" {
		values.Set("file", opts.File)
	}

	if opts.VFS != "" && opts.VFS != "opfs" {
		values.Set("vfs", opts.VFS)
	}

	if opts.BusyTimeout != 0 && opts.BusyTimeout != 5000 {
		values.Set("busy_timeout", strconv.Itoa(opts.BusyTimeout))
	}

	if opts.ParseTime {
		values.Set("parse_time", "true")
	}

	if opts.JournalMode != "" {
		values.Set("journal_mode", opts.JournalMode)
	}

	if len(opts.Pragma) > 0 {
		values.Set("pragma", strings.Join(opts.Pragma, ";"))
	}

	if len(values) == 0 {
		return ""
	}

	return values.Encode()
}

// parseDSN parses a DSN string into options
func parseDSN(dsn string) (*Options, error) {
	opts := DefaultOptions()

	if dsn == "" {
		return opts, nil
	}

	fmt.Printf("🔍 Parsing DSN: %s\n", dsn)

	values, err := url.ParseQuery(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN: %w", err)
	}

	if file := values.Get("file"); file != "" {
		// Remove any query parameters from the file path
		if questionMark := strings.Index(file, "?"); questionMark != -1 {
			file = file[:questionMark]
		}
		fmt.Printf("🔍 Extracted file: %s\n", file)
		opts.File = file
	}

	if vfs := values.Get("vfs"); vfs != "" {
		fmt.Printf("🔍 Extracted VFS: %s\n", vfs)
		opts.VFS = vfs
	}

	if timeout := values.Get("busy_timeout"); timeout != "" {
		t, err := strconv.Atoi(timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid busy_timeout: %w", err)
		}
		opts.BusyTimeout = t
	}

	if parseTime := values.Get("parse_time"); parseTime == "true" {
		opts.ParseTime = true
	}

	if journalMode := values.Get("journal_mode"); journalMode != "" {
		opts.JournalMode = journalMode
	}

	if pragma := values.Get("pragma"); pragma != "" {
		opts.Pragma = strings.Split(pragma, ";")
	}

	return opts, nil
}
