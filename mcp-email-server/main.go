package main

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"net/smtp"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SendEmailInput defines the input for the send_email tool.
type SendEmailInput struct {
	To      string `json:"to" jsonschema:"Recipient email address"`
	Subject string `json:"subject" jsonschema:"Email subject line"`
	Body    string `json:"body" jsonschema:"Plain-text email body"`
}

// SendEmailOutput defines the output for the send_email tool.
type SendEmailOutput struct {
	Status string `json:"status"`
}

// smtpConfig holds the SMTP settings needed to send email.
type smtpConfig struct {
	host      string
	port      string
	username  string
	password  string
	fromEmail string
}

// newSendEmailTool builds the send_email tool bound to the given SMTP config.
func newSendEmailTool(cfg smtpConfig) func(ctx context.Context, req *mcp.CallToolRequest, input SendEmailInput) (*mcp.CallToolResult, SendEmailOutput, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input SendEmailInput) (*mcp.CallToolResult, SendEmailOutput, error) {
		if _, err := mail.ParseAddress(input.To); err != nil {
			return nil, SendEmailOutput{}, fmt.Errorf("invalid recipient address: %w", err)
		}
		if strings.ContainsAny(input.To, "\r\n") || strings.ContainsAny(input.Subject, "\r\n") {
			return nil, SendEmailOutput{}, fmt.Errorf("to and subject must not contain newline characters")
		}

		msg := fmt.Sprintf(
			"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"utf-8\"\r\n\r\n%s",
			cfg.fromEmail, input.To, input.Subject, input.Body,
		)

		addr := fmt.Sprintf("%s:%s", cfg.host, cfg.port)
		auth := smtp.PlainAuth("", cfg.username, cfg.password, cfg.host)
		if err := smtp.SendMail(addr, auth, cfg.fromEmail, []string{input.To}, []byte(msg)); err != nil {
			return nil, SendEmailOutput{}, fmt.Errorf("failed to send email: %w", err)
		}

		return nil, SendEmailOutput{Status: "sent"}, nil
	}
}

func main() {
	// 1. Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found; falling back to system environment variables")
	}

	// 2. Read SMTP config
	cfg := smtpConfig{
		host:     os.Getenv("SMTP_HOST"),
		port:     os.Getenv("SMTP_PORT"),
		username: os.Getenv("SMTP_USERNAME"),
		password: os.Getenv("SMTP_PASSWORD"),
	}
	if cfg.host == "" || cfg.port == "" || cfg.username == "" || cfg.password == "" {
		log.Fatal("SMTP_HOST, SMTP_PORT, SMTP_USERNAME, or SMTP_PASSWORD is not set in your .env file")
	}
	cfg.fromEmail = os.Getenv("SMTP_FROM_EMAIL")
	if cfg.fromEmail == "" {
		cfg.fromEmail = cfg.username
	}

	// 3. Build the MCP server and register the send_email tool
	server := mcp.NewServer(&mcp.Implementation{Name: "email-sender", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_email",
		Description: "Sends a plain-text email to a recipient over SMTP.",
	}, newSendEmailTool(cfg))

	// 4. Run over stdio
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
