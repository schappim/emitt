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
  model: "gpt-5.2"
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

## Tools

eMitt provides three built-in tools that the LLM can use during email processing. Enable them in your mailbox configuration via the `tools` array.

### `send_email` - Email Operations

Send replies, forward emails, or compose new messages.

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | Yes | `"reply"`, `"forward"`, or `"send"` |
| `to` | array | For forward/send | Recipient email addresses |
| `cc` | array | No | CC email addresses |
| `subject` | string | For send | Email subject (auto-generated for reply/forward) |
| `body` | string | Yes | Plain text email body |
| `html_body` | string | No | HTML email body |
| `include_original` | boolean | No | Include original email (default: true for forward) |

**Example Configuration:**
```yaml
mailboxes:
  - name: "auto-responder"
    match:
      to: "info@.*"
    processor:
      type: "llm"
      system_prompt: |
        You are a helpful assistant. When someone emails, analyze their question
        and send a helpful reply using the send_email tool with action "reply".

        Always be professional and concise in your responses.
      tools:
        - send_email
```

**Example Tool Calls (what the LLM generates):**
```json
// Reply to sender
{
  "action": "reply",
  "body": "Thank you for your inquiry! Here's the information you requested..."
}

// Forward to another address
{
  "action": "forward",
  "to": ["admin@example.com"],
  "body": "Please review this customer inquiry."
}

// Send a new email
{
  "action": "send",
  "to": ["team@example.com"],
  "subject": "New Support Request",
  "body": "A new support request has been received..."
}
```

---

### `http_request` - HTTP/Webhook Calls

