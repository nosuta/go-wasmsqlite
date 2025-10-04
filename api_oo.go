//go:build js && wasm

package wasmsqlite

import (
	"fmt"
	"sync"
	"syscall/js"
)

// APIOO adapts the JavaScript SQLite OO API to work with our Go driver
type APIOO struct {
	sqlite   js.Value
	database js.Value
	mu       sync.Mutex
}

// NewAPIOO creates a new OO API
func NewAPIOO() (*APIOO, error) {
	return &APIOO{}, nil
}

// Init initializes OO API
func (b *APIOO) Init() error {
	if !b.sqlite.IsNull() && !b.sqlite.IsUndefined() {
		return nil
	}
	sqlite3InitModule := js.Global().Get("sqlite3InitModule")
	if sqlite3InitModule.IsUndefined() {
		return fmt.Errorf("missing sqlite3InitModule")
	}
	sqlite, err := callAsync(sqlite3InitModule)
	if err != nil {
		return fmt.Errorf("failed to initialize sqlite3: %s", err)
	}
	b.sqlite = sqlite
	return nil
}

// Open opens a database
func (b *APIOO) Open(path, vfs string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.Init(); err != nil {
		return "", err
	}

	if !b.database.IsNull() && !b.database.IsUndefined() {
		return "opfs", nil
	}

	opfs := b.sqlite.Get("opfs")
	if opfs.IsUndefined() {
		return "", fmt.Errorf("OPFS is not supported")
	}
	fmt.Printf("🔍 sqlite3 version: %s\n", b.sqlite.Get("version").Get("libVersion").String())

	db := b.sqlite.Get("oo1").Get("OpfsDb").New(path, "c")
	if db.IsNull() || db.IsUndefined() {
		return "", fmt.Errorf("failed to create database")
	}
	b.database = db
	return "opfs", nil
}

// Exec executes a SQL statement
func (b *APIOO) Exec(sql string, params []any) (int, int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.database.IsUndefined() {
		return 0, 0, fmt.Errorf("missing database")
	}

	// Convert params to JavaScript array
	jsParams := js.Global().Get("Array").New()
	for i, param := range params {
		jsParams.SetIndex(i, toJSValue(param))
	}

	opts := map[string]any{
		"sql":  sql,
		"bind": jsParams,
	}
	b.database.Call("exec", opts)

	opts = map[string]any{
		"sql":         `SELECT changes() as changes, last_insert_rowid() as lastId;`,
		"returnValue": "resultRows",
	}
	result := b.database.Call("exec", opts)

	// Extract rowsAffected and lastInsertId
	rowsAffected := 0
	lastInsertId := 0

	if !result.IsUndefined() && result.Length() > 0 {
		res := result.Index(0)
		if res.Length() == 2 {
			rowsAffected = res.Index(0).Int()
			lastInsertId = res.Index(1).Int()
		}
	}
	return rowsAffected, lastInsertId, nil
}

// Query executes a query and returns results
func (b *APIOO) Query(sql string, params []any) ([]string, [][]any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.database.IsUndefined() {
		return nil, nil, fmt.Errorf("missing database")
	}

	// Convert params to JavaScript array
	jsParams := js.Global().Get("Array").New()
	for i, param := range params {
		jsParams.SetIndex(i, toJSValue(param))
	}

	opts := map[string]any{
		"sql":         sql,
		"bind":        jsParams,
		"returnValue": "resultRows",
	}
	rowsJS := b.database.Call("exec", opts)

	var rows [][]any
	columnCount := 0
	if !rowsJS.IsUndefined() && rowsJS.Length() > 0 {
		rows = make([][]any, rowsJS.Length())
		for i := 0; i < rowsJS.Length(); i++ {
			r := rowsJS.Index(i)
			if r.Length() > 0 {
				columnCount = r.Length()
				row := make([]any, r.Length())
				for j := 0; j < r.Length(); j++ {
					val := r.Index(j)
					if val.IsNull() {
						row[j] = nil
					} else if val.Type() == js.TypeNumber {
						num := val.Float()
						// If it's a whole number, return as int64 to match SQLite integer types
						if num == float64(int64(num)) {
							row[j] = int64(num)
						} else {
							row[j] = num
						}
					} else if val.Type() == js.TypeString {
						row[j] = val.String()
					} else if val.Type() == js.TypeBoolean {
						row[j] = val.Bool()
					} else {
						row[j] = val.String()
					}
				}
				rows[i] = row
			}
		}
	}

	// dummy columns
	columns := make([]string, columnCount)

	return columns, rows, nil
}

// Begin starts a transaction
func (b *APIOO) Begin() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	beginMethod := b.sqlite.Get("begin")
	if beginMethod.IsUndefined() {
		return fmt.Errorf("sqliteBridge.begin method not found")
	}

	_, err := callAsync(beginMethod)
	return err
}

// Commit commits a transaction
func (b *APIOO) Commit() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	commitMethod := b.sqlite.Get("commit")
	if commitMethod.IsUndefined() {
		return fmt.Errorf("sqliteBridge.commit method not found")
	}

	_, err := callAsync(commitMethod)
	return err
}

// Rollback rolls back a transaction
func (b *APIOO) Rollback() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	rollbackMethod := b.sqlite.Get("rollback")
	if rollbackMethod.IsUndefined() {
		return fmt.Errorf("sqliteBridge.rollback method not found")
	}

	_, err := callAsync(rollbackMethod)
	return err
}

// Close closes the database connection
func (b *APIOO) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	closeMethod := b.sqlite.Get("close")
	if closeMethod.IsUndefined() {
		return fmt.Errorf("sqliteBridge.close method not found")
	}

	_, err := callAsync(closeMethod)
	return err
}

// Dump exports the database as SQL statements
func (b *APIOO) Dump() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	dumpMethod := b.sqlite.Get("dump")
	if dumpMethod.IsUndefined() {
		return "", fmt.Errorf("sqliteBridge.dump method not found")
	}

	result, err := callAsync(dumpMethod)
	if err != nil {
		return "", err
	}

	// Extract dump from result
	if !result.IsUndefined() && !result.IsNull() {
		dump := result.Get("dump")
		if dump.Truthy() {
			return dump.String(), nil
		}
	}

	return "", fmt.Errorf("no dump data received")
}

// Load imports SQL statements to restore the database
func (b *APIOO) Load(dump string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	loadMethod := b.sqlite.Get("load")
	if loadMethod.IsUndefined() {
		return fmt.Errorf("sqliteBridge.load method not found")
	}

	_, err := callAsync(loadMethod, dump)
	return err
}
