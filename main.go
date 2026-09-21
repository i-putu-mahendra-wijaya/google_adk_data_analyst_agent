package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"cloud.google.com/go/bigquery"
	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/adk/tool/mcptoolset"
)

const (
	DatasetID = "layer_stg" // Using your existing DatasetID
	AppName   = "bq_data_analyst_app"
	UserID    = "console_user"
)

// GetSchemaInput defines the input for our schema tool.
type GetSchemaInput struct {
	TableName string `json:"table_name" jsonschema:"Name of the table to get schema for"`
}

// ColumnInfo describes a single BigQuery table column.
type ColumnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// GetSchemaOutput defines the output for our schema tool.
type GetSchemaOutput struct {
	Columns []ColumnInfo `json:"columns"`
}

// QueryInput defines the input for our query tool.
type QueryInput struct {
	Query string `json:"query" jsonschema:"BigQuery Standard SQL read-only query to execute"`
}

// QueryOutput defines the output for our query tool.
type QueryOutput struct {
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"row_count"`
}

// newGetSchemaTool builds the get_table_schema tool bound to a BigQuery client.
func newGetSchemaTool(bqClient *bigquery.Client) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "get_table_schema",
		Description: fmt.Sprintf("Fetches column names and types for tables in the %s dataset.", DatasetID),
	}, func(ctx agent.ToolContext, input GetSchemaInput) (*GetSchemaOutput, error) {
		table := bqClient.Dataset(DatasetID).Table(input.TableName)
		meta, err := table.Metadata(ctx)
		if err != nil {
			return nil, fmt.Errorf("error fetching schema: %w", err)
		}
		out := &GetSchemaOutput{}
		for _, field := range meta.Schema {
			out.Columns = append(out.Columns, ColumnInfo{Name: field.Name, Type: string(field.Type)})
		}
		return out, nil
	})
}

// newQueryTool builds the execute_read_only_query tool bound to a BigQuery client.
func newQueryTool(bqClient *bigquery.Client) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "execute_read_only_query",
		Description: fmt.Sprintf("Executes a read-only BigQuery SQL query against the %s dataset and returns JSON rows.", DatasetID),
	}, func(ctx agent.ToolContext, input QueryInput) (*QueryOutput, error) {
		upper := strings.ToUpper(input.Query)
		if strings.Contains(upper, "DELETE") || strings.Contains(upper, "DROP") || strings.Contains(upper, "UPDATE") {
			return nil, fmt.Errorf("mutation queries are not allowed")
		}

		q := bqClient.Query(input.Query)
		it, err := q.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("query execution failed: %w", err)
		}

		out := &QueryOutput{}
		for {
			var row map[string]bigquery.Value
			err := it.Next(&row)
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}
			rowMap := make(map[string]any, len(row))
			for k, v := range row {
				rowMap[k] = v
			}
			out.Rows = append(out.Rows, rowMap)
			out.RowCount++
			if out.RowCount >= 50 { // Limit row count
				break
			}
		}
		return out, nil
	})
}

// newEmailToolset builds an ADK toolset backed by the mcp-email-server MCP
// server (see mcp-email-server/main.go), launched as a subprocess and spoken
// to over stdio. It exposes the send_email tool to the agent.
func newEmailToolset() (tool.Toolset, error) {
	return mcptoolset.New(mcptoolset.Config{
		Transport: &mcp.CommandTransport{
			Command: exec.Command("go", "run", "./mcp-email-server"),
		},
	})
}

func main() {
	// 1. Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found; falling back to system environment variables")
	}

	// 2. Read variables
	projectID := os.Getenv("GCP_PROJECT_ID")
	serviceAccountPath := os.Getenv("GCP_SERVICE_ACCOUNT_PATH")
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")

	if projectID == "" || serviceAccountPath == "" || geminiAPIKey == "" {
		log.Fatal("GCP_PROJECT_ID, GCP_SERVICE_ACCOUNT_PATH, or GEMINI_API_KEY is not set in your .env file")
	}

	ctx := context.Background()

	// 3. Initialize BigQuery client
	bqClient, err := bigquery.NewClient(ctx, projectID, option.WithCredentialsFile(serviceAccountPath))
	if err != nil {
		log.Fatalf("Failed to create BigQuery client: %v", err)
	}
	defer bqClient.Close()
	log.Println("BigQuery client initialized successfully")

	// 4. Instantiate the tools
	schemaTool, err := newGetSchemaTool(bqClient)
	if err != nil {
		log.Fatalf("Failed to create schema tool: %v", err)
	}
	queryTool, err := newQueryTool(bqClient)
	if err != nil {
		log.Fatalf("Failed to create query tool: %v", err)
	}
	emailToolset, err := newEmailToolset()
	if err != nil {
		log.Fatalf("Failed to create email toolset: %v", err)
	}

	// 5. Initialize Gemini Model
	model, err := gemini.NewModel(ctx, "gemini-3.6-flash", &genai.ClientConfig{
		APIKey: geminiAPIKey,
	})
	if err != nil {
		log.Fatalf("Failed to create Gemini model: %v", err)
	}

	// 6. Build the Data Analyst Agent
	systemPrompt := fmt.Sprintf(`You are a Data Analyst assistant specializing in the BigQuery dataset.
Project: %s, Dataset: %s.
When a user asks a business or analytical question:
1. Inspect the relevant table schema using 'get_table_schema' if you need column clarity.
2. Formulate standard BigQuery SQL queries and run them using 'execute_read_only_query'.
3. Always interpret the query results to directly answer the user's question with numbers, trends, and clear summaries.
When the user is satisfied with the analysis and asks you to email or share the findings:
4. Write a concise plain-text summary of the findings discussed so far, and send it with 'send_email'. If the recipient address or a subject line wasn't given, ask for them before sending.`, projectID, DatasetID)

	analystAgent, err := llmagent.New(llmagent.Config{
		Name:        "bq_data_analyst",
		Model:       model,
		Description: "Data Analyst agent for BigQuery analytics.",
		Instruction: systemPrompt,
		Tools:       []tool.Tool{schemaTool, queryTool},
		Toolsets:    []tool.Toolset{emailToolset},
	})
	if err != nil {
		log.Fatalf("Failed to configure analyst agent: %v", err)
	}

	// 7. Set up session service and runner
	sessionService := session.InMemoryService()
	createResp, err := sessionService.Create(ctx, &session.CreateRequest{AppName: AppName, UserID: UserID})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}
	sessionID := createResp.Session.ID()

	r, err := runner.New(runner.Config{AppName: AppName, Agent: analystAgent, SessionService: sessionService})
	if err != nil {
		log.Fatalf("Failed to create runner: %v", err)
	}

	// 8. Interactive REPL Loop
	fmt.Println("=== BigQuery Data Analyst Agent Ready ===")
	fmt.Println("Ask any question about your data (type 'exit' to quit):")
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("\nYou > ")
		if !scanner.Scan() {
			break
		}
		userInput := scanner.Text()
		if strings.ToLower(userInput) == "exit" {
			break
		}
		if strings.TrimSpace(userInput) == "" {
			continue
		}

		userMessage := genai.NewContentFromText(userInput, genai.RoleUser)

		for event, err := range r.Run(ctx, UserID, sessionID, userMessage, agent.RunConfig{StreamingMode: agent.StreamingModeNone}) {
			if err != nil {
				log.Printf("Agent error: %v", err)
				continue
			}
			if event.Content == nil {
				continue
			}
			for _, part := range event.Content.Parts {
				if part.Text != "" {
					fmt.Printf("\nAnalyst > %s\n", part.Text)
				}
			}
		}
	}
}
