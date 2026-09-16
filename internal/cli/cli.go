// Package cli exposes the portal client as commands mirroring the MCP tools
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kiliankoe/opendatadresdenmcp/internal/portal"
)

const usage = `Usage:
  dresden-opendata-mcp                              run the MCP server on stdio
  dresden-opendata-mcp search [flags] [query...]    search datasets, no query lists all
  dresden-opendata-mcp info <id>                    show a dataset with all its resources
  dresden-opendata-mcp fetch [flags] <id> <format>  download a dataset resource

Flags of search: --limit N (default 30), --offset N
Flags of fetch:  --limit N, --bbox minLon,minLat,maxLon,maxLat (geodata layers only)`

// Run executes one command and writes its result to stdout
func Run(ctx context.Context, client *portal.Client, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		args = []string{"help"}
	}
	err := runCommand(ctx, client, args[0], args[1:], stdout)
	if errors.Is(err, flag.ErrHelp) {
		_, err = fmt.Fprintln(stdout, usage)
	}
	return err
}

func runCommand(ctx context.Context, client *portal.Client, command string, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	switch command {
	case "search":
		limit := flags.Int("limit", 30, "")
		offset := flags.Int("offset", 0, "")
		if err := flags.Parse(args); err != nil {
			return err
		}
		result, err := client.SearchDatasets(ctx, strings.Join(flags.Args(), " "), *limit, *offset)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)

	case "info":
		if len(args) != 1 {
			return errors.New("usage: info <id>")
		}
		dataset, err := client.GetDataset(ctx, args[0])
		if err != nil {
			return err
		}
		return writeJSON(stdout, dataset)

	case "fetch":
		limit := flags.Int("limit", 0, "")
		bbox := flags.String("bbox", "", "")
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 2 {
			return errors.New("usage: fetch [flags] <id> <format>")
		}
		dataset, err := client.GetDataset(ctx, flags.Arg(0))
		if err != nil {
			return err
		}
		data, err := client.FetchResource(ctx, dataset, flags.Arg(1), portal.FetchOptions{Limit: *limit, BBox: *bbox})
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err

	case "help", "-h", "--help":
		return flag.ErrHelp

	default:
		return fmt.Errorf("unknown command %q\n%s", command, usage)
	}
}

func writeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
