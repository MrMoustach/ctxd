package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/issam/ctxd/internal/config"
	"github.com/issam/ctxd/internal/contextpack"
	"github.com/issam/ctxd/internal/graph"
	"github.com/issam/ctxd/internal/indexer"
	"github.com/issam/ctxd/internal/install"
	"github.com/issam/ctxd/internal/mcp"
	"github.com/issam/ctxd/internal/output"
	"github.com/issam/ctxd/internal/search"
	"github.com/issam/ctxd/internal/store"
	"github.com/issam/ctxd/internal/summary"
	"github.com/issam/ctxd/internal/tui"
)

func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return runInteractive(stdout, stderr)
	}
	ctx := context.Background()
	switch args[0] {
	case "setup":
		fs := flag.NewFlagSet("setup", flag.ContinueOnError)
		fs.SetOutput(stderr)
		name := fs.String("name", "", "project name")
		agentsFlag := fs.String("agents", "", "comma-separated agents (claude,codex,copilot,antigravity)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		path := fs.Arg(0)
		if path == "" {
			var err error
			path, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		var agentList []string
		if *agentsFlag != "" {
			agentList = strings.Split(*agentsFlag, ",")
		}
		resolvedName, resolvedAgents, err := tui.CollectSetupParams(path, *name, func() []string {
			if *agentsFlag != "" {
				return agentList
			}
			return nil
		}())
		if err != nil {
			return err
		}
		return runSetup(ctx, stdout, path, resolvedName, resolvedAgents)
	case "init":
		cfg, err := config.Init()
		if err == nil {
			fmt.Fprintf(stdout, "initialized ctx database at %s\n", cfg.DBPath)
		}
		return err
	case "add":
		fs := flag.NewFlagSet("add", flag.ContinueOnError)
		fs.SetOutput(stderr)
		name := fs.String("name", "", "project name")
		addArgs := reorderAddArgs(args[1:])
		if err := fs.Parse(addArgs); err != nil {
			return err
		}
		if *name == "" || fs.NArg() != 1 {
			return fmt.Errorf("usage: ctx add /path/to/project --name NAME")
		}
		st, err := open()
		if err != nil {
			return err
		}
		defer st.Close()
		p, err := st.AddProject(ctx, *name, fs.Arg(0))
		if err == nil {
			fmt.Fprintf(stdout, "added %s -> %s\n", p.Name, p.RootPath)
		}
		return err
	case "projects":
		fs := flag.NewFlagSet("projects", flag.ContinueOnError)
		fs.SetOutput(stderr)
		asJSON := fs.Bool("json", false, "json output")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		st, err := open()
		if err != nil {
			return err
		}
		defer st.Close()
		ps, err := st.Projects(ctx)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(stdout).Encode(map[string]any{"projects": ps})
		}
		for _, p := range ps {
			fmt.Fprintf(stdout, "%s\t%s\n", p.Name, p.RootPath)
		}
		return nil
	case "index":
		st, err := open()
		if err != nil {
			return err
		}
		defer st.Close()
		projectArg := ""
		if len(args) >= 2 {
			projectArg = args[1]
		}
		p, err := resolveProject(ctx, st, projectArg)
		if err != nil {
			return err
		}
		r, err := indexer.IndexProject(ctx, st, p)
		if err == nil {
			fmt.Fprintf(stdout, "indexed %d files, %d chunks\n", r.IndexedFiles, r.IndexedChunks)
		}
		if err != nil {
			return err
		}
		gs, gErr := graph.Rebuild(ctx, st, p)
		if gErr == nil {
			fmt.Fprintf(stdout, "built graph: %d symbols, %d edges\n", gs.Symbols, gs.Edges)
		}
		return gErr
	case "search":
		fs := flag.NewFlagSet("search", flag.ContinueOnError)
		fs.SetOutput(stderr)
		limit := fs.Int("limit", 20, "raw FTS fetch limit")
		asJSON := fs.Bool("json", false, "json output")
		mode := fs.String("mode", output.ModeCompact, "output mode: compact|raw|summary")
		maxResults := fs.Int("max-results", 5, "compact/summary: max results")
		maxFiles := fs.Int("max-files", 3, "compact/summary: max files")
		maxLines := fs.Int("max-lines", 40, "compact: max lines per result")
		maxChars := fs.Int("max-chars", 12000, "compact: max total chars")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: ctx search [PROJECT] QUERY")
		}
		st, err := open()
		if err != nil {
			return err
		}
		defer st.Close()
		var projectArg, query string
		if fs.NArg() == 1 {
			query = fs.Arg(0)
		} else {
			projectArg = fs.Arg(0)
			query = fs.Arg(1)
		}
		p, err := resolveProject(ctx, st, projectArg)
		if err != nil {
			return err
		}
		rs, err := search.Search(ctx, st, p, query, *limit)
		if err != nil {
			return err
		}
		switch *mode {
		case output.ModeRaw:
			if *asJSON {
				return json.NewEncoder(stdout).Encode(map[string]any{"results": rs})
			}
			for _, r := range rs {
				fmt.Fprintf(stdout, "%s:%d-%d score=%.2f reason=%s\n%s\n\n", r.Path, r.StartLine, r.EndLine, r.Score, r.Reason, trim(r.Snippet, 900))
			}
		case output.ModeSummary:
			opts := output.DefaultCompactOptions()
			opts.MaxResults = *maxResults
			opts.MaxFiles = *maxFiles
			co := output.ApplyCompact(rs, opts)
			seen := map[string]bool{}
			var summaries []summary.FileSummary
			terms := search.Terms(query)
			for _, r := range co.Results {
				if seen[r.Path] {
					continue
				}
				seen[r.Path] = true
				content, err := st.FileContent(ctx, p, r.Path)
				if err != nil {
					continue
				}
				lang := output.LangFromPath(r.Path)
				summaries = append(summaries, summary.Summarize(r.Path, content, lang, terms))
			}
			if *asJSON {
				return json.NewEncoder(stdout).Encode(map[string]any{"summaries": summaries})
			}
			fmt.Fprint(stdout, summary.FormatAll(summaries))
		default: // compact
			opts := output.CompactOptions{
				MaxResults:        *maxResults,
				MaxFiles:          *maxFiles,
				MaxLinesPerResult: *maxLines,
				MaxTotalChars:     *maxChars,
			}
			co := output.ApplyCompact(rs, opts)
			if *asJSON {
				return json.NewEncoder(stdout).Encode(co)
			}
			fmt.Fprint(stdout, output.FormatCompact(co))
		}
		return nil
	case "expand":
		fs := flag.NewFlagSet("expand", flag.ContinueOnError)
		fs.SetOutput(stderr)
		around := fs.String("around", "", "symbol or keyword to center the window on")
		lines := fs.Int("lines", 120, "number of lines to return")
		asJSON := fs.Bool("json", false, "json output")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: ctx expand [PROJECT] FILE [--around SYMBOL] [--lines N]")
		}
		st, err := open()
		if err != nil {
			return err
		}
		defer st.Close()
		var expandProjectArg, expandFile string
		if fs.NArg() == 1 {
			expandFile = fs.Arg(0)
		} else {
			expandProjectArg = fs.Arg(0)
			expandFile = fs.Arg(1)
		}
		p, err := resolveProject(ctx, st, expandProjectArg)
		if err != nil {
			return err
		}
		content, err := st.FileContent(ctx, p, expandFile)
		if err != nil {
			return err
		}
		snippet, startLine := expandSnippet(content, *around, *lines)
		if *asJSON {
			return json.NewEncoder(stdout).Encode(map[string]any{
				"path":       expandFile,
				"start_line": startLine,
				"snippet":    snippet,
			})
		}
		fmt.Fprintf(stdout, "%s:%d\n\n%s\n", expandFile, startLine, snippet)
		return nil
	case "context":
		fs := flag.NewFlagSet("context", flag.ContinueOnError)
		fs.SetOutput(stderr)
		maxTokens := fs.Int("max-tokens", 12000, "token budget")
		useGraph := fs.Bool("graph", false, "enable graph-enhanced context")
		graphDepth := fs.Int("graph-depth", 1, "graph expansion depth")
		asJSON := fs.Bool("json", false, "json output")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return fmt.Errorf("usage: ctx context [PROJECT] TASK --max-tokens 12000")
		}
		st, err := open()
		if err != nil {
			return err
		}
		defer st.Close()
		var ctxProjectArg, task string
		if fs.NArg() == 1 {
			task = fs.Arg(0)
		} else {
			ctxProjectArg = fs.Arg(0)
			task = fs.Arg(1)
		}
		p, err := resolveProject(ctx, st, ctxProjectArg)
		if err != nil {
			return err
		}
		graphEnabled := *useGraph || graph.HasGraphData(ctx, st, p.ID)
		md, _, err := contextpack.BuildWithOptions(ctx, st, p, task, contextpack.Options{MaxTokens: *maxTokens, Graph: graphEnabled, GraphDepth: *graphDepth})
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(stdout).Encode(map[string]any{"markdown": md})
		}
		fmt.Fprint(stdout, md)
		return nil
	case "graph":
		return runGraph(ctx, args[1:], stdout, stderr)
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		fs.SetOutput(stderr)
		mcpFlag := fs.Bool("mcp", false, "serve MCP over stdio")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if !*mcpFlag {
			return fmt.Errorf("only ctx serve --mcp is implemented in v1")
		}
		st, err := open()
		if err != nil {
			return err
		}
		defer st.Close()
		return mcp.Serve(ctx, st, os.Stdin, stdout)
	case "install":
		if len(args) < 2 {
			return fmt.Errorf("usage: ctxd install <claude|codex|copilot|antigravity|all>")
		}
		binPath, err := resolvedBinary()
		if err != nil {
			return err
		}
		return install.Run(args[1], binPath, stdout)
	case "analytics":
		fs := flag.NewFlagSet("analytics", flag.ContinueOnError)
		fs.SetOutput(stderr)
		filterTool := fs.String("tool", "", "filter by tool name")
		filterProject := fs.String("project", "", "filter by project name")
		filterSince := fs.String("since", "", "since date YYYY-MM-DD")
		filterUntil := fs.String("until", "", "until date YYYY-MM-DD")
		filterMinTok := fs.Int("min-tokens", 0, "min actual tokens")
		filterMaxTok := fs.Int("max-tokens", 0, "max actual tokens")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		st, err := open()
		if err != nil {
			return err
		}
		defer st.Close()
		var f store.AnalyticsFilter
		f.Tool = *filterTool
		f.Project = *filterProject
		f.MinTokens = *filterMinTok
		f.MaxTokens = *filterMaxTok
		if *filterSince != "" {
			f.Since, err = time.Parse("2006-01-02", *filterSince)
			if err != nil {
				return fmt.Errorf("invalid --since date: %w", err)
			}
		}
		if *filterUntil != "" {
			f.Until, err = time.Parse("2006-01-02", *filterUntil)
			if err != nil {
				return fmt.Errorf("invalid --until date: %w", err)
			}
			f.Until = f.Until.Add(24*time.Hour - time.Second)
		}
		records, err := st.QueryAnalytics(ctx, f)
		if err != nil {
			return err
		}
		return printAnalytics(stdout, records)
	case "doctor":
		binPath, err := resolvedBinary()
		if err != nil {
			return err
		}
		return install.Doctor(binPath, stdout)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		if _, err := strconv.Atoi(args[0]); err == nil {
			return fmt.Errorf("unknown command %q", args[0])
		}
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInteractive(stdout, stderr io.Writer) error {
	var projectNames []string
	if st, err := open(); err == nil {
		if ps, err := st.Projects(context.Background()); err == nil {
			for _, p := range ps {
				projectNames = append(projectNames, p.Name)
			}
		}
		st.Close()
	}
	args, err := tui.Collect(projectNames)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return nil
	}
	return Run(args, stdout, stderr)
}

