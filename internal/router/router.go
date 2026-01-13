package router

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/emitt/emitt/internal/config"
	"github.com/emitt/emitt/internal/email"
)

// ProcessorType defines how an email should be processed
type ProcessorType string

const (
	ProcessorTypeLLM     ProcessorType = "llm"
	ProcessorTypeForward ProcessorType = "forward"
	ProcessorTypeWebhook ProcessorType = "webhook"
	ProcessorTypeNoop    ProcessorType = "noop"
)

// RouteResult contains the routing decision for an email
type RouteResult struct {
	MailboxName  string
	ProcessorType ProcessorType
	Config       *config.ProcessorConfig
}

// Router routes incoming emails to the appropriate processor
type Router struct {
	rules  *RuleSet
	logger zerolog.Logger
}

// NewRouter creates a new Router
func NewRouter(mailboxes []config.MailboxConfig, logger zerolog.Logger) (*Router, error) {
	rules, err := NewRuleSet(mailboxes)
	if err != nil {
		return nil, fmt.Errorf("failed to compile routing rules: %w", err)
	}

	return &Router{
		rules:  rules,
		logger: logger.With().Str("component", "router").Logger(),
	}, nil
}

// Route determines how to process an email
func (r *Router) Route(ctx context.Context, e *email.InboundEmail) (*RouteResult, error) {
	rule := r.rules.FindMatch(e)

	if rule == nil {
		r.logger.Debug().
			Str("from", e.From.Address).
			Str("subject", e.Subject).
			Msg("No matching rule found, using noop")

		return &RouteResult{
			MailboxName:   "unmatched",
			ProcessorType: ProcessorTypeNoop,
			Config:        nil,
		}, nil
	}

	r.logger.Info().
		Str("mailbox", rule.Name).
		Str("processor_type", rule.Processor.Type).
		Str("from", e.From.Address).
		Str("subject", e.Subject).
		Msg("Email routed to mailbox")

	procType := ProcessorType(rule.Processor.Type)
	if procType == "" {
		procType = ProcessorTypeLLM
	}

	return &RouteResult{
		MailboxName:   rule.Name,
		ProcessorType: procType,
		Config:        rule.Processor,
	}, nil
}

// GetMailboxNames returns all configured mailbox names
func (r *Router) GetMailboxNames() []string {
	rules := r.rules.Rules()
	names := make([]string, len(rules))
	for i, rule := range rules {
		names[i] = rule.Name
	}
	return names
}

// GetRule returns a specific rule by mailbox name
func (r *Router) GetRule(name string) *Rule {
	return r.rules.GetRuleByName(name)
}
