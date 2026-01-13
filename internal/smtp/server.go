package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/rs/zerolog"

	"github.com/emitt/emitt/internal/config"
	"github.com/emitt/emitt/internal/email"
)

// EmailHandler is called when a new email is received
type EmailHandler func(ctx context.Context, email *email.InboundEmail) error

// Server is an SMTP server for receiving inbound emails
type Server struct {
	cfg     *config.ServerConfig
	server  *smtp.Server
	handler EmailHandler
	parser  *email.Parser
	logger  zerolog.Logger
	mu      sync.RWMutex
}

// NewServer creates a new SMTP server
func NewServer(cfg *config.ServerConfig, handler EmailHandler, logger zerolog.Logger) *Server {
	s := &Server{
		cfg:     cfg,
		handler: handler,
		parser:  email.NewParser(),
		logger:  logger.With().Str("component", "smtp").Logger(),
	}

	backend := &smtpBackend{server: s}

	s.server = smtp.NewServer(backend)
	s.server.Addr = fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	s.server.Domain = "localhost"
	s.server.ReadTimeout = 60 * time.Second
	s.server.WriteTimeout = 60 * time.Second
	s.server.MaxMessageBytes = 25 * 1024 * 1024 // 25MB
	s.server.MaxRecipients = 100
	s.server.AllowInsecureAuth = true

	if cfg.TLS.Enabled {
		cert, err := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to load TLS certificate")
		} else {
			s.server.TLSConfig = &tls.Config{
				Certificates: []tls.Certificate{cert},
			}
		}
	}

	return s
}

// Start starts the SMTP server
func (s *Server) Start() error {
	s.logger.Info().
		Str("addr", s.server.Addr).
		Msg("Starting SMTP server")

	return s.server.ListenAndServe()
}

// Stop gracefully stops the SMTP server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info().Msg("Stopping SMTP server")
	return s.server.Shutdown(ctx)
}

// isAllowedDomain checks if the recipient domain is allowed
func (s *Server) isAllowedDomain(addr string) bool {
	if len(s.cfg.AllowedDomains) == 0 {
		return true
	}

	parts := strings.Split(addr, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(parts[1])

	for _, allowed := range s.cfg.AllowedDomains {
		if strings.ToLower(allowed) == domain {
			return true
		}
	}
	return false
}

// smtpBackend implements smtp.Backend
type smtpBackend struct {
	server *Server
}

func (b *smtpBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &smtpSession{
		server: b.server,
	}, nil
}

// smtpSession implements smtp.Session
type smtpSession struct {
	server *Server
	from   string
	to     []string
}

func (s *smtpSession) AuthPlain(username, password string) error {
	// Accept any auth for inbound emails
	return nil
}

func (s *smtpSession) Mail(from string, opts *smtp.MailOptions) error {
	s.server.logger.Debug().Str("from", from).Msg("MAIL FROM")
	s.from = from
	return nil
}

func (s *smtpSession) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.server.logger.Debug().Str("to", to).Msg("RCPT TO")

	if !s.server.isAllowedDomain(to) {
		s.server.logger.Warn().
			Str("to", to).
			Msg("Rejected: domain not allowed")
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 7, 1},
			Message:      "Domain not allowed",
		}
	}

	s.to = append(s.to, to)
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		s.server.logger.Error().Err(err).Msg("Failed to read message data")
		return err
	}

	rawMessage := buf.Bytes()
	s.server.logger.Debug().
		Int("size", len(rawMessage)).
		Msg("Received message data")

	// Parse the email
	parsedEmail, err := s.server.parser.Parse(rawMessage)
	if err != nil {
		s.server.logger.Error().Err(err).Msg("Failed to parse email")
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 6, 0},
			Message:      "Failed to parse message",
		}
	}

	// Set envelope information if not in headers
	if parsedEmail.From.Address == "" && s.from != "" {
		parsedEmail.From = email.Address{Address: s.from}
	}
	if len(parsedEmail.To) == 0 && len(s.to) > 0 {
		for _, addr := range s.to {
			parsedEmail.To = append(parsedEmail.To, email.Address{Address: addr})
		}
	}

	s.server.logger.Info().
		Str("from", parsedEmail.From.Address).
		Strs("to", parsedEmail.GetToAddresses()).
		Str("subject", parsedEmail.Subject).
		Str("message_id", parsedEmail.MessageID).
		Msg("Received email")

	// Handle the email asynchronously
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := s.server.handler(ctx, parsedEmail); err != nil {
			s.server.logger.Error().
				Err(err).
				Str("message_id", parsedEmail.MessageID).
				Msg("Failed to handle email")
		}
	}()

	return nil
}

func (s *smtpSession) Reset() {
	s.from = ""
	s.to = nil
}

func (s *smtpSession) Logout() error {
	return nil
}
