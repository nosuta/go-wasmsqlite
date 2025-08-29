// This worker uses SQLite's official Worker1 Promiser API which properly supports OPFS
import { sqlite3Worker1Promiser } from '@sqlite.org/sqlite-wasm';

// Message types that match our Go driver's expectations
type RequestMessage =
  | { id: number; type: 'init' }
  | { id: number; type: 'open'; file: string; vfs?: string }
  | { id: number; type: 'exec'; sql: string; params?: any[] }
  | { id: number; type: 'query'; sql: string; params?: any[] }
  | { id: number; type: 'begin' }
  | { id: number; type: 'commit' }
  | { id: number; type: 'rollback' }
  | { id: number; type: 'close' }
  | { id: number; type: 'dump' }
  | { id: number; type: 'load'; sql: string };

type ResponseMessage = {
  id: number;
  ok: boolean;
  error?: string;
  columns?: string[];
  rows?: any[][];
  rowsAffected?: number;
  lastInsertId?: number;
  vfsType?: string;
  dump?: string;
};

let promiser: any = null;
let dbId: string | null = null;
let vfsType = 'unknown';

function postSuccess(id: number, payload: Partial<ResponseMessage> = {}) {
  const message: ResponseMessage = { id, ok: true, ...payload };
  self.postMessage(message);
}

function postError(id: number, error: any) {
  const message: ResponseMessage = { 
    id, 
    ok: false, 
    error: String(error?.result?.message || error?.message || error) 
  };
  self.postMessage(message);
}

// Initialize the SQLite Worker1 Promiser
async function initPromiser() {
  return new Promise((resolve) => {
    const _promiser = sqlite3Worker1Promiser({
      onready: () => {
        console.log('SQLite Worker1 Promiser initialized');
        resolve(_promiser);
      },
      onerror: (error: any) => {
        console.error('SQLite Worker1 Promiser error:', error);
      }
    });
  });
}

