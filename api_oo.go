//go:build js && wasm

package wasmsqlite

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"syscall/js"
)

// APIOO adapts the JavaScript SQLite OO API to work with our Go driver.
// It uses sqlite3.oo1.OpfsDb (or sqlite3.oo1.DB) directly inside the Go
// WASM Worker, so SQLite and Go share the same Worker thread and there
// is no nested Worker bridge.
type APIOO struct {
	sqlite   js.Value
	database js.Value
	mu       sync.Mutex
}

// NewAPIOO creates a new OO API.
func NewAPIOO() (*APIOO, error) {
	return &APIOO{}, nil
}

// Init initializes OO API.
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

// Open opens a database.
func (b *APIOO) Open(path, vfs string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.Init(); err != nil {
		return "", err
	}

	if !b.database.IsNull() && !b.database.IsUndefined() {
		return "opfs", nil
	}

	opfsDb := b.sqlite.Get("oo1").Get("OpfsDb")
	if opfsDb.IsUndefined() {
		return "", fmt.Errorf("OPFS is not supported")
	}
	fmt.Printf("🔍 sqlite3 version: %s\n", b.sqlite.Get("version").Get("libVersion").String())

	db := opfsDb.New(path, "c")
	if db.IsNull() || db.IsUndefined() {
		return "", fmt.Errorf("failed to create database")
	}
	b.database = db
	return "opfs", nil
}

// Exec executes a SQL statement.
func (b *APIOO) Exec(sqlStr string, params []any) (rowsAffected int, lastInsertId int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sqlite exec failed: %v", r)
		}
	}()
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.database.IsUndefined() {
		return 0, 0, fmt.Errorf("missing database")
	}

	beforeChanges := b.database.Call("changes", true).Int()

	stmt := b.database.Call("prepare", sqlStr)
	defer stmt.Call("finalize")

	jsParams, err := b.normalizeParams(stmt, params)
	if err != nil {
		return 0, 0, err
	}
	if !jsParams.IsUndefined() {
		stmt.Call("bind", jsParams)
	}
	for stmt.Call("step").Bool() {
	}

	rowsAffected = b.database.Call("changes", true).Int() - beforeChanges
	lastInsertId = b.lastInsertRowID()
	return rowsAffected, lastInsertId, err
}

// Query executes a query and returns results.
func (b *APIOO) Query(sqlStr string, params []any) (columns []string, rows [][]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sqlite query failed: %v", r)
		}
	}()
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.database.IsUndefined() {
		return nil, nil, fmt.Errorf("missing database")
	}

	stmt := b.database.Call("prepare", sqlStr)
	defer stmt.Call("finalize")

	jsParams, err := b.normalizeParams(stmt, params)
	if err != nil {
		return nil, nil, err
	}
	if !jsParams.IsUndefined() {
		stmt.Call("bind", jsParams)
	}

	columns = b.readColumnNames(stmt)

	for stmt.Call("step").Bool() {
		rowJS := stmt.Call("get", js.Global().Get("Array").New())
		row := make([]any, rowJS.Length())
		for j := 0; j < rowJS.Length(); j++ {
			row[j] = normalizeResultValue(rowJS.Index(j))
		}
		rows = append(rows, row)
	}

	return columns, rows, err
}

// Begin starts a transaction.
func (b *APIOO) Begin() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sqlite begin failed: %v", r)
		}
	}()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.database.Call("exec", map[string]any{"sql": "BEGIN IMMEDIATE;"})
	return nil
}

// Commit commits a transaction.
func (b *APIOO) Commit() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sqlite commit failed: %v", r)
		}
	}()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.database.Call("exec", map[string]any{"sql": "COMMIT;"})
	return nil
}

// Rollback rolls back a transaction.
func (b *APIOO) Rollback() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sqlite rollback failed: %v", r)
		}
	}()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.database.Call("exec", map[string]any{"sql": "ROLLBACK;"})
	return nil
}

// Close closes the database connection.
func (b *APIOO) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sqlite close failed: %v", r)
		}
	}()
	b.mu.Lock()
	defer b.mu.Unlock()

	b.database.Call("close")
	return nil
}