// resolveProject resolves a project from a name, path, or CWD (when arg is empty).
// If arg looks like a path (absolute, relative, or "."), it resolves by path.
// Otherwise it resolves by name.
func resolveProject(ctx context.Context, st *store.Store, arg string) (store.Project, error) {
	if isPathArg(arg) {
		abs := arg
		if arg == "" {
			var err error
			abs, err = os.Getwd()
			if err != nil {
				return store.Project{}, err
			}
		} else {
			var err error
			abs, err = filepath.Abs(arg)
			if err != nil {
				return store.Project{}, err
			}
		}
		return st.ProjectByPath(ctx, abs)
	}
	return st.ProjectByName(ctx, arg)
}

func isPathArg(s string) bool {
	return s == "" || s == "." || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")
}

func open() (*store.Store, error) {
	db, err := config.DefaultDBPath()
	if err != nil {
		return nil, err
	}
	return store.Open(db)
}

func resolvedBinary() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine binary path: %w", err)
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("cannot resolve binary path: %w", err)
	}
	return p, nil
}

func runSetup(ctx context.Context, stdout io.Writer, path, name string, agents []string) error {
	// Step 1: init if DB does not yet exist
	dbPath, err := config.DefaultDBPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		cfg, err := config.Init()
		if err != nil {
			return fmt.Errorf("init: %w", err)
		}
		fmt.Fprintf(stdout, "initialized ctx database at %s\n", cfg.DBPath)
	} else {
		fmt.Fprintf(stdout, "database already exists at %s\n", dbPath)
	}

	// Step 2: add project
	st, err := open()
	if err != nil {
		return err
	}
	defer st.Close()
	p, err := st.AddProject(ctx, name, path)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	fmt.Fprintf(stdout, "added %s -> %s\n", p.Name, p.RootPath)

	// Step 3: index
	r, err := indexer.IndexProject(ctx, st, p)
	if err != nil {
		return fmt.Errorf("index: %w", err)
	}
	fmt.Fprintf(stdout, "indexed %d files, %d chunks\n", r.IndexedFiles, r.IndexedChunks)

	// Step 4: build graph
	gs, gErr := graph.Rebuild(ctx, st, p)
	if gErr != nil {
		return fmt.Errorf("graph: %w", gErr)
	}
	fmt.Fprintf(stdout, "built graph: %d symbols, %d edges\n", gs.Symbols, gs.Edges)

	// Step 5: install agents
	if len(agents) > 0 {
		binPath, err := resolvedBinary()
		if err != nil {
			return err
		}
		for _, agent := range agents {
			agent = strings.TrimSpace(agent)
			if agent == "" {
				continue
			}
			if err := install.Run(agent, binPath, stdout); err != nil {
				fmt.Fprintf(stdout, "warning: install %s: %v\n", agent, err)
			}
		}
	}

	return nil
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: ctxd <setup|init|add|projects|index|search|context|graph|serve|install|analytics|doctor>")
}