Make HTTP requests to external APIs and webhooks.

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `method` | string | Yes | `"GET"`, `"POST"`, `"PUT"`, `"PATCH"`, or `"DELETE"` |
| `url` | string | Yes | Full URL (must start with http:// or https://) |
| `headers` | object | No | HTTP headers as key-value pairs |
| `body` | string | No | Request body (for POST/PUT/PATCH) |
| `json_body` | object | No | JSON body (auto-serialized, sets Content-Type) |

**Example Configuration:**
```yaml
mailboxes:
  - name: "ticket-creator"
    match:
      to: "support@.*"
    processor:
      type: "llm"
      system_prompt: |
        You are a support ticket manager. When you receive an email:
        1. Extract the issue description and priority
        2. Create a ticket in our system using http_request
        3. Reply to the sender with the ticket number

        Ticket API endpoint: https://api.example.com/tickets
        API Key: Bearer xyz123
      tools:
        - http_request
        - send_email
```

**Example Tool Calls:**
```json
// POST with JSON body
{
  "method": "POST",
  "url": "https://api.example.com/tickets",
  "headers": {
    "Authorization": "Bearer xyz123"
  },
  "json_body": {
    "title": "Login issue reported",
    "description": "User cannot log in to their account",
    "priority": "high",
    "email": "customer@example.com"
  }
}

// GET request
{
  "method": "GET",
  "url": "https://api.example.com/users/123",
  "headers": {
    "Authorization": "Bearer xyz123"
  }
}

// POST to Slack webhook
{
  "method": "POST",
  "url": "https://hooks.slack.com/services/xxx/yyy/zzz",
  "json_body": {
    "text": "New support email received from customer@example.com"
  }
}
```

---

### `database_query` - SQL Database Operations

Execute SQL queries against the SQLite database.

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | SQL query to execute |
| `params` | array | No | Query parameters (for parameterized queries) |

**Supported Operations:**
- `SELECT` - Query data
- `INSERT` - Insert new records
- `UPDATE` - Update existing records
- `DELETE` - Delete records

**Blocked Operations:** `DROP`, `TRUNCATE`, `ALTER`, `CREATE` (DDL operations are not allowed)

**Example Configuration:**
```yaml
mailboxes:
  - name: "invoice-processor"
    match:
      subject: "(?i)invoice.*"
    processor:
      type: "llm"
      system_prompt: |
        You are an invoice processor. For each invoice email:
        1. Extract: invoice number, vendor name, amount, date
        2. Store the data in the invoices table using database_query
        3. Reply confirming the invoice was processed

        Database schema:
        - invoices(id, invoice_number, vendor, amount, date, processed_at)
      tools:
        - database_query
        - send_email
```

**Example Tool Calls:**
```json
// INSERT with parameters (safe from SQL injection)
{
  "query": "INSERT INTO invoices (invoice_number, vendor, amount, date, processed_at) VALUES (?, ?, ?, ?, datetime('now'))",
  "params": ["INV-2024-001", "Acme Corp", "1500.00", "2024-01-15"]
}

// SELECT query
{
  "query": "SELECT * FROM invoices WHERE vendor = ? ORDER BY date DESC LIMIT 10",
  "params": ["Acme Corp"]
}

// UPDATE query
{
  "query": "UPDATE invoices SET status = ? WHERE invoice_number = ?",
  "params": ["paid", "INV-2024-001"]
}
```

---

## Complete Example: Support Ticket System

Here's a complete example that uses all three tools to create an automated support system:

```yaml
server:
  smtp_port: 25
  smtp_host: "0.0.0.0"
  allowed_domains:
    - "support.example.com"

database:
  path: "./emitt.db"

llm:
  provider: "openai"
  api_key: "${OPENAI_API_KEY}"
  model: "gpt-5.2"

smtp:
  provider: "resend"
  resend_key: "${RESEND_API_KEY}"
  from_address: "support@example.com"
  from_name: "Support Team"

mailboxes:
  - name: "support-tickets"
    match:
      to: "support@.*"
    processor:
      type: "llm"
      system_prompt: |
        You are an intelligent support ticket manager. When an email arrives:

        1. ANALYZE the email to determine:
           - Category: bug, feature_request, question, billing, other
           - Priority: low, medium, high, urgent
           - Summary: Brief one-line description

        2. CREATE a ticket by calling http_request:
           POST https://api.linear.app/graphql
           Headers: Authorization: Bearer ${LINEAR_API_KEY}
           Create an issue with the extracted information

        3. STORE in database for tracking:
           INSERT INTO support_tickets (email, category, priority, summary, created_at)

        4. REPLY to the sender:
           - Acknowledge receipt
           - Provide ticket number
           - Set expectations for response time

        Be professional, empathetic, and helpful.
      tools:
        - http_request
        - database_query
        - send_email

  - name: "urgent-alerts"
    match:
      subject: "(?i)(urgent|critical|down|outage)"
    processor:
      type: "llm"
      system_prompt: |
        This is an urgent alert. Immediately:
        1. POST to Slack webhook to alert the on-call team
        2. Create a high-priority ticket
        3. Reply acknowledging the urgency
      tools:
        - http_request
        - send_email

  - name: "catch-all"
    match:
      to: ".*"
    processor:
      type: "forward"
      forward_to: "admin@example.com"
```

### Environment Variables

- `OPENAI_API_KEY` - Your OpenAI API key
- `RESEND_API_KEY` - Your Resend API key (if using Resend for email)

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
┌──────────────────────────────────────────────────────────────────────────────┐
│                                                                              │
│                                 eMitt Server                                 │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│                                                                              │
│    ┌────────────┐     ┌────────────┐    ┌────────────┐     ┌────────────┐    │
│    │    SMTP    │     │   Router   │    │ Processor  │     │  Actions   │    │
│    │   Server   │────▶│   Engine   │───▶│   (LLM)    │────▶│  (Tools)   │    │
│    └──────┬─────┘     └──────┬─────┘    └─────┬──────┘     └─────┬──────┘    │
│           │                  │                │                  │           │
│           ▼                  ▼                ▼                  ▼           │
│    ┌────────────────────────────────────────────────────────────────────┐    │
│    │                                                                    │    │
│    │                          SQLite Database                           │    │
│    │                                                                    │    │
│    └─────────────────────────────────┬──────────────────────────────────┘    │
│                                      │                                       │
│                                      ▼                                       │
│    ┌────────────────────────────────────────────────────────────────────┐    │
│    │                                                                    │    │
│    │                       MCP Client (optional)                        │    │
│    │                                                                    │    │
│    └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
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
