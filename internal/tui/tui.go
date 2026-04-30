package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/issam/ctxd/internal/config"
)

// Collect shows an interactive menu and returns assembled CLI args ready for cli.Run.
// projectNames populates project selects; may be empty (falls back to text input).
// Returns nil, nil when the user quits or aborts.
func Collect(projectNames []string) ([]string, error) {
	var cmd string
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("ctxd").
			Description("What would you like to do?").
			Options(
				huh.NewOption("setup        init + add + index + install", "setup"),
				huh.NewOption("init         initialize the database", "init"),
				huh.NewOption("add          register a project", "add"),
				huh.NewOption("projects     list all projects", "projects"),
				huh.NewOption("index        index a project", "index"),
				huh.NewOption("search       search a project", "search"),
				huh.NewOption("context      build context for a task", "context"),
				huh.NewOption("graph        build graph index", "graph"),
				huh.NewOption("serve --mcp  start MCP stdio server", "serve"),
				huh.NewOption("install      install AI tool integrations", "install"),
				huh.NewOption("analytics    view token savings analytics", "analytics"),
				huh.NewOption("doctor       check integration health", "doctor"),
				huh.NewOption("quit", "quit"),
			).
			Value(&cmd),
	)).Run()
	if err != nil {
		return nil, abort(err)
	}

	switch cmd {
	case "quit":
		return nil, nil
	case "setup":
		return collectSetup()
	case "init":
		return collectInit()
	case "add":
		return collectAdd()
	case "projects":
		return []string{"projects"}, nil
	case "index":
		return collectSingleProject("index", projectNames)
	case "search":
		return collectSearch(projectNames)
	case "context":
		return collectContext(projectNames)
	case "graph":
		return collectGraph(projectNames)
	case "serve":
		return []string{"serve", "--mcp"}, nil
	case "install":
		return collectInstall()
	case "analytics":
		return collectAnalytics(projectNames)
	case "doctor":
		return []string{"doctor"}, nil
	}
	return nil, nil
}

func collectAnalytics(projectNames []string) ([]string, error) {
	tools := []string{"", "ctxd_project_map", "ctxd_search", "ctxd_context", "ctxd_read_files", "ctxd_graph_neighbors", "ctxd_graph_path", "ctxd_graph_rebuild", "ctxd_graph_stats", "ctxd_graph_report", "reindex_project"}
	toolOpts := make([]huh.Option[string], len(tools))
	toolOpts[0] = huh.NewOption("all tools", "")
	for i, t := range tools[1:] {
		toolOpts[i+1] = huh.NewOption(t, t)
	}

	var tool string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Filter by tool").
			Options(toolOpts...).
			Value(&tool),
	)).Run(); err != nil {
		return nil, abort(err)
	}

	var project string
	if len(projectNames) > 0 {
		projOpts := make([]huh.Option[string], len(projectNames)+1)
		projOpts[0] = huh.NewOption("all projects", "")
		for i, n := range projectNames {
			projOpts[i+1] = huh.NewOption(n, n)
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Filter by project").
				Options(projOpts...).
				Value(&project),
		)).Run(); err != nil {
			return nil, abort(err)
		}
	}

	var since, until, minTok, maxTok string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Since date (YYYY-MM-DD, blank = no limit)").Value(&since),
		huh.NewInput().Title("Until date (YYYY-MM-DD, blank = no limit)").Value(&until),
		huh.NewInput().Title("Min actual tokens (blank = no limit)").Value(&minTok),
		huh.NewInput().Title("Max actual tokens (blank = no limit)").Value(&maxTok),
	)).Run(); err != nil {
		return nil, abort(err)
	}

	args := []string{"analytics"}
	if tool != "" {
		args = append(args, "--tool", tool)
	}
	if project != "" {
		args = append(args, "--project", project)
	}
	if since != "" {
		args = append(args, "--since", since)
	}
	if until != "" {
		args = append(args, "--until", until)
	}
	if minTok != "" {
		args = append(args, "--min-tokens", minTok)
	}
	if maxTok != "" {
		args = append(args, "--max-tokens", maxTok)
	}
	return args, nil
}

func collectInit() ([]string, error) {
	dbPath, _ := config.DefaultDBPath()

	var confirm bool
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Initialize database").
			Description(fmt.Sprintf("Will create: %s", dbPath)).
			Affirmative("Initialize").
			Negative("Cancel").
			Value(&confirm),
	)).Run()
	if err != nil {
		return nil, abort(err)
	}
	if !confirm {
		return nil, nil
	}
	return []string{"init"}, nil
}

func collectAdd() ([]string, error) {
	cwd, _ := os.Getwd()

	// Step 1: path
	path := cwd
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Project path").
			Description("Absolute path to the project root").
			Value(&path).
			Validate(func(s string) error {
				if s == "" {
					s = cwd
				}
				if _, err := os.Stat(s); err != nil {
					return fmt.Errorf("path does not exist")
				}
				return nil
			}),
	)).Run()
	if err != nil {
		return nil, abort(err)
	}
	if path == "" {
		path = cwd
	}

	// Step 2: name derived from path
	name := filepath.Base(path)
	err = huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Project name").
			Description(fmt.Sprintf("Short identifier for %s", path)).
			Value(&name),
	)).Run()
	if err != nil {
		return nil, abort(err)
	}
	if name == "" {
		name = filepath.Base(path)
	}

	return []string{"add", path, "--name", name}, nil
}

