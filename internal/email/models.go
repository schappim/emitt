package email

import (
	"time"
)

// Address represents an email address with optional name
type Address struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

// String returns the formatted address
func (a Address) String() string {
	if a.Name != "" {
		return a.Name + " <" + a.Address + ">"
	}
	return a.Address
}

// Attachment represents an email attachment
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id,omitempty"`
	Size        int64  `json:"size"`
	Data        []byte `json:"-"`
}

// InboundEmail represents a parsed inbound email
type InboundEmail struct {
	MessageID   string            `json:"message_id"`
	From        Address           `json:"from"`
	To          []Address         `json:"to"`
	Cc          []Address         `json:"cc"`
	Bcc         []Address         `json:"bcc"`
	ReplyTo     *Address          `json:"reply_to,omitempty"`
	Subject     string            `json:"subject"`
	Date        time.Time         `json:"date"`
	TextBody    string            `json:"text_body"`
	HTMLBody    string            `json:"html_body"`
	Headers     map[string]string `json:"headers"`
	Attachments []Attachment      `json:"attachments"`
	RawMessage  []byte            `json:"-"`
	ReceivedAt  time.Time         `json:"received_at"`
}

// GetToAddresses returns just the email addresses from To
func (e *InboundEmail) GetToAddresses() []string {
	addrs := make([]string, len(e.To))
	for i, a := range e.To {
		addrs[i] = a.Address
	}
	return addrs
}

// GetCcAddresses returns just the email addresses from Cc
func (e *InboundEmail) GetCcAddresses() []string {
	addrs := make([]string, len(e.Cc))
	for i, a := range e.Cc {
		addrs[i] = a.Address
	}
	return addrs
}

// Body returns the best available body (text preferred for LLM)
func (e *InboundEmail) Body() string {
	if e.TextBody != "" {
		return e.TextBody
	}
	return e.HTMLBody
}

// HasAttachments returns true if the email has attachments
func (e *InboundEmail) HasAttachments() bool {
	return len(e.Attachments) > 0
}

// OutboundEmail represents an email to be sent
type OutboundEmail struct {
	From        Address      `json:"from"`
	To          []Address    `json:"to"`
	Cc          []Address    `json:"cc"`
	Bcc         []Address    `json:"bcc"`
	ReplyTo     *Address     `json:"reply_to,omitempty"`
	Subject     string       `json:"subject"`
	TextBody    string       `json:"text_body"`
	HTMLBody    string       `json:"html_body"`
	Attachments []Attachment `json:"attachments"`
	InReplyTo   string       `json:"in_reply_to,omitempty"`
	References  []string     `json:"references,omitempty"`
}

// EmailContext provides email information to the LLM
type EmailContext struct {
	From        string            `json:"from"`
	To          []string          `json:"to"`
	Cc          []string          `json:"cc"`
	Subject     string            `json:"subject"`
	Body        string            `json:"body"`
	Date        string            `json:"date"`
	HasHTML     bool              `json:"has_html"`
	Attachments []AttachmentInfo  `json:"attachments,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// AttachmentInfo provides attachment metadata for LLM context
type AttachmentInfo struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// ToContext converts an InboundEmail to EmailContext for LLM
func (e *InboundEmail) ToContext() EmailContext {
	ctx := EmailContext{
		From:    e.From.String(),
		To:      e.GetToAddresses(),
		Cc:      e.GetCcAddresses(),
		Subject: e.Subject,
		Body:    e.Body(),
		Date:    e.Date.Format(time.RFC1123),
		HasHTML: e.HTMLBody != "",
		Headers: e.Headers,
	}

	for _, att := range e.Attachments {
		ctx.Attachments = append(ctx.Attachments, AttachmentInfo{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			Size:        att.Size,
		})
	}

	return ctx
}
