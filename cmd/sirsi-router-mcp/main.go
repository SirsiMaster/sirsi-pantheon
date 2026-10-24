// Command sirsi-router-mcp is the legacy launcher for the shared router MCP core.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routermcp"
)

func run(ctx context.Context, in io.ReadCloser, out io.Writer) error {
	return routermcp.Run(ctx, in, out)
}

func main() {
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[sirsi-router-mcp] server: %v\n", err)
		os.Exit(1)
	}
}
