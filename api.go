//go:build js && wasm

package wasmsqlite

// API adapts the JavaScript SQLite APIs to work with our Go driver
type API interface {
	Init() error
	Open(filename, vfs string) (string, error)
	Exec(sql string, params []any) (int, int, error)
	Query(sql string, params []any) ([]string, [][]any, error)
	Begin() error
	Commit() error
	Rollback() error
	Close() error
}
