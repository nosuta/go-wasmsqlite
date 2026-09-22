# go-wasmsqlite

A minimal WebAssembly SQLite driver for Go's `database/sql`, running SQLite
inside the same Web Worker as the Go WASM binary with **OPFS** persistence.

This is the slim driver used by [godash](https://github.com/nosuta/godash)'s web
target. It implements a single route — the SQLite **OO direct** API
(`sqlite3.oo1.OpfsDb` / `sqlite3.oo1.DB`) — with no nested Worker bridge,
no embedded assets, and no migration/dump helpers.

## Usage

The driver registers itself under the name `wasmsqlite` when imported:

```go
import (
	"database/sql"

	_ "github.com/nosuta/go-wasmsqlite"
)

db, err := sql.Open("wasmsqlite", "file=/app.db")
```

### DSN

| key    | default    | meaning                        |
|--------|------------|--------------------------------|
| `file` | `/app.db`  | database path in the OPFS VFS  |
| `vfs`  | `opfs`     | virtual file system to use     |

## Runtime requirements

- Go 1.24+ with `GOOS=js GOARCH=wasm`.
- The page must be cross-origin isolated (COOP/COEP) for OPFS.
- `sqlite3.js` (which defines `sqlite3InitModule`) and `sqlite3.wasm` must be
  served next to the worker that runs the WASM binary. godash downloads the
  official SQLite WASM distribution into `web/` automatically.

## Scope

Included: `database/sql` driver with `Exec`/`Query`/`Prepare`, transactions
(`BEGIN IMMEDIATE`/`COMMIT`/`ROLLBACK`), BLOB round-tripping, and OPFS
persistence.

Intentionally excluded: golang-migrate integration, asset embedding, VFS
detection, and database dump/load. The published module previously carried
these (and debug logging); they are removed because the only consumer is the
godash web SQLite wrapper, which uses the driver directly.
