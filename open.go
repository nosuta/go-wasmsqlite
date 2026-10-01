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
	// UnlockASAP releases the OPFS sync access handle after each operation
	// instead of holding it until the VFS goes idle. It trades I/O speed for a
	// smaller window in which a page reload can race a held handle, so it is
	// opt-in. Only meaningful for the OPFS VFS.
	UnlockASAP bool
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
// string, which is stripped. Extra options are read as sibling query
// parameters, e.g. "file=/app.db&unlock-asap=1" or "file=/app.db&vfs=opfs".
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

	switch strings.ToLower(values.Get("unlock-asap")) {
	case "1", "true", "yes", "on":
		opts.UnlockASAP = true
	}

	return opts, nil
}
