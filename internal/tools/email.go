package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/emitt/emitt/internal/email"
)

// EmailSender is an interface for sending emails
type EmailSender interface {
	Send(ctx context.Context, email *email.OutboundEmail) error
}

// EmailTool handles email operations (reply, forward, send)
type EmailTool struct {
	sender       EmailSender
	fromAddress  string
	fromName     string
	currentEmail *email.InboundEmail
}

// NewEmailTool creates a new email tool
func NewEmailTool(sender EmailSender, fromAddress, fromName string) *EmailTool {
	return &EmailTool{
		sender:      sender,
		fromAddress: fromAddress,
		fromName:    fromName,
	}
}

// SetCurrentEmail sets the current email context for replies
func (t *EmailTool) SetCurrentEmail(e *email.InboundEmail) {
	t.currentEmail = e
}

func (t *EmailTool) Name() string {
	return "send_email"
}

func (t *EmailTool) Description() string {
	return "Sends emails. Can reply to the current email, forward it, or send a new email. Use 'reply' action to respond to sender, 'forward' to send to another address, or 'send' for a new email."
}

func (t *EmailTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"reply", "forward", "send"},
				"description": "The email action to perform",
			},
			"to": map[string]interface{}{
				"type":        "array",
				"description": "Recipient email addresses (required for forward and send)",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"cc": map[string]interface{}{
				"type":        "array",
				"description": "CC email addresses",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"subject": map[string]interface{}{
				"type":        "string",
				"description": "Email subject (auto-generated for reply/forward if not provided)",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Email body content (plain text)",
			},
			"html_body": map[string]interface{}{
				"type":        "string",
				"description": "Email body content (HTML)",
			},
			"include_original": map[string]interface{}{
				"type":        "boolean",
				"description": "Include original email in reply/forward (default: true for forward)",
			},
		},
		"required": []string{"action", "body"},
	}
}

// EmailArgs represents the arguments for the email tool
type EmailArgs struct {
	Action          string   `json:"action"`
	To              []string `json:"to"`
	Cc              []string `json:"cc"`
	Subject         string   `json:"subject"`
	Body            string   `json:"body"`
	HTMLBody        string   `json:"html_body"`
	IncludeOriginal *bool    `json:"include_original"`
}

