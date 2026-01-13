package config

import (
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	SMTP      SMTPOutConfig   `yaml:"smtp"`
	Database  DatabaseConfig  `yaml:"database"`
	LLM       LLMConfig       `yaml:"llm"`
	MCP       MCPConfig       `yaml:"mcp"`
	Mailboxes []MailboxConfig `yaml:"mailboxes"`
}

// SMTPOutConfig holds outbound email settings
type SMTPOutConfig struct {
	Provider    string `yaml:"provider"` // "resend", "smtp", or empty for none
	ResendKey   string `yaml:"resend_key"`
	FromAddress string `yaml:"from_address"`
	FromName    string `yaml:"from_name"`
	// SMTP settings (if provider is "smtp")
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// ServerConfig holds SMTP server settings
type ServerConfig struct {
	SMTPPort       int        `yaml:"smtp_port"`
	SMTPHost       string     `yaml:"smtp_host"`
	TLS            TLSConfig  `yaml:"tls"`
	AllowedDomains []string   `yaml:"allowed_domains"`
}

// TLSConfig holds TLS settings
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// LLMConfig holds LLM provider settings
type LLMConfig struct {
	Provider    string  `yaml:"provider"`
	APIKey      string  `yaml:"api_key"`
	Model       string  `yaml:"model"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float32 `yaml:"temperature"`
}

// MCPConfig holds MCP server configurations
type MCPConfig struct {
	Servers []MCPServerConfig `yaml:"servers"`
}

// MCPServerConfig represents a single MCP server
type MCPServerConfig struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env"`
}

// MailboxConfig defines a routing rule and processor
type MailboxConfig struct {
	Name      string          `yaml:"name"`
	Match     MatchConfig     `yaml:"match"`
	Processor ProcessorConfig `yaml:"processor"`
}

// MatchConfig defines email matching criteria
type MatchConfig struct {
	From    string `yaml:"from"`
	To      string `yaml:"to"`
	Subject string `yaml:"subject"`
}

// CompiledMatch holds compiled regex patterns for matching
type CompiledMatch struct {
	From    *regexp.Regexp
	To      *regexp.Regexp
	Subject *regexp.Regexp
}

// Compile compiles the match patterns into regex
func (m *MatchConfig) Compile() (*CompiledMatch, error) {
	cm := &CompiledMatch{}
	var err error

	if m.From != "" {
		cm.From, err = regexp.Compile(m.From)
		if err != nil {
			return nil, err
		}
	}

	if m.To != "" {
		cm.To, err = regexp.Compile(m.To)
		if err != nil {
			return nil, err
		}
	}

	if m.Subject != "" {
		cm.Subject, err = regexp.Compile(m.Subject)
		if err != nil {
			return nil, err
		}
	}

	return cm, nil
}

// ProcessorConfig defines how to process matched emails
type ProcessorConfig struct {
	Type         string   `yaml:"type"` // "llm", "forward", "webhook"
	SystemPrompt string   `yaml:"system_prompt"`
	Tools        []string `yaml:"tools"`
	ForwardTo    string   `yaml:"forward_to"`
	WebhookURL   string   `yaml:"webhook_url"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand environment variables
	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}

	// Set defaults
	cfg.setDefaults()

	return &cfg, nil
}

// expandEnvVars expands ${VAR} patterns in the string
func expandEnvVars(s string) string {
	return os.Expand(s, func(key string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return "${" + key + "}"
	})
}

// setDefaults sets default values for missing configuration
func (c *Config) setDefaults() {
	if c.Server.SMTPPort == 0 {
		c.Server.SMTPPort = 2525
	}
	if c.Server.SMTPHost == "" {
		c.Server.SMTPHost = "0.0.0.0"
	}
	if c.Database.Path == "" {
		c.Database.Path = "./emitt.db"
	}
	if c.LLM.Provider == "" {
		c.LLM.Provider = "openai"
	}
	if c.LLM.Model == "" {
		c.LLM.Model = "gpt-5.2"
	}
	if c.LLM.MaxTokens == 0 {
		c.LLM.MaxTokens = 4096
	}
	if c.LLM.Temperature == 0 {
		c.LLM.Temperature = 0.7
	}
}

// GetMailboxByName returns a mailbox configuration by name
func (c *Config) GetMailboxByName(name string) *MailboxConfig {
	for i := range c.Mailboxes {
		if strings.EqualFold(c.Mailboxes[i].Name, name) {
			return &c.Mailboxes[i]
		}
	}
	return nil
}
