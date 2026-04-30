package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/issam/ctxd/internal/graph"
)

func runGraph(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ctxd graph <build|report|export|neighbors|path|stats> [PROJECT]")
	}
	cmd := args[0]
	st, err := open()
	if err != nil {
		return err
	}
	defer st.Close()

	// args[1] is optional project (name or path); if missing, resolve from CWD.
	projectArg := ""
	extraArgs := args[1:]
	if len(args) >= 2 {
		projectArg = args[1]
		extraArgs = args[2:]
	}
	project, err := resolveProject(ctx, st, projectArg)
	if err != nil {
		return err
	}

	switch cmd {
	case "build":
		stats, err := graph.Rebuild(ctx, st, project)
		if err == nil {
			fmt.Fprintf(stdout, "built graph: %d files, %d symbols, %d edges\n", stats.Files, stats.Symbols, stats.Edges)
		}
		return err
	case "report":
		path, err := graph.WriteReport(ctx, st, project)
		if err == nil {
			fmt.Fprintf(stdout, "wrote %s\n", path)
		}
		return err
	case "export":
		fs := flag.NewFlagSet("graph export", flag.ContinueOnError)
		fs.SetOutput(stderr)
		format := fs.String("format", "json", "json|html")
		if err := fs.Parse(extraArgs); err != nil {
			return err
		}
		var path string
		if *format == "html" {
			path, err = graph.ExportHTML(ctx, st, project)
		} else if *format == "json" {
			path, err = graph.ExportJSON(ctx, st, project)
		} else {
			return fmt.Errorf("unknown graph export format %q", *format)
		}
		if err == nil {
			fmt.Fprintf(stdout, "wrote %s\n", path)
		}
		return err
	case "neighbors":
		fs := flag.NewFlagSet("graph neighbors", flag.ContinueOnError)
		fs.SetOutput(stderr)
		depth := fs.Int("depth", 1, "neighbor depth")
		asJSON := fs.Bool("json", false, "json output")
		if err := fs.Parse(extraArgs); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: ctxd graph neighbors [PROJECT] SYMBOL_OR_FILE")
		}
		nodes, edges, err := graph.Neighbors(ctx, st, project, fs.Arg(0), *depth)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(stdout).Encode(map[string]any{"nodes": nodes, "edges": edges})
		}
		return printNodes(stdout, nodes, edges)
	case "path":
		asJSON := false
		if len(extraArgs) > 2 && extraArgs[2] == "--json" {
			asJSON = true
		}
		if len(extraArgs) < 2 {
			return fmt.Errorf("usage: ctxd graph path [PROJECT] FROM TO")
		}
		nodes, edges, err := graph.Path(ctx, st, project, extraArgs[0], extraArgs[1])
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(stdout).Encode(map[string]any{"nodes": nodes, "edges": edges})
		}
		for i, n := range nodes {
			if i > 0 {
				fmt.Fprint(stdout, " -> ")
			}
			fmt.Fprintf(stdout, "%s(%s)", n.Name, n.Type)
		}
		fmt.Fprintln(stdout)
		return nil
	case "stats":
		asJSON := len(extraArgs) > 0 && extraArgs[0] == "--json"
		stats, err := graph.ProjectStats(ctx, st, project)
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(stdout).Encode(stats)
		}
		fmt.Fprintf(stdout, "files: %d\nsymbols: %d\nedges: %d\n", stats.Files, stats.Symbols, stats.Edges)
		return nil
	default:
		return fmt.Errorf("unknown graph command %q", cmd)
	}
}

func printNodes(w io.Writer, nodes []graph.Node, edges []graph.Edge) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TYPE\tNAME\tFILE\tLINE")
	for _, n := range nodes {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", n.Type, n.Name, n.FilePath, n.StartLine)
	}
	fmt.Fprintf(tw, "\n%d edge(s)\n", len(edges))
	return tw.Flush()
}
