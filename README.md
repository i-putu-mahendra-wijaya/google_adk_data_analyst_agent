# BigQuery Data Analyst Agent

A conversational data-analyst agent built with [Google's Agent Development Kit (ADK) for Go](https://google.github.io/adk-docs/) and Gemini. The agent answers natural-language questions about a BigQuery dataset by inspecting table schemas and running read-only SQL queries on your behalf, then summarizing the results in plain English.

## How it works

The agent is an ADK `llmagent` backed by a Gemini model and equipped with two tools:

| Tool | Purpose |
|---|---|
| `get_table_schema` | Fetches column names and types for a table in the configured BigQuery dataset. |
| `execute_read_only_query` | Runs a BigQuery Standard SQL query and returns up to 50 rows as JSON. Queries containing `DELETE`, `DROP`, or `UPDATE` are rejected. |

On startup, the agent opens an interactive REPL in your terminal: you ask questions, the agent decides whether it needs schema info, writes and executes SQL, and interprets the results for you.

## Requirements

- **Go 1.25.0** or later (see [`go.mod`](go.mod))
- A **Google Cloud project** with the BigQuery API enabled and a dataset to query
- A **GCP service account** with BigQuery read permissions (`roles/bigquery.dataViewer` and `roles/bigquery.jobUser` at minimum), downloaded as a JSON key file
- A **Gemini API key** (from [Google AI Studio](https://aistudio.google.com/apikey))

### Key dependencies

Declared in [`go.mod`](go.mod):

- [`google.golang.org/adk`](https://pkg.go.dev/google.golang.org/adk) — Agent Development Kit (agent, model, runner, session, tool abstractions)
- [`google.golang.org/genai`](https://pkg.go.dev/google.golang.org/genai) — Gemini client used by the ADK model provider
- [`cloud.google.com/go/bigquery`](https://pkg.go.dev/cloud.google.com/go/bigquery) — BigQuery client
- [`google.golang.org/api`](https://pkg.go.dev/google.golang.org/api) — Google API client options (credentials, iterators)
- [`github.com/joho/godotenv`](https://pkg.go.dev/github.com/joho/godotenv) — loads environment variables from a `.env` file

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
```

| Variable | Description |
|---|---|
| `GCP_PROJECT_ID` | The GCP project that hosts your BigQuery dataset. |
| `GCP_SERVICE_ACCOUNT_PATH` | Filesystem path to a service account JSON key with BigQuery read access. Keep this file outside version control. |
| `GEMINI_API_KEY` | API key used to authenticate with the Gemini model. |

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

## Notes for reproducing this work

- **Credentials are never read from flags or hardcoded values** — the app fails fast with a clear error if `GCP_PROJECT_ID`, `GCP_SERVICE_ACCOUNT_PATH`, or `GEMINI_API_KEY` are missing.
- **Query safety** — `execute_read_only_query` does a simple keyword check to block `DELETE`/`DROP`/`UPDATE` statements and caps results at 50 rows. This is a basic guardrail, not a substitute for least-privilege IAM permissions on the service account.
- **Session state** is in-memory only (`session.InMemoryService()`); conversation history is lost when the process exits.
- **Model** — the agent currently targets `gemini-3.6-flash`; change the model name in `main.go` if you need a different Gemini variant.
- The compiled binary (`google_adk_data_analyst_agent`) and `.env` are git-ignored — don't commit either.
