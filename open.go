//go:build js && wasm

package wasmsqlite

import (
	"fmt"
	"net/url"
	"strings"
)

// Options represents configuration options for opening a wasmsqlite database.
//
// Only the OO direct route is supported. SQLite runs inside the same Worker
// as the Go WASM binary via sqlite3.oo1.OpfsDb / sqlite3.oo1.DB, so there is
// no nested Worker bridge.
type Options struct {
	// File path for the database (default: "/app.db").
	File string
	// VFS to use (default: "opfs").
	VFS string
}

// DefaultOptions returns default options for opening a database.
func DefaultOptions() *Options {
	return &Options{
		File: "/app.db",
		VFS:  "opfs",
	}
}

// parseDSN parses a DSN string into options. The godash sqlite wrapper opens
// the driver as "file=<path>"; the file value may carry a trailing query
// string, which is stripped.
func parseDSN(dsn string) (*Options, error) {
	opts := DefaultOptions()

	if dsn == "" {
		return opts, nil
	}

	values, err := url.ParseQuery(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN: %w", err)
	}

	if file := values.Get("file"); file != "" {
		if questionMark := strings.Index(file, "?"); questionMark != -1 {
			file = file[:questionMark]
		}
		opts.File = file
	}

	if vfs := values.Get("vfs"); vfs != "" {
		opts.VFS = vfs
	}

	return opts, nil
}
