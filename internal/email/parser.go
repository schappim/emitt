package email

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
)

// Parser parses raw email messages
type Parser struct{}

// NewParser creates a new email parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses a raw email message
func (p *Parser) Parse(rawMessage []byte) (*InboundEmail, error) {
	reader := bytes.NewReader(rawMessage)

	entity, err := message.Read(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	email := &InboundEmail{
		RawMessage: rawMessage,
		ReceivedAt: time.Now(),
		Headers:    make(map[string]string),
	}

	// Parse headers
	header := entity.Header

	// Message-ID
	email.MessageID = header.Get("Message-ID")
	if email.MessageID == "" {
		email.MessageID = generateMessageID()
	}

	// From
	if from := header.Get("From"); from != "" {
		addr, err := parseAddress(from)
		if err == nil {
			email.From = addr
		}
	}

	// To
	if to := header.Get("To"); to != "" {
		addrs, err := parseAddressList(to)
		if err == nil {
			email.To = addrs
		}
	}

	// Cc
	if cc := header.Get("Cc"); cc != "" {
		addrs, err := parseAddressList(cc)
		if err == nil {
			email.Cc = addrs
		}
	}

	// Bcc
	if bcc := header.Get("Bcc"); bcc != "" {
		addrs, err := parseAddressList(bcc)
		if err == nil {
			email.Bcc = addrs
		}
	}

	// Reply-To
	if replyTo := header.Get("Reply-To"); replyTo != "" {
		addr, err := parseAddress(replyTo)
		if err == nil {
			email.ReplyTo = &addr
		}
	}

	// Subject
	email.Subject = decodeHeader(header.Get("Subject"))

	// Date
	if dateStr := header.Get("Date"); dateStr != "" {
		if t, err := mail.ParseDate(dateStr); err == nil {
			email.Date = t
		}
	}
	if email.Date.IsZero() {
		email.Date = time.Now()
	}

	// Store common headers
	commonHeaders := []string{
		"X-Priority", "X-Mailer", "X-Spam-Status", "X-Spam-Score",
		"List-Unsubscribe", "List-Id", "Precedence", "Auto-Submitted",
	}
	for _, h := range commonHeaders {
		if val := header.Get(h); val != "" {
			email.Headers[h] = val
		}
	}

	// Parse body
	if err := p.parseBody(entity, email); err != nil {
		return nil, fmt.Errorf("failed to parse body: %w", err)
	}

	return email, nil
}

// parseBody recursively parses the message body and attachments
func (p *Parser) parseBody(entity *message.Entity, email *InboundEmail) error {
	mediaType, params, err := entity.Header.ContentType()
	if err != nil {
		mediaType = "text/plain"
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := entity.MultipartReader()
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if err := p.parseBody(part, email); err != nil {
				return err
			}
		}
		return nil
	}

	// Read the body
	body, err := io.ReadAll(entity.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}

	// Check if it's an attachment
	disposition, dispParams, _ := entity.Header.ContentDisposition()
	filename := dispParams["filename"]
	if filename == "" {
		filename = params["name"]
	}

	if disposition == "attachment" || (filename != "" && disposition != "inline") {
		att := Attachment{
			Filename:    decodeHeader(filename),
			ContentType: mediaType,
			Data:        body,
			Size:        int64(len(body)),
		}
		if contentID := entity.Header.Get("Content-ID"); contentID != "" {
			att.ContentID = strings.Trim(contentID, "<>")
		}
		email.Attachments = append(email.Attachments, att)
		return nil
	}

	// It's a body part
	switch {
	case strings.HasPrefix(mediaType, "text/plain"):
		email.TextBody = string(body)
	case strings.HasPrefix(mediaType, "text/html"):
		email.HTMLBody = string(body)
	}

	return nil
}

// parseAddress parses a single email address
func parseAddress(s string) (Address, error) {
	addr, err := mail.ParseAddress(s)
	if err != nil {
		// Try to extract just the email
		s = strings.TrimSpace(s)
		if strings.Contains(s, "@") {
			return Address{Address: s}, nil
		}
		return Address{}, err
	}
	return Address{
		Name:    addr.Name,
		Address: addr.Address,
	}, nil
}

// parseAddressList parses a comma-separated list of email addresses
func parseAddressList(s string) ([]Address, error) {
	addrs, err := mail.ParseAddressList(s)
	if err != nil {
		// Try splitting manually
		parts := strings.Split(s, ",")
		var result []Address
		for _, p := range parts {
			addr, err := parseAddress(strings.TrimSpace(p))
			if err == nil {
				result = append(result, addr)
			}
		}
		if len(result) > 0 {
			return result, nil
		}
		return nil, err
	}

	result := make([]Address, len(addrs))
	for i, addr := range addrs {
		result[i] = Address{
			Name:    addr.Name,
			Address: addr.Address,
		}
	}
	return result, nil
}

// decodeHeader decodes RFC 2047 encoded header values
func decodeHeader(s string) string {
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("<%d.emitt@localhost>", time.Now().UnixNano())
}