func collectSingleProject(cmd string, projectNames []string) ([]string, error) {
	proj, err := pickProject("Project", projectNames)
	if err != nil {
		return nil, err
	}
	if proj == "" {
		return nil, nil
	}
	return []string{cmd, proj}, nil
}

func collectGraph(projectNames []string) ([]string, error) {
	proj, err := pickProject("Project", projectNames)
	if err != nil || proj == "" {
		return nil, err
	}
	return []string{"graph", "build", proj}, nil
}

func collectSearch(projectNames []string) ([]string, error) {
	proj, err := pickProject("Project to search", projectNames)
	if err != nil || proj == "" {
		return nil, err
	}

	var query string
	limit := "10"
	err = huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Search query").
			Value(&query).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("query is required")
				}
				return nil
			}),
		huh.NewInput().
			Title("Result limit").
			Value(&limit),
	)).Run()
	if err != nil {
		return nil, abort(err)
	}

	return []string{"search", proj, query, "--limit", limit}, nil
}

func collectContext(projectNames []string) ([]string, error) {
	proj, err := pickProject("Project", projectNames)
	if err != nil || proj == "" {
		return nil, err
	}

	var task string
	maxTokens := "12000"
	err = huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Task description").
			Description("Describe what you're implementing or investigating").
			Value(&task).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("task description is required")
				}
				return nil
			}),
		huh.NewInput().
			Title("Max tokens").
			Value(&maxTokens),
	)).Run()
	if err != nil {
		return nil, abort(err)
	}

	return []string{"context", proj, task, "--max-tokens", maxTokens}, nil
}

func collectInstall() ([]string, error) {
	var target string
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Install target").
			Options(
				huh.NewOption("all          all integrations", "all"),
				huh.NewOption("claude       Claude Code", "claude"),
				huh.NewOption("codex        OpenAI Codex", "codex"),
				huh.NewOption("copilot      GitHub Copilot / VS Code", "copilot"),
				huh.NewOption("antigravity  Antigravity", "antigravity"),
			).
			Value(&target),
	)).Run()
	if err != nil {
		return nil, abort(err)
	}
	return []string{"install", target}, nil
}

// pickProject shows a Select if names are available, otherwise a text Input.
func pickProject(label string, projectNames []string) (string, error) {
	if len(projectNames) > 0 {
		var proj string
		opts := make([]huh.Option[string], len(projectNames))
		for i, n := range projectNames {
			opts[i] = huh.NewOption(n, n)
		}
		err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title(label).
				Options(opts...).
				Value(&proj),
		)).Run()
		if err != nil {
			return "", abort(err)
		}
		return proj, nil
	}

	var proj string
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title(label).
			Description("No indexed projects found — enter name manually").
			Value(&proj).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("project name is required")
				}
				return nil
			}),
	)).Run()
	if err != nil {
		return "", abort(err)
	}
	return proj, nil
}

func collectSetup() ([]string, error) {
	cwd, _ := os.Getwd()
	path := cwd
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Project path").
			Description("Absolute path to the project root").
			Value(&path).
			Validate(func(s string) error {
				if s == "" {
					s = cwd
				}
				if _, err := os.Stat(s); err != nil {
					return fmt.Errorf("path does not exist")
				}
				return nil
			}),
	)).Run()
	if err != nil {
		return nil, abort(err)
	}
	if path == "" {
		path = cwd
	}

	name, agents, err := CollectSetupParams(path, "", nil)
	if err != nil {
		return nil, err
	}

	args := []string{"setup", path, "--name", name}
	if len(agents) > 0 {
		args = append(args, "--agents", strings.Join(agents, ","))
	}
	return args, nil
}

// CollectSetupParams prompts for any missing setup parameters.
// Pass empty name to prompt for it; pass nil agents to prompt for them.
func CollectSetupParams(path, name string, agents []string) (string, []string, error) {
	if name == "" {
		dflt := filepath.Base(path)
		n := dflt
		err := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Description(fmt.Sprintf("Short identifier for %s", path)).
				Value(&n),
		)).Run()
		if err != nil {
			return "", nil, abort(err)
		}
		if n == "" {
			n = dflt
		}
		name = n
	}

	if agents == nil {
		var claude, codex, copilot, antigravity bool
		err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title("Install Claude Code integration?").Value(&claude),
			huh.NewConfirm().Title("Install OpenAI Codex integration?").Value(&codex),
			huh.NewConfirm().Title("Install GitHub Copilot / VS Code integration?").Value(&copilot),
			huh.NewConfirm().Title("Install Antigravity integration?").Value(&antigravity),
		)).Run()
		if err != nil {
			return "", nil, abort(err)
		}
		if claude {
			agents = append(agents, "claude")
		}
		if codex {
			agents = append(agents, "codex")
		}
		if copilot {
			agents = append(agents, "copilot")
		}
		if antigravity {
			agents = append(agents, "antigravity")
		}
	}

	return name, agents, nil
}

// abort converts huh.ErrUserAborted (Ctrl+C / Esc) to a clean nil error.
func abort(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return nil
	}
	return err
}