self.onmessage = async (event: MessageEvent<RequestMessage>) => {
  const { id, type } = event.data;
  
  try {
    switch (type) {
      case 'init':
        // Initialize the promiser if not already done
        if (!promiser) {
          promiser = await initPromiser();
          
          // Get SQLite version info
          const configResponse = await promiser('config-get', {});
          console.log('SQLite version:', configResponse.result.version.libVersion);
        }
        
        postSuccess(id);
        break;

      case 'open':
        if (!promiser) {
          throw new Error('SQLite not initialized. Call init first.');
        }
        
        let { file = 'mydb.sqlite3', vfs } = event.data;
        
        // Close existing database if open
        if (dbId) {
          await promiser('close', { dbId });
          dbId = null;
        }
        
        // Determine the filename and VFS to use
        let filename = file;
        
        // Try to use OPFS if available and no specific VFS requested
        if (!vfs || vfs === 'opfs' || vfs === 'opfs-sahpool') {
          // Check if OPFS is available
          const configResponse = await promiser('config-get', {});
          const hasOpfs = configResponse.result.vfsList.includes('opfs');
          
          if (hasOpfs) {
            // Use OPFS with the specified filename
            filename = `file:${file}?vfs=opfs`;
            vfsType = 'opfs';
            console.log('Using OPFS for persistence');
          } else {
            // Fall back to in-memory
            filename = ':memory:';
            vfsType = 'memory';
            console.warn('OPFS not available, using in-memory database');
          }
        } else if (file === ':memory:' || vfs === 'memory') {
          filename = ':memory:';
          vfsType = 'memory';
        } else {
          // Use specified VFS
          filename = `file:${file}?vfs=${vfs}`;
          vfsType = vfs;
        }
        
        // Open the database
        const openResponse = await promiser('open', {
          filename
        });
        
        dbId = openResponse.dbId;
        console.log(`Database opened: ${filename} (dbId: ${dbId})`);
        
        // Create tables if needed
        const initSql = `
          CREATE TABLE IF NOT EXISTS users (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            username TEXT NOT NULL UNIQUE,
            email TEXT NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
          );
          
          CREATE TABLE IF NOT EXISTS posts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            user_id INTEGER NOT NULL,
            title TEXT NOT NULL,
            content TEXT,
            published BOOLEAN DEFAULT FALSE,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (user_id) REFERENCES users(id)
          );
        `;
        
        await promiser('exec', {
          dbId,
          sql: initSql
        });
        
        postSuccess(id, { vfsType });
        break;

      case 'exec':
        if (!dbId) {
          throw new Error('Database not opened');
        }
        
        const { sql, params = [] } = event.data;
        
        // Execute the SQL
        const execResult = await promiser('exec', {
          dbId,
          sql,
          bind: params,
          returnValue: 'resultRows'
        });
        
        // Get rows affected and last insert ID
        // Note: Worker1 API doesn't directly expose these, so we'll query them
        const changesResult = await promiser('exec', {
          dbId,
          sql: 'SELECT changes() as changes, last_insert_rowid() as lastId',
          returnValue: 'resultRows'
        });
        
        const rowsAffected = changesResult?.result?.resultRows?.[0]?.[0] || 0;
        const lastInsertId = changesResult?.result?.resultRows?.[0]?.[1] || 0;
        
        postSuccess(id, {
          rowsAffected,
          lastInsertId
        });
        break;

      case 'query':
        if (!dbId) {
          throw new Error('Database not opened');
        }
        
        const { sql: querySql, params: queryParams = [] } = event.data;
        
        // Execute query and get results
        const queryResult = await promiser('exec', {
          dbId,
          sql: querySql,
          bind: queryParams,
          returnValue: 'resultRows',
          rowMode: 'array'
        });
        
        let columns: string[] = [];
        let rows: any[][] = [];
        
        if (queryResult?.result?.resultRows) {
          rows = queryResult.result.resultRows;
          
          // Try to get column names
          // First, try to extract from the SQL
          const selectMatch = querySql.match(/SELECT\s+(.+?)\s+FROM/i);
          if (selectMatch) {
            const selectClause = selectMatch[1].trim();
            if (selectClause === '*') {
              // For SELECT *, determine columns based on table
              if (querySql.toLowerCase().includes('from users')) {
                columns = ['id', 'username', 'email', 'created_at'];
              } else if (querySql.toLowerCase().includes('from posts')) {
                columns = ['id', 'user_id', 'title', 'content', 'published', 'created_at'];
              } else if (rows[0]) {
                columns = rows[0].map((_, i) => `column${i}`);
              }
            } else {
              // Parse column names
              columns = selectClause.split(',').map(c => {
                const parts = c.trim().split(/\s+as\s+/i);
                if (parts.length > 1) {
                  return parts[1].trim();
                }
                const dotParts = parts[0].split('.');
                return dotParts[dotParts.length - 1].trim();
              });
            }
          } else if (rows[0]) {
            // Fallback to generic column names
            columns = rows[0].map((_, i) => `column${i}`);
          }
        }
        
        postSuccess(id, { columns, rows });
        break;

      case 'begin':
        if (!dbId) {
          throw new Error('Database not opened');
        }
        
        await promiser('exec', {
          dbId,
          sql: 'BEGIN IMMEDIATE'
        });
        
        postSuccess(id);
        break;

      case 'commit':
        if (!dbId) {
          throw new Error('Database not opened');
        }
        
        await promiser('exec', {
          dbId,
          sql: 'COMMIT'
        });
        
        postSuccess(id);
        break;

      case 'rollback':
        if (!dbId) {
          throw new Error('Database not opened');
        }
        
        await promiser('exec', {
          dbId,
          sql: 'ROLLBACK'
        });
        
        postSuccess(id);
        break;

      case 'close':
        if (dbId) {
          await promiser('close', { dbId });
          dbId = null;
        }
        postSuccess(id);
        break;

      case 'dump':
        if (!dbId) {
          throw new Error('Database not opened');
        }
        
        // Export the database
        const exportResult = await promiser('export', {
          dbId
        });
        
        // Convert Uint8Array to SQL dump
        // Note: This is a binary export, not SQL text
        // For SQL text dump, we need to query the schema and data
        let dumpSql = '';
        
        // Get schema
        const schemaResult = await promiser('exec', {
          dbId,
          sql: "SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY tbl_name, type DESC, name",
          returnValue: 'resultRows'
        });
        
        if (schemaResult?.result?.resultRows) {
          for (const [sql] of schemaResult.result.resultRows) {
            dumpSql += sql + ';\n';
          }
        }
        
        // Get data for each table
        const tablesResult = await promiser('exec', {
          dbId,
          sql: "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'",
          returnValue: 'resultRows'
        });
        
        if (tablesResult?.result?.resultRows) {
          for (const [tableName] of tablesResult.result.resultRows) {
            const dataResult = await promiser('exec', {
              dbId,
              sql: `SELECT * FROM ${tableName}`,
              returnValue: 'resultRows'
            });
            
            if (dataResult?.result?.resultRows?.length > 0) {
              // Get column names
              const pragmaResult = await promiser('exec', {
                dbId,
                sql: `PRAGMA table_info(${tableName})`,
                returnValue: 'resultRows'
              });
              
              const columnNames = pragmaResult?.result?.resultRows?.map((row: any[]) => row[1]) || [];
              
              for (const row of dataResult.result.resultRows) {
                const values = row.map((v: any) => {
                  if (v === null) return 'NULL';
                  if (typeof v === 'string') return `'${v.replace(/'/g, "''")}'`;
                  return String(v);
                }).join(', ');
                dumpSql += `INSERT INTO ${tableName} (${columnNames.join(', ')}) VALUES (${values});\n`;
              }
            }
          }
        }
        
        postSuccess(id, { dump: dumpSql });
        break;

      case 'load':
        if (!dbId) {
          throw new Error('Database not opened');
        }
        
        const { sql: loadSql } = event.data;
        
        // Drop existing tables first
        const dropTablesResult = await promiser('exec', {
          dbId,
          sql: "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'",
          returnValue: 'resultRows'
        });
        
        if (dropTablesResult?.result?.resultRows) {
          for (const [tableName] of dropTablesResult.result.resultRows) {
            await promiser('exec', {
              dbId,
              sql: `DROP TABLE IF EXISTS ${tableName}`
            });
          }
        }
        
        // Load the SQL dump
        await promiser('exec', {
          dbId,
          sql: loadSql
        });
        
        postSuccess(id);
        break;

      default:
        throw new Error(`Unknown message type: ${(event.data as any).type}`);
    }
  } catch (error) {
    postError(id, error);
  }
};