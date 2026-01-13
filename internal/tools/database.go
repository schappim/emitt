package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// DatabaseTool executes database queries
type DatabaseTool struct {
	db            *sql.DB
	allowedTables []string
	readOnly      bool
}

// NewDatabaseTool creates a new database tool
func NewDatabaseTool(db *sql.DB, allowedTables []string, readOnly bool) *DatabaseTool {
	return &DatabaseTool{
		db:            db,
		allowedTables: allowedTables,
		readOnly:      readOnly,
	}
}

func (t *DatabaseTool) Name() string {
	return "database_query"
}

func (t *DatabaseTool) Description() string {
	desc := "Executes SQL queries against the database. "
	if t.readOnly {
		desc += "Only SELECT queries are allowed. "
	} else {
		desc += "Supports SELECT, INSERT, UPDATE, and DELETE queries. "
	}
	if len(t.allowedTables) > 0 {
		desc += fmt.Sprintf("Allowed tables: %s", strings.Join(t.allowedTables, ", "))
	}
	return desc
}

func (t *DatabaseTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "SQL query to execute",
			},
			"params": map[string]interface{}{
				"type":        "array",
				"description": "Query parameters (for parameterized queries)",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
		},
		"required": []string{"query"},
	}
}

// DatabaseArgs represents the arguments for the database tool
type DatabaseArgs struct {
	Query  string   `json:"query"`
	Params []string `json:"params"`
}

// QueryResult represents the result of a query
type QueryResult struct {
	Columns      []string                 `json:"columns,omitempty"`
	Rows         []map[string]interface{} `json:"rows,omitempty"`
	RowsAffected int64                    `json:"rows_affected,omitempty"`
	LastInsertID int64                    `json:"last_insert_id,omitempty"`
}

func (t *DatabaseTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params DatabaseArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return NewErrorResult(fmt.Errorf("invalid arguments: %w", err))
	}

	if params.Query == "" {
		return NewErrorResult(fmt.Errorf("query is required"))
	}

	// Validate query type
	queryUpper := strings.ToUpper(strings.TrimSpace(params.Query))
	isSelect := strings.HasPrefix(queryUpper, "SELECT")

	if t.readOnly && !isSelect {
		return NewErrorResult(fmt.Errorf("only SELECT queries are allowed in read-only mode"))
	}

	// Check for dangerous operations
	if strings.Contains(queryUpper, "DROP ") ||
		strings.Contains(queryUpper, "TRUNCATE ") ||
		strings.Contains(queryUpper, "ALTER ") ||
		strings.Contains(queryUpper, "CREATE ") {
		return NewErrorResult(fmt.Errorf("DDL operations are not allowed"))
	}

	// Convert params to interface slice
	queryParams := make([]interface{}, len(params.Params))
	for i, p := range params.Params {
		queryParams[i] = p
	}

	if isSelect {
		return t.executeSelect(ctx, params.Query, queryParams)
	}
	return t.executeModify(ctx, params.Query, queryParams)
}

func (t *DatabaseTool) executeSelect(ctx context.Context, query string, params []interface{}) (json.RawMessage, error) {
	rows, err := t.db.QueryContext(ctx, query, params...)
	if err != nil {
		return NewErrorResult(fmt.Errorf("query failed: %w", err))
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return NewErrorResult(fmt.Errorf("failed to get columns: %w", err))
	}

	result := QueryResult{
		Columns: columns,
		Rows:    make([]map[string]interface{}, 0),
	}

	// Limit rows to prevent memory issues
	maxRows := 1000
	rowCount := 0

	for rows.Next() && rowCount < maxRows {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return NewErrorResult(fmt.Errorf("failed to scan row: %w", err))
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for readability
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result.Rows = append(result.Rows, row)
		rowCount++
	}

	return NewSuccessResult(result)
}

func (t *DatabaseTool) executeModify(ctx context.Context, query string, params []interface{}) (json.RawMessage, error) {
	res, err := t.db.ExecContext(ctx, query, params...)
	if err != nil {
		return NewErrorResult(fmt.Errorf("query failed: %w", err))
	}

	result := QueryResult{}

	if affected, err := res.RowsAffected(); err == nil {
		result.RowsAffected = affected
	}

	if lastID, err := res.LastInsertId(); err == nil {
		result.LastInsertID = lastID
	}

	return NewSuccessResult(result)
}

// SchemaInfo provides database schema information
type SchemaInfo struct {
	Tables []TableInfo `json:"tables"`
}

// TableInfo describes a database table
type TableInfo struct {
	Name    string       `json:"name"`
	Columns []ColumnInfo `json:"columns"`
}

// ColumnInfo describes a table column
type ColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	PK       bool   `json:"pk"`
}

// GetSchema returns the database schema information
func (t *DatabaseTool) GetSchema(ctx context.Context) (*SchemaInfo, error) {
	rows, err := t.db.QueryContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schema := &SchemaInfo{Tables: make([]TableInfo, 0)}

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}

		// Check if table is allowed
		if len(t.allowedTables) > 0 {
			allowed := false
			for _, at := range t.allowedTables {
				if at == tableName {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		tableInfo := TableInfo{Name: tableName, Columns: make([]ColumnInfo, 0)}

		// Get column info
		colRows, err := t.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			continue
		}

		for colRows.Next() {
			var cid int
			var name, colType string
			var notNull, pk int
			var dfltValue interface{}
			if err := colRows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
				continue
			}
			tableInfo.Columns = append(tableInfo.Columns, ColumnInfo{
				Name:     name,
				Type:     colType,
				Nullable: notNull == 0,
				PK:       pk == 1,
			})
		}
		colRows.Close()

		schema.Tables = append(schema.Tables, tableInfo)
	}

	return schema, nil
}
