// Knowledge Engine — MCP server over stdio.
//
// Add it to an MCP client as a command, for example:
//
//	{
//	  "mcpServers": {
//	    "knowledge": {
//	      "command": "knowledge-mcp",
//	      "env": {"DATABASE_URL": "postgres://...", "MCP_USER_ID": "..."}
//	    }
//	  }
//	}
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"

	knowledge "github.com/omnidev/knowledge-engine"
	"github.com/omnidev/knowledge-engine/cmd/internal/envconfig"
	"github.com/omnidev/knowledge-engine/transport/mcp"
)

func main() {
	cfg := envconfig.Load()

	userID := envconfig.GetString("MCP_USER_ID", envconfig.GetString("DEFAULT_USER_ID", knowledge.DefaultUserID))
	if _, err := uuid.Parse(userID); err != nil {
		fmt.Fprintf(os.Stderr, "MCP_USER_ID must be a UUID: %v\n", err)
		os.Exit(1)
	}

	// stdio clients supply the identity through the environment instead of a token.
	cfg.Auth = knowledge.AuthConfig{Type: "noop", DefaultUserID: userID}
	// stdout is reserved for JSON-RPC messages.
	cfg.Logger = stderrLogger{}

	engine, err := knowledge.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create engine: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()

	server := mcp.NewServer(engine.KnowledgeBaseService(), engine.SearchService())
	if err := server.ServeStdio(context.Background(), uuid.MustParse(userID), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

type stderrLogger struct{}

func (stderrLogger) Info(msg string, kv ...any)  { fmt.Fprintf(os.Stderr, "[INFO] %s %v\n", msg, kv) }
func (stderrLogger) Warn(msg string, kv ...any)  { fmt.Fprintf(os.Stderr, "[WARN] %s %v\n", msg, kv) }
func (stderrLogger) Error(msg string, kv ...any) { fmt.Fprintf(os.Stderr, "[ERROR] %s %v\n", msg, kv) }
func (stderrLogger) Debug(string, ...any)        {}