// EmailResult represents the result of an email operation
type EmailResult struct {
	Sent    bool     `json:"sent"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Message string   `json:"message"`
}

func (t *EmailTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params EmailArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return NewErrorResult(fmt.Errorf("invalid arguments: %w", err))
	}

	switch params.Action {
	case "reply":
		return t.executeReply(ctx, params)
	case "forward":
		return t.executeForward(ctx, params)
	case "send":
		return t.executeSend(ctx, params)
	default:
		return NewErrorResult(fmt.Errorf("unknown action: %s", params.Action))
	}
}

func (t *EmailTool) executeReply(ctx context.Context, params EmailArgs) (json.RawMessage, error) {
	if t.currentEmail == nil {
		return NewErrorResult(fmt.Errorf("no current email to reply to"))
	}

	// Determine recipient (reply to sender or reply-to address)
	var toAddr email.Address
	if t.currentEmail.ReplyTo != nil {
		toAddr = *t.currentEmail.ReplyTo
	} else {
		toAddr = t.currentEmail.From
	}

	// Build subject
	subject := params.Subject
	if subject == "" {
		if !strings.HasPrefix(strings.ToLower(t.currentEmail.Subject), "re:") {
			subject = "Re: " + t.currentEmail.Subject
		} else {
			subject = t.currentEmail.Subject
		}
	}

	// Build body with original message if requested
	body := params.Body
	if params.IncludeOriginal != nil && *params.IncludeOriginal {
		body = t.appendOriginalMessage(body)
	}

	outbound := &email.OutboundEmail{
		From:      email.Address{Name: t.fromName, Address: t.fromAddress},
		To:        []email.Address{toAddr},
		Subject:   subject,
		TextBody:  body,
		HTMLBody:  params.HTMLBody,
		InReplyTo: t.currentEmail.MessageID,
	}

	if err := t.sender.Send(ctx, outbound); err != nil {
		return NewErrorResult(fmt.Errorf("failed to send reply: %w", err))
	}

	return NewSuccessResult(EmailResult{
		Sent:    true,
		To:      []string{toAddr.Address},
		Subject: subject,
		Message: "Reply sent successfully",
	})
}

func (t *EmailTool) executeForward(ctx context.Context, params EmailArgs) (json.RawMessage, error) {
	if t.currentEmail == nil {
		return NewErrorResult(fmt.Errorf("no current email to forward"))
	}

	if len(params.To) == 0 {
		return NewErrorResult(fmt.Errorf("recipients required for forward"))
	}

	// Build subject
	subject := params.Subject
	if subject == "" {
		if !strings.HasPrefix(strings.ToLower(t.currentEmail.Subject), "fwd:") {
			subject = "Fwd: " + t.currentEmail.Subject
		} else {
			subject = t.currentEmail.Subject
		}
	}

	// Include original message by default for forwards
	includeOriginal := params.IncludeOriginal == nil || *params.IncludeOriginal
	body := params.Body
	if includeOriginal {
		body = t.appendOriginalMessage(body)
	}

	// Convert to addresses
	toAddrs := make([]email.Address, len(params.To))
	for i, addr := range params.To {
		toAddrs[i] = email.Address{Address: addr}
	}

	ccAddrs := make([]email.Address, len(params.Cc))
	for i, addr := range params.Cc {
		ccAddrs[i] = email.Address{Address: addr}
	}

	outbound := &email.OutboundEmail{
		From:     email.Address{Name: t.fromName, Address: t.fromAddress},
		To:       toAddrs,
		Cc:       ccAddrs,
		Subject:  subject,
		TextBody: body,
		HTMLBody: params.HTMLBody,
	}

	if err := t.sender.Send(ctx, outbound); err != nil {
		return NewErrorResult(fmt.Errorf("failed to forward email: %w", err))
	}

	return NewSuccessResult(EmailResult{
		Sent:    true,
		To:      params.To,
		Subject: subject,
		Message: "Email forwarded successfully",
	})
}

func (t *EmailTool) executeSend(ctx context.Context, params EmailArgs) (json.RawMessage, error) {
	if len(params.To) == 0 {
		return NewErrorResult(fmt.Errorf("recipients required"))
	}

	if params.Subject == "" {
		return NewErrorResult(fmt.Errorf("subject required for new email"))
	}

	toAddrs := make([]email.Address, len(params.To))
	for i, addr := range params.To {
		toAddrs[i] = email.Address{Address: addr}
	}

	ccAddrs := make([]email.Address, len(params.Cc))
	for i, addr := range params.Cc {
		ccAddrs[i] = email.Address{Address: addr}
	}

	outbound := &email.OutboundEmail{
		From:     email.Address{Name: t.fromName, Address: t.fromAddress},
		To:       toAddrs,
		Cc:       ccAddrs,
		Subject:  params.Subject,
		TextBody: params.Body,
		HTMLBody: params.HTMLBody,
	}

	if err := t.sender.Send(ctx, outbound); err != nil {
		return NewErrorResult(fmt.Errorf("failed to send email: %w", err))
	}

	return NewSuccessResult(EmailResult{
		Sent:    true,
		To:      params.To,
		Subject: params.Subject,
		Message: "Email sent successfully",
	})
}

func (t *EmailTool) appendOriginalMessage(body string) string {
	if t.currentEmail == nil {
		return body
	}

	original := fmt.Sprintf(`

---------- Original Message ----------
From: %s
Date: %s
Subject: %s

%s`,
		t.currentEmail.From.String(),
		t.currentEmail.Date.Format("Mon, 02 Jan 2006 15:04:05 -0700"),
		t.currentEmail.Subject,
		t.currentEmail.Body(),
	)

	return body + original
}

// SMTPSender sends emails via SMTP
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
}

// NewSMTPSender creates a new SMTP sender
func NewSMTPSender(host string, port int, username, password string) *SMTPSender {
	return &SMTPSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
	}
}

func (s *SMTPSender) Send(ctx context.Context, e *email.OutboundEmail) error {
	// Build recipient list
	var recipients []string
	for _, to := range e.To {
		recipients = append(recipients, to.Address)
	}
	for _, cc := range e.Cc {
		recipients = append(recipients, cc.Address)
	}
	for _, bcc := range e.Bcc {
		recipients = append(recipients, bcc.Address)
	}

	// Build message
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", e.From.String()))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", formatAddresses(e.To)))
	if len(e.Cc) > 0 {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", formatAddresses(e.Cc)))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", e.Subject))
	if e.InReplyTo != "" {
		msg.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", e.InReplyTo))
	}
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(e.TextBody)

	// Send via SMTP
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	return smtp.SendMail(addr, auth, e.From.Address, recipients, []byte(msg.String()))
}

func formatAddresses(addrs []email.Address) string {
	parts := make([]string, len(addrs))
	for i, a := range addrs {
		parts[i] = a.String()
	}
	return strings.Join(parts, ", ")
}

// NoopSender is a sender that does nothing (for testing or when sending is disabled)
type NoopSender struct{}

func (s *NoopSender) Send(ctx context.Context, e *email.OutboundEmail) error {
	return nil
}
