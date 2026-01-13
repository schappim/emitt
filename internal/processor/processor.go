package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/emitt/emitt/internal/config"
	"github.com/emitt/emitt/internal/email"
	"github.com/emitt/emitt/internal/router"
	"github.com/emitt/emitt/internal/storage"
	"github.com/emitt/emitt/internal/tools"
)

// Processor orchestrates email processing
type Processor struct {
	store    *storage.Store
	router   *router.Router
	llm      *LLMClient
	registry *tools.Registry
	emailTool *tools.EmailTool
	logger   zerolog.Logger
}

// NewProcessor creates a new email processor
func NewProcessor(
	store *storage.Store,
	router *router.Router,
	llm *LLMClient,
	registry *tools.Registry,
	emailTool *tools.EmailTool,
	logger zerolog.Logger,
) *Processor {
	return &Processor{
		store:    store,
		router:   router,
		llm:      llm,
		registry: registry,
		emailTool: emailTool,
		logger:   logger.With().Str("component", "processor").Logger(),
	}
}

// Process handles an incoming email
func (p *Processor) Process(ctx context.Context, inbound *email.InboundEmail) error {
	start := time.Now()

	// Store the email
	dbEmail := &storage.Email{
		MessageID:   inbound.MessageID,
		From:        inbound.From.Address,
		To:          inbound.GetToAddresses(),
		Cc:          inbound.GetCcAddresses(),
		Subject:     inbound.Subject,
		TextBody:    inbound.TextBody,
		HTMLBody:    inbound.HTMLBody,
		RawMessage:  inbound.RawMessage,
		ReceivedAt:  inbound.ReceivedAt,
		Status:      storage.EmailStatusPending,
	}

	// Store headers as JSON
	if len(inbound.Headers) > 0 {
		headersJSON, _ := json.Marshal(inbound.Headers)
		dbEmail.Headers = headersJSON
	}

	// Store attachments metadata
	if len(inbound.Attachments) > 0 {
		attInfo := make([]storage.Attachment, len(inbound.Attachments))
		for i, att := range inbound.Attachments {
			attInfo[i] = storage.Attachment{
				Filename:    att.Filename,
				ContentType: att.ContentType,
				Size:        att.Size,
			}
		}
		attJSON, _ := json.Marshal(attInfo)
		dbEmail.Attachments = attJSON
	}

	if err := p.store.SaveEmail(ctx, dbEmail); err != nil {
		return fmt.Errorf("failed to save email: %w", err)
	}

	// Save attachments data
	for _, att := range inbound.Attachments {
		if err := p.store.SaveAttachment(ctx, dbEmail.ID, &storage.Attachment{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			Size:        att.Size,
			Data:        att.Data,
		}); err != nil {
			p.logger.Warn().Err(err).Str("filename", att.Filename).Msg("Failed to save attachment")
		}
	}

	// Route the email
	routeResult, err := p.router.Route(ctx, inbound)
	if err != nil {
		return fmt.Errorf("failed to route email: %w", err)
	}

	dbEmail.MailboxName = routeResult.MailboxName

	// Update status to processing
	if err := p.store.UpdateEmailStatus(ctx, dbEmail.ID, storage.EmailStatusProcessing); err != nil {
		return fmt.Errorf("failed to update email status: %w", err)
	}

	// Process based on type
	var processErr error
	switch routeResult.ProcessorType {
	case router.ProcessorTypeLLM:
		processErr = p.processWithLLM(ctx, dbEmail.ID, inbound, routeResult.Config)
	case router.ProcessorTypeForward:
		processErr = p.processForward(ctx, dbEmail.ID, inbound, routeResult.Config)
	case router.ProcessorTypeWebhook:
		processErr = p.processWebhook(ctx, dbEmail.ID, inbound, routeResult.Config)
	case router.ProcessorTypeNoop:
		p.logger.Info().Int64("email_id", dbEmail.ID).Msg("No-op processor, email stored only")
	}

	// Update final status
	finalStatus := storage.EmailStatusCompleted
	if processErr != nil {
		finalStatus = storage.EmailStatusFailed
		p.logger.Error().Err(processErr).Int64("email_id", dbEmail.ID).Msg("Processing failed")
	}

	if err := p.store.UpdateEmailStatus(ctx, dbEmail.ID, finalStatus); err != nil {
		p.logger.Error().Err(err).Msg("Failed to update final status")
	}

	duration := time.Since(start)
	p.logger.Info().
		Int64("email_id", dbEmail.ID).
		Str("mailbox", routeResult.MailboxName).
		Str("status", string(finalStatus)).
		Dur("duration", duration).
		Msg("Email processing completed")

	return processErr
}