// expandSnippet returns up to maxLines lines from content.
// If around is non-empty, it centers the window on the first matching line.
// Hard cap: 50 000 chars.
func expandSnippet(content, around string, maxLines int) (snippet string, startLine int) {
	const maxChars = 50000
	lines := strings.Split(content, "\n")
	center := 0
	if around != "" {
		lower := strings.ToLower(around)
		for i, l := range lines {
			if strings.Contains(strings.ToLower(l), lower) {
				center = i
				break
			}
		}
	}
	half := maxLines / 2
	start := center - half
	if start < 0 {
		start = 0
	}
	end := start + maxLines
	if end > len(lines) {
		end = len(lines)
	}
	text := strings.Join(lines[start:end], "\n")
	if len(text) > maxChars {
		text = text[:maxChars] + "\n..."
	}
	return text, start + 1
}

func printAnalytics(w io.Writer, records []store.AnalyticsRecord) error {
	if len(records) == 0 {
		fmt.Fprintln(w, "no analytics records found")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tTOOL\tPROJECT\tWITHOUT\tACTUAL\tSAVINGS\tSAVED%")
	fmt.Fprintln(tw, "----\t----\t-------\t-------\t------\t-------\t------")
	var totalWithout, totalActual int
	for _, r := range records {
		savings := r.TokensWithout - r.TokensActual
		var pct float64
		if r.TokensWithout > 0 {
			pct = float64(savings) / float64(r.TokensWithout) * 100
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%.0f%%\n",
			r.CalledAt.Local().Format("01-02 15:04"),
			r.Tool,
			r.Project,
			r.TokensWithout,
			r.TokensActual,
			savings,
			pct,
		)
		totalWithout += r.TokensWithout
		totalActual += r.TokensActual
	}
	fmt.Fprintln(tw, "----\t----\t-------\t-------\t------\t-------\t------")
	totalSavings := totalWithout - totalActual
	var totalPct float64
	if totalWithout > 0 {
		totalPct = float64(totalSavings) / float64(totalWithout) * 100
	}
	fmt.Fprintf(tw, "TOTAL (%d)\t\t\t%d\t%d\t%d\t%.0f%%\n",
		len(records), totalWithout, totalActual, totalSavings, totalPct)
	return tw.Flush()
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n..."
}

func reorderAddArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--name" || args[i] == "-name" {
			flags = append(flags, args[i])
			if i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		if len(args[i]) > 7 && args[i][:7] == "--name=" {
			flags = append(flags, args[i])
			continue
		}
		positional = append(positional, args[i])
	}
	return append(flags, positional...)
}
