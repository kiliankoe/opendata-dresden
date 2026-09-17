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
	"text/tabwriter"

	"github.com/kiliankoe/opendata-dresden/internal/config"
	"github.com/kiliankoe/opendata-dresden/internal/index"
	"github.com/kiliankoe/opendata-dresden/internal/portal"
)

const usage = `Usage:
  od3                             run the MCP server on stdio
  od3 search [flags] [query...]   search datasets, no query lists all
  od3 info <id>                   show a dataset with all its resources
  od3 fetch [flags] <id> <format> download a dataset resource
  od3 history [flags] <id>        show how a statistics table changed over time
  od3 index [flags]               snapshot all datasets with their descriptions
  od3 version                     print the version

Flags of search:  --limit N (default 30), --offset N, --output text|json
Flags of info:    --output text|json
Flags of fetch:   --limit N, --bbox minLon,minLat,maxLon,maxLat (geodata layers only)
Flags of history: --limit N (default 10), --diff shows the changed rows, --output text|json
Flags of index:   --file PATH (default data/index.json), --data DIR mirrors the tables`

// Run executes one command and writes its result to stdout
func Run(ctx context.Context, cfg *config.Config, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		args = []string{"help"}
	}
	err := runCommand(ctx, cfg, args[0], args[1:], stdout)
	if errors.Is(err, flag.ErrHelp) {
		_, err = fmt.Fprintln(stdout, usage)
	}
	return err
}

func runCommand(ctx context.Context, cfg *config.Config, command string, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	catalog := index.NewCatalog(cfg)

	switch command {
	case "search":
		limit := flags.Int("limit", 30, "")
		offset := flags.Int("offset", 0, "")
		output := flags.String("output", "text", "")
		if err := flags.Parse(args); err != nil {
			return err
		}
		write, err := writer(*output)
		if err != nil {
			return err
		}
		result, err := catalog.Search(ctx, strings.Join(flags.Args(), " "), *limit, *offset)
		if err != nil {
			return err
		}
		return write(stdout, result)

	case "info":
		output := flags.String("output", "text", "")
		if err := flags.Parse(args); err != nil {
			return err
		}
		write, err := writer(*output)
		if err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return errors.New("usage: info [flags] <id>")
		}
		dataset, err := catalog.Get(ctx, flags.Arg(0))
		if err != nil {
			return err
		}
		return write(stdout, dataset)

	case "fetch":
		limit := flags.Int("limit", 0, "")
		bbox := flags.String("bbox", "", "")
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 2 {
			return errors.New("usage: fetch [flags] <id> <format>")
		}
		data, err := catalog.Fetch(ctx, flags.Arg(0), flags.Arg(1), portal.FetchOptions{Limit: *limit, BBox: *bbox})
		if err != nil {
			return err
		}
		_, err = stdout.Write(data)
		return err

	case "history":
		limit := flags.Int("limit", 10, "")
		diff := flags.Bool("diff", false, "")
		output := flags.String("output", "text", "")
		if err := flags.Parse(args); err != nil {
			return err
		}
		write, err := writer(*output)
		if err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return errors.New("usage: history [flags] <id>")
		}
		history, err := catalog.History(ctx, flags.Arg(0), *limit, *diff)
		if err != nil {
			return err
		}
		return write(stdout, history)

	case "index":
		file := flags.String("file", "data/index.json", "")
		data := flags.String("data", "", "")
		if err := flags.Parse(args); err != nil {
			return err
		}
		previous, err := index.Read(*file)
		if err != nil {
			return err
		}
		client := portal.NewClient(cfg)
		idx, changed, err := index.Build(ctx, client, previous)
		if err != nil {
			return err
		}
		summary := fmt.Sprintf("%d datasets, %d new or changed", len(idx.Datasets), len(changed))
		if *data != "" {
			written, err := index.Mirror(ctx, client, idx, changed, *data)
			if err != nil {
				return err
			}
			summary += fmt.Sprintf(", %d mirrored", written)
		}
		// Mirroring dates the datasets whose tables moved, so it goes first
		if err := idx.Write(*file); err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, summary)
		return err

	case "version", "--version":
		_, err := fmt.Fprintln(stdout, "od3", config.Version)
		return err

	case "help", "-h", "--help":
		return flag.ErrHelp

	default:
		return fmt.Errorf("unknown command %q\n%s", command, usage)
	}
}

func writer(output string) (func(io.Writer, any) error, error) {
	switch output {
	case "text":
		return writeText, nil
	case "json":
		return writeJSON, nil
	default:
		return nil, fmt.Errorf("unknown output format %q, use text or json", output)
	}
}

func writeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func writeText(w io.Writer, v any) error {
	switch v := v.(type) {
	case *portal.SearchResult:
		return writeSearchText(w, v)
	case *portal.Dataset:
		return writeDatasetText(w, v)
	case *index.History:
		return writeHistoryText(w, v)
	default:
		return fmt.Errorf("no text format for %T", v)
	}
}

func writeSearchText(w io.Writer, result *portal.SearchResult) error {
	var b strings.Builder
	for _, ds := range result.Datasets {
		formats := make([]string, len(ds.Resources))
		for i, r := range ds.Resources {
			formats[i] = r.Format
		}
		fmt.Fprintf(&b, "%s\n  %s  updated %s  %s\n", ds.Title, ds.ID, ds.Updated, strings.Join(formats, ", "))
	}
	fmt.Fprintf(&b, "\n%d of %d datasets\n", len(result.Datasets), result.Total)
	_, err := io.WriteString(w, b.String())
	return err
}

func writeHistoryText(w io.Writer, h *index.History) error {
	var b strings.Builder
	fmt.Fprintln(&b, h.Title)
	if len(h.Changes) == 0 {
		fmt.Fprintln(&b, "no recorded changes yet")
	}
	for _, c := range h.Changes {
		fmt.Fprintf(&b, "%s  %.7s  %s\n", c.Date, c.Commit, c.Message)
		if c.Diff != "" {
			fmt.Fprintf(&b, "%s\n", c.Diff)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeDatasetText(w io.Writer, ds *portal.Dataset) error {
	fields := []struct{ label, value string }{
		{"ID", ds.ID},
		{"Updated", ds.Updated},
		{"Changed", ds.Changed},
		{"Source", ds.Source},
		{"License", ds.License},
		{"Topics", strings.Join(ds.Topics, ", ")},
		{"Regions", strings.Join(ds.Regions, ", ")},
		{"Years", strings.Join(ds.Years, ", ")},
		{"Origin", ds.Origin},
	}
	fmt.Fprintln(w, ds.Title)
	if ds.Description != "" {
		fmt.Fprintf(w, "%s\n", ds.Description)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, f := range fields {
		if f.value != "" {
			fmt.Fprintf(tw, "%s\t%s\n", f.label, f.value)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(w, "\nResources")
	for _, r := range ds.Resources {
		fmt.Fprintf(tw, "  %s\t%s\n", r.Format, r.URL)
	}
	return tw.Flush()
}