// Dump exports the database as SQL statements.
func (b *APIOO) Dump() (string, error) {
	return "", fmt.Errorf("unimplemented")
}

// Load imports SQL statements to restore the database.
func (b *APIOO) Load(dump string) error {
	return fmt.Errorf("unimplemented")
}

// --- helpers ported from upstream sqlite-worker.js ---

var numericParamRE = regexp.MustCompile(`^[:@$?]([1-9][0-9]*)$`)

func numericParamIndex(name string) int {
	m := numericParamRE.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

func (b *APIOO) normalizeParams(stmt js.Value, params []any) (js.Value, error) {
	if len(params) == 0 {
		return js.Undefined(), nil
	}

	// remap positional params: each SQLite bind slot i (1..parameterCount)
	// receives params[numericIndex($N)-1] when named $N, otherwise
	// params[i-1]. Without this, SQLite's array-bind ties params to slots
	// by textual order-of-first-appearance, mis-binding $N that appear out
	// of order (e.g. UPDATE ... SET col=$3 WHERE k=$1).
	paramCount := 0
	if pc := stmt.Get("parameterCount"); !pc.IsUndefined() {
		paramCount = pc.Int()
	} else {
		// Methods/getters aren't canonical in oo1; fall back to integer read.
		paramCount = stmt.Call("getParameterCount").Int()
	}

	normalized := make([]any, 0, paramCount)
	used := make(map[int]bool, len(params))

	for i := 1; i <= paramCount; i++ {
		bindName := ""
		if name := stmt.Call("getParamName", i); !name.IsUndefined() && !name.IsNull() {
			bindName = name.String()
		}

		var idx int
		if n := numericParamIndex(bindName); n > 0 {
			idx = n - 1
		} else {
			idx = i - 1
		}

		if idx < 0 || idx >= len(params) {
			return js.Undefined(), fmt.Errorf("missing SQL parameter for %s", bindName)
		}
		normalized = append(normalized, params[idx])
		used[idx] = true
	}

	for i := range params {
		if !used[i] {
			return js.Undefined(), fmt.Errorf("unused SQL parameter at index %d", i+1)
		}
	}

	// pack into a JS array
	arr := js.Global().Get("Array").New(len(normalized))
	for i, v := range normalized {
		arr.SetIndex(i, toJSValue(v))
	}
	return arr, nil
}

func (b *APIOO) readColumnNames(stmt js.Value) []string {
	colCount := 0
	if cc := stmt.Get("columnCount"); !cc.IsUndefined() {
		colCount = cc.Int()
	} else {
		colCount = stmt.Call("getColumnCount").Int()
	}
	if colCount <= 0 {
		return []string{}
	}
	colArr := stmt.Call("getColumnNames", js.Global().Get("Array").New())
	out := make([]string, colArr.Length())
	for i := 0; i < colArr.Length(); i++ {
		out[i] = colArr.Index(i).String()
	}
	return out
}

func (b *APIOO) lastInsertRowID() int {
	stmt := b.database.Call("prepare", "SELECT last_insert_rowid() AS id;")
	defer stmt.Call("finalize")
	if !stmt.Call("step").Bool() {
		return 0
	}
	v := stmt.Call("get", 0)
	if v.Type() == js.TypeNumber {
		return v.Int()
	}
	return 0
}

func normalizeResultValue(val js.Value) any {
	if val.IsNull() {
		return nil
	}
	switch val.Type() {
	case js.TypeNumber:
		num := val.Float()
		if num == float64(int64(num)) {
			return int64(num)
		}
		return num
	case js.TypeString:
		return val.String()
	case js.TypeBoolean:
		return val.Bool()
	case js.TypeObject:
		// Uint8Array -> []byte (BLOB)
		c := val.Get("constructor")
		if !c.IsUndefined() && c.Get("name").String() == "Uint8Array" {
			out := make([]byte, val.Length())
			js.CopyBytesToGo(out, val)
			return out
		}
		return val.String()
	default:
		return val.String()
	}
}