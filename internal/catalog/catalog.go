// Package catalog is the shared command reference for help, completion and agent instructions.
package catalog

type Command struct {
	Name        string `json:"name"`
	Usage       string `json:"usage"`
	Description string `json:"description"`
}

type Tool struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Commands    []Command `json:"commands"`
	Notes       []string  `json:"notes"`
}

func Tools() []Tool {
	return []Tool{
		{"dev", "Project development actions", []Command{
			{"list", "dev list --json", "List project actions, descriptions and resolution status"},
			{"explain", "dev explain ACTION --json", "Inspect an action's command and directory without executing it"},
		}, []string{"Use explicit actions such as dev test or dev build in automation. Bare dev opens a picker in a terminal.", "Pass runner arguments after --. Resolve ambiguous actions with project .worktools.toml mappings; do not guess a runner."}},
		{"pod", "Containers and Compose projects", []Command{
			{"providers", "pod providers --json", "Inspect installed container backends and Compose support"},
			{"actions", "pod actions --json", "Inspect operations supported by the selected backend"},
			{"explain", "pod explain up --json", "Inspect backend, scope and command without executing it"},
			{"services", "pod services --json", "List configured Compose service names"},
			{"containers", "pod containers --json", "List container summaries across projects in the selected engine/context"},
			{"ps", "pod ps", "List this Compose project's containers"},
			{"up", "pod up --detach", "Start project services in the background"},
			{"down", "pod down", "Stop and remove project containers and networks"},
			{"logs", "pod logs --follow", "Follow project service logs"},
			{"exec", "pod exec SERVICE -- COMMAND", "Run a command inside a Compose service"},
			{"restart", "pod restart SERVICE", "Restart project services"},
			{"build", "pod build", "Build project service images"},
		}, []string{"Project ps and engine-wide containers have different scopes. Use explain to check the selected backend before changing services.", "Provider selection: --provider, WORKTOOLS_POD_PROVIDER, project [pod] provider, personal default, then the sole installed backend. Native contexts are inherited; Worktools does not start engines or switch contexts automatically."}},
		{"svc", "Services and timers", []Command{
			{"list", "svc list --json", "List services"},
			{"status", "svc status UNIT --json", "Inspect a unit"},
			{"start", "svc start UNIT", "Start a unit"},
			{"stop", "svc stop UNIT", "Stop a unit"},
			{"restart", "svc restart UNIT", "Restart a unit"},
			{"logs", "svc logs UNIT", "Read unit logs"},
			{"timers", "svc timers --json", "List timers"},
			{"failed", "svc failed --json", "List failed units"},
		}, []string{"Defaults to user systemd units. --system explicitly selects system units; service mutations retain native systemd authorization.", "Use svc list --aliases --json for offline alias inspection. Log --json uses JSON lines."}},
		{"db", "Project database connections", []Command{
			{"list", "db list --json", "List configured connections"},
			{"info", "db info NAME --json", "Inspect connection metadata without connecting"},
			{"connect", "db connect NAME", "Open the native database client"},
			{"query", "db query NAME --file FILE.sql", "Execute a SQL file"},
		}, []string{"Connections are explicitly declared in .db.json. Credentials come from exported environment variables; Worktools does not source .env or include credentials in inspection.", "SQL files run as supplied and may modify data. Worktools does not infer read-only SQL or run migrations automatically."}},
		{"dots", "Dotfiles inspection and editing", []Command{
			{"list", "dots list --json", "List configuration paths"},
			{"show", "dots show NAME --json", "Inspect a configuration and its actual symlink source"},
			{"edit", "dots edit NAME", "Edit the actual configuration source"},
			{"check", "dots check --json", "Check links and supported configuration syntax"},
			{"diff", "dots diff", "Show tracked dotfiles changes"},
		}, []string{"DOTFILES selects the checkout, default ~/dotfiles. Actual symlink sources can point elsewhere; inspect before editing. No automatic relinking occurs."}},
		{"agent", "Project instruction onboarding", []Command{
			{"init", "worktools agent init", "Choose tools and instruction files, preview changes, then confirm"},
			{"preview", "worktools agent preview", "Preview managed instruction updates without writing"},
			{"sync", "worktools agent sync", "Update selected managed instruction sections"},
		}, nil},
	}
}

func Get(name string) (Tool, bool) {
	for _, tool := range Tools() {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

func Selectable() []Tool {
	result := []Tool{}
	for _, tool := range Tools() {
		if tool.Name != "agent" {
			result = append(result, tool)
		}
	}
	return result
}
