package storage

import (
	"encoding/json"
	"time"
)

// Email represents a stored email record
type Email struct {
	ID          int64           `json:"id"`
	MessageID   string          `json:"message_id"`
	From        string          `json:"from"`
	To          []string        `json:"to"`
	Cc          []string        `json:"cc"`
	Subject     string          `json:"subject"`
	TextBody    string          `json:"text_body"`
	HTMLBody    string          `json:"html_body"`
	RawMessage  []byte          `json:"raw_message"`
	Headers     json.RawMessage `json:"headers"`
	Attachments json.RawMessage `json:"attachments"`
	ReceivedAt  time.Time       `json:"received_at"`
	ProcessedAt *time.Time      `json:"processed_at"`
	MailboxName string          `json:"mailbox_name"`
	Status      EmailStatus     `json:"status"`
}

// EmailStatus represents the processing status of an email
type EmailStatus string

const (
	EmailStatusPending    EmailStatus = "pending"
	EmailStatusProcessing EmailStatus = "processing"
	EmailStatusCompleted  EmailStatus = "completed"
	EmailStatusFailed     EmailStatus = "failed"
)

// ProcessingLog represents a log entry for email processing
type ProcessingLog struct {
	ID        int64     `json:"id"`
	EmailID   int64     `json:"email_id"`
	Step      string    `json:"step"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	Error     string    `json:"error"`
	Duration  int64     `json:"duration_ms"`
	CreatedAt time.Time `json:"created_at"`
}

// ToolCall represents a record of a tool invocation
type ToolCall struct {
	ID           int64           `json:"id"`
	EmailID      int64           `json:"email_id"`
	ToolName     string          `json:"tool_name"`
	Arguments    json.RawMessage `json:"arguments"`
	Result       json.RawMessage `json:"result"`
	Error        string          `json:"error"`
	Duration     int64           `json:"duration_ms"`
	CalledAt     time.Time       `json:"called_at"`
}

// Attachment represents an email attachment metadata
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ContentID   string `json:"content_id"`
	Data        []byte `json:"-"` // Not stored in JSON, loaded separately
}

// EmailListFilter defines filter options for listing emails
type EmailListFilter struct {
	Status      *EmailStatus
	MailboxName *string
	FromDate    *time.Time
	ToDate      *time.Time
	Limit       int
	Offset      int
}

// EmailStats represents email processing statistics
type EmailStats struct {
	TotalEmails     int64 `json:"total_emails"`
	PendingEmails   int64 `json:"pending_emails"`
	ProcessedEmails int64 `json:"processed_emails"`
	FailedEmails    int64 `json:"failed_emails"`
}
