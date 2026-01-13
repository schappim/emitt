# 🧤 eMitt

Catch every email. **eMitt** is a Go application for receiving and processing inbound emails with LLM-powered automation. Similar to Postmark's inbound email processing or Rails Action Mailbox, but with built-in AI capabilities.

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

### Quick Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/schappim/emitt/main/install.sh | sh
```

This automatically detects your OS and architecture, downloads the appropriate binary, and installs it to `/usr/local/bin`.

### Download Binary

Download the latest release for your platform:

| Platform | Architecture | Download |
|----------|--------------|----------|
| Linux | x86_64 | [emitt-linux-amd64](https://github.com/schappim/emitt/releases/latest/download/emitt-linux-amd64) |
| Linux | ARM64 | [emitt-linux-arm64](https://github.com/schappim/emitt/releases/latest/download/emitt-linux-arm64) |
| macOS | Intel | [emitt-darwin-amd64](https://github.com/schappim/emitt/releases/latest/download/emitt-darwin-amd64) |
| macOS | Apple Silicon | [emitt-darwin-arm64](https://github.com/schappim/emitt/releases/latest/download/emitt-darwin-arm64) |
| Windows | x86_64 | [emitt-windows-amd64.exe](https://github.com/schappim/emitt/releases/latest/download/emitt-windows-amd64.exe) |

```bash
# Example: Linux x86_64
curl -fsSL https://github.com/schappim/emitt/releases/latest/download/emitt-linux-amd64 -o emitt
chmod +x emitt
sudo mv emitt /usr/local/bin/
```

### From Source

```bash
git clone https://github.com/schappim/emitt.git
cd emitt
go build -o emitt ./cmd/emitt
```

### Requirements

- OpenAI API key (for LLM processing)
- Go 1.21+ (only if building from source)

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
Description=eMitt Email Processing Server
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
│                       🧤 eMitt                                   │
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

## Development

### Build from Source

```bash
# Build for current platform
./scripts/build.sh local

# Build for all platforms
./scripts/build.sh

# Build for specific OS
./scripts/build.sh linux
./scripts/build.sh darwin
./scripts/build.sh windows
```

### Create a Release

```bash
# Interactive release (prompts for version)
./scripts/release.sh

# Or specify version directly
./scripts/release.sh v1.1.0
```

The release script will:
1. Build binaries for all platforms
2. Create SHA-256 checksums
3. Create and push a git tag
4. Create a GitHub release with all assets

## License

MIT
