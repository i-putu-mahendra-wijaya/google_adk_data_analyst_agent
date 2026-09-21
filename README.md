# BigQuery Data Analyst Agent

A conversational data-analyst agent built with [Google's Agent Development Kit (ADK) for Go](https://google.github.io/adk-docs/) and Gemini. The agent answers natural-language questions about a BigQuery dataset by inspecting table schemas and running read-only SQL queries on your behalf, then summarizing the results in plain English.

## How it works

The agent is an ADK `llmagent` backed by a Gemini model and equipped with three tools:

| Tool | Purpose |
|---|---|
| `get_table_schema` | Fetches column names and types for a table in the configured BigQuery dataset. |
| `execute_read_only_query` | Runs a BigQuery Standard SQL query and returns up to 50 rows as JSON. Queries containing `DELETE`, `DROP`, or `UPDATE` are rejected. |
| `send_email` | Sends a plain-text email over SMTP. Backed by the [`mcp-email-server`](#email-sending-mcp-server) MCP server below, wired in via `mcptoolset` (see `newEmailToolset` in [`main.go`](main.go)). |

On startup, the agent opens an interactive REPL in your terminal: you ask questions, the agent decides whether it needs schema info, writes and executes SQL, and interprets the results for you. Once you're satisfied with the exploration, ask the agent to summarize the findings and email them to someone — it will draft a plain-text summary from the conversation and call `send_email`.

**Note:** `send_email` is served by `mcp-email-server`, which the agent launches as a subprocess (`go run ./mcp-email-server`) the first time it needs the tool list. This means the SMTP variables below must be configured in `.env` even if you never end up sending an email — if the MCP subprocess can't start, the *entire* tool list (including the BigQuery tools) fails to load for that turn.

## Requirements

- **Go 1.25.0** or later (see [`go.mod`](go.mod))
- A **Google Cloud project** with the BigQuery API enabled and a dataset to query
- A **GCP service account** with BigQuery read permissions (`roles/bigquery.dataViewer` and `roles/bigquery.jobUser` at minimum), downloaded as a JSON key file
- A **Gemini API key** (from [Google AI Studio](https://aistudio.google.com/apikey))
- **SMTP credentials** for sending email (see [Email-Sending MCP Server](#email-sending-mcp-server) below) — required at startup, not just for the email feature

### Key dependencies

Declared in [`go.mod`](go.mod):

- [`google.golang.org/adk`](https://pkg.go.dev/google.golang.org/adk) — Agent Development Kit (agent, model, runner, session, tool abstractions)
- [`google.golang.org/genai`](https://pkg.go.dev/google.golang.org/genai) — Gemini client used by the ADK model provider
- [`cloud.google.com/go/bigquery`](https://pkg.go.dev/cloud.google.com/go/bigquery) — BigQuery client
- [`google.golang.org/api`](https://pkg.go.dev/google.golang.org/api) — Google API client options (credentials, iterators)
- [`github.com/joho/godotenv`](https://pkg.go.dev/github.com/joho/godotenv) — loads environment variables from a `.env` file
- [`github.com/modelcontextprotocol/go-sdk`](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk) — MCP client/server implementation used to talk to `mcp-email-server`

All transitive dependencies are pinned in [`go.sum`](go.sum).

## Setup

### 1. Clone and install dependencies

```bash
mkdir 20260826_google_adk 
git clone https://github.com/i-putu-mahendra-wijaya/google_adk_data_analyst_agent.git
cd 20260826_google_adk
go mod download
```

### 2. Configure credentials

Copy the provided template to a real `.env` file (this file is git-ignored and must never be committed):

```bash
cp env.template .env
```

Then fill in your own values:

```dotenv
GCP_PROJECT_ID="your-project-id"
GCP_SERVICE_ACCOUNT_PATH="path/to/your/service_account.json"
GEMINI_API_KEY="YOUR_GEMINI_API_KEY"

SMTP_HOST="smtp.gmail.com"
SMTP_PORT="587"
SMTP_USERNAME="your-email@gmail.com"
SMTP_PASSWORD="your-app-password"
SMTP_FROM_EMAIL="your-email@gmail.com"
```

| Variable | Description |
|---|---|
| `GCP_PROJECT_ID` | The GCP project that hosts your BigQuery dataset. |
| `GCP_SERVICE_ACCOUNT_PATH` | Filesystem path to a service account JSON key with BigQuery read access. Keep this file outside version control. |
| `GEMINI_API_KEY` | API key used to authenticate with the Gemini model. |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD` | SMTP credentials used by the `send_email` tool (see [Email-Sending MCP Server](#email-sending-mcp-server)). Required — the agent fails the *first* tool call of any kind if these are missing, since it loads all tools (BigQuery + email) together. |
| `SMTP_FROM_EMAIL` | Optional. `From` address on outgoing mail; defaults to `SMTP_USERNAME` if unset. |

By default, the agent queries the `layer_stg` dataset (see the `DatasetID` constant in [`main.go`](main.go)). Update that constant if your dataset has a different name.

## Running the agent

```bash
go run main.go
```

Or build a binary first:

```bash
go build -o google_adk_data_analyst_agent .
./google_adk_data_analyst_agent
```

On startup you'll see:

```
=== BigQuery Data Analyst Agent Ready ===
Ask any question about your data (type 'exit' to quit):

You >
```

Type any question about your dataset (e.g. `How many trips on average per week for each station?`) and press Enter. Type `exit` to quit.

Once you've explored enough, ask the agent to send a summary, e.g.:

```
You > Summarize what we found and email it to jane@example.com with the subject "Trip duration findings"
```

The agent drafts a plain-text summary from the conversation and calls `send_email`; if you omit the recipient or subject, it will ask for them before sending.

## Notes for reproducing this work

- **Credentials are never read from flags or hardcoded values** — the app fails fast with a clear error if `GCP_PROJECT_ID`, `GCP_SERVICE_ACCOUNT_PATH`, or `GEMINI_API_KEY` are missing. `mcp-email-server` applies the same fail-fast check to the `SMTP_*` variables, but since it's a separate subprocess, that failure surfaces as an "Agent error" on the first tool call rather than at agent startup.
- **Query safety** — `execute_read_only_query` does a simple keyword check to block `DELETE`/`DROP`/`UPDATE` statements and caps results at 50 rows. This is a basic guardrail, not a substitute for least-privilege IAM permissions on the service account.
- **Session state** is in-memory only (`session.InMemoryService()`); conversation history is lost when the process exits.
- **Model** — the agent currently targets `gemini-3.6-flash`; change the model name in `main.go` if you need a different Gemini variant.
- The compiled binary (`google_adk_data_analyst_agent`) and `.env` are git-ignored — don't commit either.

## Email-Sending MCP Server

[`mcp-email-server`](mcp-email-server/main.go) is a [Model Context Protocol](https://modelcontextprotocol.io/) server that exposes a single `send_email` tool over SMTP. The BigQuery agent above launches it as a subprocess via `mcptoolset` (see `newEmailToolset` in [`main.go`](main.go)) so it can email analysis summaries, but it's also a standalone server any other MCP client (Claude Desktop, Claude Code, etc.) can use — it shares this repo's `go.mod`/`go.sum` but has no other dependency on the BigQuery agent.

### `send_email` tool contract

| Field | Type | Description |
|---|---|---|
| `to` | string (input) | Recipient email address. Validated with `net/mail.ParseAddress`; rejected if it contains a newline. |
| `subject` | string (input) | Email subject line. Rejected if it contains a newline (prevents SMTP header injection). |
| `body` | string (input) | Plain-text email body. |
| `status` | string (output) | `"sent"` on success. |

### Configuration

Add these variables to your `.env` (see [`env.template`](env.template)):

| Variable | Description |
|---|---|
| `SMTP_HOST` | SMTP server hostname, e.g. `smtp.gmail.com`. |
| `SMTP_PORT` | SMTP server port, e.g. `587`. |
| `SMTP_USERNAME` | SMTP account username (usually your full email address). |
| `SMTP_PASSWORD` | SMTP account password or app password. |
| `SMTP_FROM_EMAIL` | Optional. `From` address on outgoing mail; defaults to `SMTP_USERNAME` if unset. |

`SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, and `SMTP_PASSWORD` are required — the server fails fast with a clear error if any are missing.

If you're using Gmail, generate an [App Password](https://myaccount.google.com/apppasswords) (requires 2-Step Verification enabled) rather than using your regular account password — Google blocks plain password SMTP auth.

### Running standalone

```bash
go run ./mcp-email-server
```

Or build a binary first:

```bash
go build -o mcp-email-server/mcp-email-server ./mcp-email-server
./mcp-email-server/mcp-email-server
```

The server communicates over stdio, so it's meant to be launched by an MCP client rather than run interactively.

### Registering with an MCP client

From the repo root, with Claude Code:

```bash
claude mcp add email-sender -- go run ./mcp-email-server
```

Or point at a built binary:

```bash
claude mcp add email-sender -- /absolute/path/to/mcp-email-server/mcp-email-server
```

To inspect it manually, use the [MCP Inspector](https://github.com/modelcontextprotocol/inspector):

```bash
npx @modelcontextprotocol/inspector go run ./mcp-email-server
```
