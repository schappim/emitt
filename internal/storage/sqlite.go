package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store provides database operations
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store with the given database path
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return store, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate runs database migrations
func (s *Store) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS emails (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id TEXT UNIQUE,
			from_addr TEXT NOT NULL,
			to_addrs TEXT NOT NULL,
			cc_addrs TEXT,
			subject TEXT,
			text_body TEXT,
			html_body TEXT,
			raw_message BLOB,
			headers TEXT,
			attachments TEXT,
			received_at DATETIME NOT NULL,
			processed_at DATETIME,
			mailbox_name TEXT,
			status TEXT NOT NULL DEFAULT 'pending'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_emails_status ON emails(status)`,
		`CREATE INDEX IF NOT EXISTS idx_emails_mailbox ON emails(mailbox_name)`,
		`CREATE INDEX IF NOT EXISTS idx_emails_received ON emails(received_at)`,

		`CREATE TABLE IF NOT EXISTS processing_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email_id INTEGER NOT NULL,
			step TEXT NOT NULL,
			input TEXT,
			output TEXT,
			error TEXT,
			duration_ms INTEGER,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (email_id) REFERENCES emails(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_email ON processing_logs(email_id)`,

		`CREATE TABLE IF NOT EXISTS tool_calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email_id INTEGER NOT NULL,
			tool_name TEXT NOT NULL,
			arguments TEXT,
			result TEXT,
			error TEXT,
			duration_ms INTEGER,
			called_at DATETIME NOT NULL,
			FOREIGN KEY (email_id) REFERENCES emails(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_email ON tool_calls(email_id)`,

		`CREATE TABLE IF NOT EXISTS attachments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email_id INTEGER NOT NULL,
			filename TEXT NOT NULL,
			content_type TEXT,
			size INTEGER,
			content_id TEXT,
			data BLOB,
			FOREIGN KEY (email_id) REFERENCES emails(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_email ON attachments(email_id)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// SaveEmail stores a new email record
func (s *Store) SaveEmail(ctx context.Context, email *Email) error {
	toJSON, _ := json.Marshal(email.To)
	ccJSON, _ := json.Marshal(email.Cc)

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO emails (
			message_id, from_addr, to_addrs, cc_addrs, subject,
			text_body, html_body, raw_message, headers, attachments,
			received_at, processed_at, mailbox_name, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		email.MessageID, email.From, string(toJSON), string(ccJSON),
		email.Subject, email.TextBody, email.HTMLBody, email.RawMessage,
		string(email.Headers), string(email.Attachments),
		email.ReceivedAt, email.ProcessedAt, email.MailboxName, email.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to save email: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	email.ID = id

	return nil
}

// GetEmail retrieves an email by ID
func (s *Store) GetEmail(ctx context.Context, id int64) (*Email, error) {
	var email Email
	var toJSON, ccJSON string
	var processedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, message_id, from_addr, to_addrs, cc_addrs, subject,
			   text_body, html_body, raw_message, headers, attachments,
			   received_at, processed_at, mailbox_name, status
		FROM emails WHERE id = ?
	`, id).Scan(
		&email.ID, &email.MessageID, &email.From, &toJSON, &ccJSON,
		&email.Subject, &email.TextBody, &email.HTMLBody, &email.RawMessage,
		&email.Headers, &email.Attachments,
		&email.ReceivedAt, &processedAt, &email.MailboxName, &email.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get email: %w", err)
	}

	json.Unmarshal([]byte(toJSON), &email.To)
	json.Unmarshal([]byte(ccJSON), &email.Cc)
	if processedAt.Valid {
		email.ProcessedAt = &processedAt.Time
	}

	return &email, nil
}

// UpdateEmailStatus updates the status of an email
func (s *Store) UpdateEmailStatus(ctx context.Context, id int64, status EmailStatus) error {
	var processedAt *time.Time
	if status == EmailStatusCompleted || status == EmailStatusFailed {
		now := time.Now()
		processedAt = &now
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE emails SET status = ?, processed_at = ? WHERE id = ?
	`, status, processedAt, id)
	if err != nil {
		return fmt.Errorf("failed to update email status: %w", err)
	}

	return nil
}

// ListEmails returns emails matching the filter criteria
func (s *Store) ListEmails(ctx context.Context, filter EmailListFilter) ([]*Email, error) {
	var conditions []string
	var args []interface{}

	if filter.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *filter.Status)
	}
	if filter.MailboxName != nil {
		conditions = append(conditions, "mailbox_name = ?")
		args = append(args, *filter.MailboxName)
	}
	if filter.FromDate != nil {
		conditions = append(conditions, "received_at >= ?")
		args = append(args, *filter.FromDate)
	}
	if filter.ToDate != nil {
		conditions = append(conditions, "received_at <= ?")
		args = append(args, *filter.ToDate)
	}

	query := `
		SELECT id, message_id, from_addr, to_addrs, cc_addrs, subject,
			   text_body, html_body, headers, attachments,
			   received_at, processed_at, mailbox_name, status
		FROM emails
	`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY received_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list emails: %w", err)
	}
	defer rows.Close()

	var emails []*Email
	for rows.Next() {
		var email Email
		var toJSON, ccJSON string
		var processedAt sql.NullTime

		if err := rows.Scan(
			&email.ID, &email.MessageID, &email.From, &toJSON, &ccJSON,
			&email.Subject, &email.TextBody, &email.HTMLBody,
			&email.Headers, &email.Attachments,
			&email.ReceivedAt, &processedAt, &email.MailboxName, &email.Status,
		); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}

		json.Unmarshal([]byte(toJSON), &email.To)
		json.Unmarshal([]byte(ccJSON), &email.Cc)
		if processedAt.Valid {
			email.ProcessedAt = &processedAt.Time
		}

		emails = append(emails, &email)
	}

	return emails, nil
}

