package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/resend/resend-go/v2"

	"github.com/emitt/emitt/internal/email"
)

// ResendSender sends emails via Resend API
type ResendSender struct {
	client *resend.Client
}

// NewResendSender creates a new Resend sender
func NewResendSender(apiKey string) *ResendSender {
	return &ResendSender{
		client: resend.NewClient(apiKey),
	}
}

func (s *ResendSender) Send(ctx context.Context, e *email.OutboundEmail) error {
	// Build recipient list
	to := make([]string, len(e.To))
	for i, addr := range e.To {
		to[i] = addr.Address
	}

	// Build CC list
	var cc []string
	for _, addr := range e.Cc {
		cc = append(cc, addr.Address)
	}

	// Build BCC list
	var bcc []string
	for _, addr := range e.Bcc {
		bcc = append(bcc, addr.Address)
	}

	// Build from address
	from := e.From.Address
	if e.From.Name != "" {
		from = fmt.Sprintf("%s <%s>", e.From.Name, e.From.Address)
	}

	// Build request
	params := &resend.SendEmailRequest{
		From:    from,
		To:      to,
		Subject: e.Subject,
	}

	if len(cc) > 0 {
		params.Cc = cc
	}
	if len(bcc) > 0 {
		params.Bcc = bcc
	}

	// Set body (prefer HTML if available)
	if e.HTMLBody != "" {
		params.Html = e.HTMLBody
	}
	if e.TextBody != "" {
		params.Text = e.TextBody
	}

	// Set reply headers
	if e.InReplyTo != "" {
		params.Headers = map[string]string{
			"In-Reply-To": e.InReplyTo,
		}
		if len(e.References) > 0 {
			params.Headers["References"] = strings.Join(e.References, " ")
		}
	}

	// Send
	_, err := s.client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}

	return nil
}