// processWithLLM processes an email using the LLM
func (p *Processor) processWithLLM(ctx context.Context, emailID int64, inbound *email.InboundEmail, cfg *config.ProcessorConfig) error {
	startTime := time.Now()

	// Set current email context for the email tool
	if p.emailTool != nil {
		p.emailTool.SetCurrentEmail(inbound)
	}

	// Build email context message
	emailCtx := inbound.ToContext()
	emailJSON, _ := json.MarshalIndent(emailCtx, "", "  ")

	userMessage := fmt.Sprintf(`Process the following email:

%s

Analyze the email and take appropriate actions using the available tools.`, string(emailJSON))

	// Log processing start
	p.store.SaveProcessingLog(ctx, &storage.ProcessingLog{
		EmailID:   emailID,
		Step:      "llm_start",
		Input:     userMessage,
		CreatedAt: time.Now(),
	})

	// Process with LLM
	result, err := p.llm.ProcessWithTools(
		ctx,
		cfg.SystemPrompt,
		userMessage,
		p.registry,
		cfg.Tools,
		10, // max iterations
	)

	duration := time.Since(startTime).Milliseconds()

	// Log completion
	logEntry := &storage.ProcessingLog{
		EmailID:   emailID,
		Step:      "llm_complete",
		Output:    result,
		Duration:  duration,
		CreatedAt: time.Now(),
	}
	if err != nil {
		logEntry.Error = err.Error()
	}
	p.store.SaveProcessingLog(ctx, logEntry)

	return err
}

// processForward forwards the email to the configured address
func (p *Processor) processForward(ctx context.Context, emailID int64, inbound *email.InboundEmail, cfg *config.ProcessorConfig) error {
	if cfg.ForwardTo == "" {
		return fmt.Errorf("forward_to address not configured")
	}

	if p.emailTool == nil {
		return fmt.Errorf("email tool not configured")
	}

	p.emailTool.SetCurrentEmail(inbound)

	args := map[string]interface{}{
		"action": "forward",
		"to":     []string{cfg.ForwardTo},
		"body":   "Forwarded email - see original below.",
		"include_original": true,
	}
	argsJSON, _ := json.Marshal(args)

	_, err := p.emailTool.Execute(ctx, argsJSON)
	return err
}

// processWebhook sends the email to a webhook URL
func (p *Processor) processWebhook(ctx context.Context, emailID int64, inbound *email.InboundEmail, cfg *config.ProcessorConfig) error {
	if cfg.WebhookURL == "" {
		return fmt.Errorf("webhook_url not configured")
	}

	httpTool := tools.NewHTTPTool()

	emailCtx := inbound.ToContext()
	payload := map[string]interface{}{
		"event":    "email.received",
		"email_id": emailID,
		"email":    emailCtx,
	}
	payloadJSON, _ := json.Marshal(payload)

	args := map[string]interface{}{
		"method":    "POST",
		"url":       cfg.WebhookURL,
		"json_body": json.RawMessage(payloadJSON),
	}
	argsJSON, _ := json.Marshal(args)

	_, err := httpTool.Execute(ctx, argsJSON)
	return err
}

// ProcessPending processes all pending emails
func (p *Processor) ProcessPending(ctx context.Context, limit int) error {
	emails, err := p.store.GetPendingEmails(ctx, limit)
	if err != nil {
		return fmt.Errorf("failed to get pending emails: %w", err)
	}

	p.logger.Info().Int("count", len(emails)).Msg("Processing pending emails")

	for _, dbEmail := range emails {
		// Re-parse the email from raw message
		parser := email.NewParser()
		inbound, err := parser.Parse(dbEmail.RawMessage)
		if err != nil {
			p.logger.Error().Err(err).Int64("email_id", dbEmail.ID).Msg("Failed to parse stored email")
			p.store.UpdateEmailStatus(ctx, dbEmail.ID, storage.EmailStatusFailed)
			continue
		}

		if err := p.Process(ctx, inbound); err != nil {
			p.logger.Error().Err(err).Int64("email_id", dbEmail.ID).Msg("Failed to process pending email")
		}
	}

	return nil
}