// GetPendingEmails returns emails with pending status
func (s *Store) GetPendingEmails(ctx context.Context, limit int) ([]*Email, error) {
	status := EmailStatusPending
	return s.ListEmails(ctx, EmailListFilter{
		Status: &status,
		Limit:  limit,
	})
}

// SaveProcessingLog stores a processing log entry
func (s *Store) SaveProcessingLog(ctx context.Context, log *ProcessingLog) error {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO processing_logs (email_id, step, input, output, error, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, log.EmailID, log.Step, log.Input, log.Output, log.Error, log.Duration, log.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save processing log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	log.ID = id

	return nil
}

// SaveToolCall stores a tool call record
func (s *Store) SaveToolCall(ctx context.Context, call *ToolCall) error {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_calls (email_id, tool_name, arguments, result, error, duration_ms, called_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, call.EmailID, call.ToolName, string(call.Arguments), string(call.Result), call.Error, call.Duration, call.CalledAt)
	if err != nil {
		return fmt.Errorf("failed to save tool call: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	call.ID = id

	return nil
}

// GetProcessingLogs returns all processing logs for an email
func (s *Store) GetProcessingLogs(ctx context.Context, emailID int64) ([]*ProcessingLog, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, email_id, step, input, output, error, duration_ms, created_at
		FROM processing_logs WHERE email_id = ? ORDER BY created_at ASC
	`, emailID)
	if err != nil {
		return nil, fmt.Errorf("failed to get processing logs: %w", err)
	}
	defer rows.Close()

	var logs []*ProcessingLog
	for rows.Next() {
		var log ProcessingLog
		if err := rows.Scan(
			&log.ID, &log.EmailID, &log.Step, &log.Input, &log.Output,
			&log.Error, &log.Duration, &log.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan processing log: %w", err)
		}
		logs = append(logs, &log)
	}

	return logs, nil
}

// GetToolCalls returns all tool calls for an email
func (s *Store) GetToolCalls(ctx context.Context, emailID int64) ([]*ToolCall, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, email_id, tool_name, arguments, result, error, duration_ms, called_at
		FROM tool_calls WHERE email_id = ? ORDER BY called_at ASC
	`, emailID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool calls: %w", err)
	}
	defer rows.Close()

	var calls []*ToolCall
	for rows.Next() {
		var call ToolCall
		var args, result sql.NullString
		if err := rows.Scan(
			&call.ID, &call.EmailID, &call.ToolName, &args, &result,
			&call.Error, &call.Duration, &call.CalledAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan tool call: %w", err)
		}
		if args.Valid {
			call.Arguments = json.RawMessage(args.String)
		}
		if result.Valid {
			call.Result = json.RawMessage(result.String)
		}
		calls = append(calls, &call)
	}

	return calls, nil
}

// GetStats returns email processing statistics
func (s *Store) GetStats(ctx context.Context) (*EmailStats, error) {
	var stats EmailStats

	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM emails`).Scan(&stats.TotalEmails)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM emails WHERE status = 'pending'`).Scan(&stats.PendingEmails)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM emails WHERE status = 'completed'`).Scan(&stats.ProcessedEmails)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM emails WHERE status = 'failed'`).Scan(&stats.FailedEmails)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

// SaveAttachment stores an attachment
func (s *Store) SaveAttachment(ctx context.Context, emailID int64, att *Attachment) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO attachments (email_id, filename, content_type, size, content_id, data)
		VALUES (?, ?, ?, ?, ?, ?)
	`, emailID, att.Filename, att.ContentType, att.Size, att.ContentID, att.Data)
	if err != nil {
		return fmt.Errorf("failed to save attachment: %w", err)
	}
	return nil
}

// GetAttachments returns all attachments for an email
func (s *Store) GetAttachments(ctx context.Context, emailID int64) ([]*Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT filename, content_type, size, content_id, data
		FROM attachments WHERE email_id = ?
	`, emailID)
	if err != nil {
		return nil, fmt.Errorf("failed to get attachments: %w", err)
	}
	defer rows.Close()

	var attachments []*Attachment
	for rows.Next() {
		var att Attachment
		if err := rows.Scan(&att.Filename, &att.ContentType, &att.Size, &att.ContentID, &att.Data); err != nil {
			return nil, fmt.Errorf("failed to scan attachment: %w", err)
		}
		attachments = append(attachments, &att)
	}

	return attachments, nil
}

// DB returns the underlying database connection for custom queries
func (s *Store) DB() *sql.DB {
	return s.db
}
