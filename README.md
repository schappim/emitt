# Emitt

A Go application for receiving and processing inbound emails with LLM-powered automation. Similar to Postmark's inbound email processing or Rails Action Mailbox, but with built-in AI capabilities.

## Features

- **SMTP Server**: Built-in SMTP server for receiving inbound emails
- **Flexible Routing**: YAML-based routing rules with regex matching on from/to/subject
- **LLM Processing**: OpenAI integration with function calling for intelligent email handling
- **Built-in Tools**:
  - `http_request` - Make HTTP/webhook calls
  - `database_query` - Execute SQL queries
  - `send_email` - Reply, forward, or send new emails
- **MCP Support**: Connect to Model Context Protocol servers for extensible tooling
- **SQLite Storage**: Persistent storage for emails, processing logs, and tool calls

## Installation

### From Source

```bash
git clone https://github.com/yourusername/emitt.git
cd emitt
go build -o emitt ./cmd/emitt
```

### Requirements

- Go 1.21+
- OpenAI API key (for LLM processing)

## Configuration

Create a `config.yaml` file:

```yaml
server:
  smtp_port: 25
  smtp_host: "0.0.0.0"
  allowed_domains:
    - "yourdomain.com"

database:
  path: "./emitt.db"

llm:
  provider: "openai"
  api_key: "${OPENAI_API_KEY}"
  model: "gpt-4o"
  max_tokens: 4096
  temperature: 0.7

mcp:
  servers: []

mailboxes:
  - name: "support"
    match:
      to: "support@.*"
    processor:
      type: "llm"
      system_prompt: |
        You are a support assistant. Analyze incoming emails and respond appropriately.
      tools:
        - http_request
        - database_query
        - send_email

  - name: "catch-all"
    match:
      to: ".*"
    processor:
      type: "noop"
```

### Processor Types

- `llm` - Process with OpenAI, can use tools
- `forward` - Forward to another email address
- `webhook` - POST email data to a URL
- `noop` - Store only, no processing

### Environment Variables

- `OPENAI_API_KEY` - Your OpenAI API key

## Usage

```bash
# Run with default config
./emitt

# Run with custom config
./emitt -config /path/to/config.yaml

# Enable debug logging
./emitt -debug
```

## Deployment

### Systemd Service

Create `/etc/systemd/system/emitt.service`:

```ini
[Unit]
Description=Emitt Email Processing Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/emitt
ExecStart=/opt/emitt/emitt -config /opt/emitt/config.yaml
Restart=on-failure
RestartSec=5
Environment=OPENAI_API_KEY=your-key-here

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable emitt
sudo systemctl start emitt
```

### DNS Records

For email reception, add these DNS records:

| Type | Name | Value |
|------|------|-------|
| A | mail.yourdomain.com | your-server-ip |
| MX | yourdomain.com | 10 mail.yourdomain.com |
| TXT | yourdomain.com | v=spf1 ip4:your-server-ip ~all |

### Firewall

Open port 25 (SMTP):

```bash
sudo ufw allow 25/tcp
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         EMITT                                    │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐  │
│  │  SMTP    │───▶│  Router  │───▶│ Processor│───▶│  Actions │  │
│  │  Server  │    │  Engine  │    │  (LLM)   │    │  (Tools) │  │
│  └──────────┘    └──────────┘    └──────────┘    └──────────┘  │
│       │              │               │                │         │
│       ▼              ▼               ▼                ▼         │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    SQLite Database                        │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              │                                  │
│                              ▼                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    MCP Client (optional)                  │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Project Structure

```
emitt/
├── cmd/emitt/main.go           # Application entry point
├── config.yaml                 # Default configuration
├── internal/
│   ├── config/                 # Configuration loading
│   ├── email/                  # Email models and parsing
│   ├── mcp/                    # MCP protocol client
│   ├── processor/              # LLM integration and orchestration
│   ├── router/                 # Email routing engine
│   ├── smtp/                   # SMTP server
│   ├── storage/                # SQLite database layer
│   └── tools/                  # Built-in tools (HTTP, DB, Email)
```

## Testing

Send a test email:

```bash
# Using swaks
swaks --to test@yourdomain.com \
      --from sender@example.com \
      --server localhost:25 \
      --header "Subject: Test Email" \
      --body "This is a test message"

# Using telnet
telnet localhost 25
HELO test
MAIL FROM:<sender@example.com>
RCPT TO:<test@yourdomain.com>
DATA
Subject: Test
This is a test.
.
QUIT
```

## License

MIT
