package router

import (
	"github.com/emitt/emitt/internal/config"
	"github.com/emitt/emitt/internal/email"
)

// Rule represents a compiled routing rule
type Rule struct {
	Name      string
	Match     *config.CompiledMatch
	Processor *config.ProcessorConfig
	Priority  int
}

// Matches checks if an email matches this rule
func (r *Rule) Matches(e *email.InboundEmail) bool {
	// Check From pattern
	if r.Match.From != nil {
		if !r.Match.From.MatchString(e.From.Address) {
			return false
		}
	}

	// Check To pattern (match any recipient)
	if r.Match.To != nil {
		matched := false
		for _, to := range e.To {
			if r.Match.To.MatchString(to.Address) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check Subject pattern
	if r.Match.Subject != nil {
		if !r.Match.Subject.MatchString(e.Subject) {
			return false
		}
	}

	return true
}

// RuleSet is a collection of routing rules
type RuleSet struct {
	rules []*Rule
}

// NewRuleSet creates a new RuleSet from mailbox configurations
func NewRuleSet(mailboxes []config.MailboxConfig) (*RuleSet, error) {
	rs := &RuleSet{
		rules: make([]*Rule, 0, len(mailboxes)),
	}

	for i, mb := range mailboxes {
		compiled, err := mb.Match.Compile()
		if err != nil {
			return nil, err
		}

		rule := &Rule{
			Name:      mb.Name,
			Match:     compiled,
			Processor: &mailboxes[i].Processor,
			Priority:  i, // Earlier rules have higher priority
		}
		rs.rules = append(rs.rules, rule)
	}

	return rs, nil
}

// FindMatch finds the first matching rule for an email
func (rs *RuleSet) FindMatch(e *email.InboundEmail) *Rule {
	for _, rule := range rs.rules {
		if rule.Matches(e) {
			return rule
		}
	}
	return nil
}

// GetRuleByName returns a rule by its name
func (rs *RuleSet) GetRuleByName(name string) *Rule {
	for _, rule := range rs.rules {
		if rule.Name == name {
			return rule
		}
	}
	return nil
}

// Rules returns all rules
func (rs *RuleSet) Rules() []*Rule {
	return rs.rules
}
