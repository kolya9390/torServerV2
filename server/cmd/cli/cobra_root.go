package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Execute runs the CLI with the given arguments.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	opts := &globalOptions{}

	root := &cobra.Command{
		Use:           "torrserver",
		Short:         "TorrServer — торрент стриминг сервер и CLI",
		SilenceUsage:  true,
		SilenceErrors: false,
		Long:          "TorrServer — минималистичный сервер для стриминга торрентов.\n\nБез аргументов запускает сервер.\nС аргументами работает как CLI для управления.",
		Example: strings.Join([]string{
			"  torrserver                                        # Запуск сервера",
			"  torrserver status                                 # Проверить статус",
			"  torrserver torrents list                          # Список торрентов",
			"  torrserver torrents add --link 'magnet:...'       # Добавить торрент",
			"  torrserver settings get                           # Получить настройки",
			"  torrserver settings set CacheSize 128MB           # Изменить кэш",
		}, "\n"),
	}

	root.PersistentFlags().StringVar(
		&opts.Context,
		"context",
		strings.TrimSpace(os.Getenv("TSCTL_CONTEXT")),
		"context name",
	)
	root.PersistentFlags().StringVar(&opts.Server, "server", "", "base server URL (overrides context)")
	root.PersistentFlags().StringVar(&opts.User, "user", "", "basic auth username")
	root.PersistentFlags().StringVar(&opts.Pass, "pass", "", "basic auth password")
	root.PersistentFlags().StringVar(&opts.Token, "token", "", "shutdown token (for public shutdown mode)")
	root.PersistentFlags().DurationVar(&opts.Timeout, "timeout", 15*time.Second, "HTTP timeout, e.g. 15s")
	root.PersistentFlags().BoolVar(&opts.Insecure, "insecure", false, "skip TLS certificate verification")
	root.PersistentFlags().StringVar(&opts.Output, "output", "table", "output format: table|json")

	root.AddCommand(newContextCmd())
	root.AddCommand(newCompletionCmd())
	root.AddCommand(newStatusCmd(opts))
	root.AddCommand(newTorrentsCmd(opts))
	root.AddCommand(newURLCmd(opts))
	root.AddCommand(newSettingsCmd(opts))
	root.AddCommand(newAuthCmd(opts))
	root.AddCommand(newShutdownCmd(opts))

	return root
}

func newContextCmd() *cobra.Command {
	contextCmd := &cobra.Command{
		Use:   "context",
		Short: "Управление контекстами (несколько серверов)",
	}

	contextCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Показать все контексты",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadContextConfig()
			if err != nil {
				return err
			}

			return contextList(cfg)
		},
	})

	contextCmd.AddCommand(&cobra.Command{
		Use:   "current",
		Short: "Показать текущий контекст",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadContextConfig()
			if err != nil {
				return err
			}

			return contextCurrent(cfg)
		},
	})

	var (
		addName     string
		addServer   string
		addUser     string
		addPass     string
		addToken    string
		addInsecure bool
	)

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Добавить/обновить контекст",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadContextConfig()
			if err != nil {
				return err
			}

			return contextAdd(
				cfg,
				[]string{
					"--name", addName,
					"--server", addServer,
					"--user", addUser,
					"--pass", addPass,
					"--token", addToken,
					boolFlagArg("insecure", addInsecure),
				},
			)
		},
	}
	addCmd.Flags().StringVar(&addName, "name", "", "context name")
	addCmd.Flags().StringVar(&addServer, "server", "", "server URL")
	addCmd.Flags().StringVar(&addUser, "user", "", "basic auth user")
	addCmd.Flags().StringVar(&addPass, "pass", "", "basic auth password")
	addCmd.Flags().StringVar(&addToken, "token", "", "shutdown token")
	addCmd.Flags().BoolVar(&addInsecure, "insecure", false, "skip TLS verification")
	_ = addCmd.MarkFlagRequired("name")
	_ = addCmd.MarkFlagRequired("server")
	contextCmd.AddCommand(addCmd)

	var useName string

	useCmd := &cobra.Command{
		Use:   "use",
		Short: "Сделать контекст текущим",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadContextConfig()
			if err != nil {
				return err
			}

			return contextUse(cfg, []string{"--name", useName})
		},
	}
	useCmd.Flags().StringVar(&useName, "name", "", "context name")
	_ = useCmd.MarkFlagRequired("name")
	contextCmd.AddCommand(useCmd)

	var removeName string

	removeCmd := &cobra.Command{
		Use:   "remove",
		Short: "Удалить контекст",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadContextConfig()
			if err != nil {
				return err
			}

			return contextRemove(cfg, []string{"--name", removeName})
		},
	}
	removeCmd.Flags().StringVar(&removeName, "name", "", "context name")
	_ = removeCmd.MarkFlagRequired("name")
	contextCmd.AddCommand(removeCmd)

	return contextCmd
}

func newStatusCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Проверка состояния сервера",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdStatus(cli, resolved)
			})
		},
	}
}

func newTorrentsCmd(opts *globalOptions) *cobra.Command {
	torrentsCmd := &cobra.Command{
		Use:   "torrents",
		Short: "Операции с торрентами",
	}

	torrentsCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Список торрентов (с индексом для поиска)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsList(cli, resolved)
			})
		},
	})

	getCmd := &cobra.Command{
		Use:   "get [INDEX|NAME|HASH]",
		Short: "Получить статус торрента по индексу, названию или хэшу",
		Long: `Получить информацию о торренте.
Принимает:
  - Числовой индекс (1-based, из списка torrents list)
  - Частичное название (case-insensitive поиск)
  - Полный 40-символьный hash

Примеры:
  torrserver torrents get 1        # Первый торрент из списка
  torrserver torrents get "Beef"   # Поиск по названию
  torrserver torrents get HASH     # По хэшу`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsGet(cli, resolved, args)
			})
		},
	}
	torrentsCmd.AddCommand(getCmd)

	var (
		addLink     string
		addTitle    string
		addPoster   string
		addCategory string
		addData     string
		addSave     bool
	)

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Добавить торрент",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsAdd(cli, resolved, []string{
					"--link", addLink,
					"--title", addTitle,
					"--poster", addPoster,
					"--category", addCategory,
					"--data", addData,
					boolFlagArg("save", addSave),
				})
			})
		},
	}
	addCmd.Flags().StringVar(&addLink, "link", "", "magnet/hash/file link")
	addCmd.Flags().StringVar(&addTitle, "title", "", "title")
	addCmd.Flags().StringVar(&addPoster, "poster", "", "poster URL")
	addCmd.Flags().StringVar(&addCategory, "category", "", "category")
	addCmd.Flags().StringVar(&addData, "data", "", "custom data")
	addCmd.Flags().BoolVar(&addSave, "save", false, "save torrent to db")
	_ = addCmd.MarkFlagRequired("link")
	torrentsCmd.AddCommand(addCmd)

	remCmd := &cobra.Command{
		Use:   "rem [INDEX|NAME|HASH]",
		Short: "Удалить торрент по индексу, названию или хэшу",
		Long: `Удалить торрент.
Принимает:
  - Числовой индекс (1-based)
  - Частичное название (case-insensitive)
  - Полный 40-символьный hash`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsHashAction(cli, resolved, "rem", args)
			})
		},
	}
	torrentsCmd.AddCommand(remCmd)

	dropCmd := &cobra.Command{
		Use:   "drop [INDEX|NAME|HASH]",
		Short: "Выгрузить торрент из памяти (без удаления из БД)",
		Long: `Выгрузить торрент из активной памяти.
Принимает:
  - Числовой индекс (1-based)
  - Частичное название (case-insensitive)
  - Полный 40-символьный hash`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsHashAction(cli, resolved, "drop", args)
			})
		},
	}
	torrentsCmd.AddCommand(dropCmd)

	torrentsCmd.AddCommand(&cobra.Command{
		Use:   "wipe",
		Short: "Удалить все торренты",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsWipe(cli, resolved)
			})
		},
	})

	return torrentsCmd
}

func newSettingsCmd(opts *globalOptions) *cobra.Command {
	settingsCmd := &cobra.Command{
		Use:   "settings",
		Short: "Операции с настройками",
	}

	settingsCmd.AddCommand(&cobra.Command{
		Use:   "get [KEY]",
		Short: "Получить настройки (все или конкретный ключ)",
		Long: `Получить настройки сервера.
Без аргументов — показывает все настройки таблицей.
С ключом — показывает одно значение.

Примеры:
  torrserver settings get             # Все настройки
  torrserver settings get CacheSize   # Конкретная настройка
  torrserver settings get ConnectionsLimit`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				key := ""
				if len(args) > 0 {
					key = args[0]
				}

				return cmdSettingsGet(cli, resolved, key)
			})
		},
	})

	setCmd := &cobra.Command{
		Use:   "set KEY VALUE",
		Short: "Обновить настройку (KEY VALUE или --json/--file)",
		Long: `Обновить настройку сервера.

Примеры:
  torrserver settings set CacheSize 128MB
  torrserver settings set ConnectionsLimit 50
  torrserver settings set EnableDLNA true
  torrserver settings set FriendlyName "MyServer"
  torrserver settings set --json '{"CacheSize": 134217728}'

Поддерживаемые суффиксы:
  Размеры: KB, MB, GB (например 128MB → 134217728)
  Время: s, m, h (например 30s → 30)`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				// If --json or --file flags are used, fall back to JSON-based update
				jsonRaw, _ := cmd.Flags().GetString("json")
				filePath, _ := cmd.Flags().GetString("file")
				if jsonRaw != "" || filePath != "" {
					sets, err := readSettingsPayload(jsonRaw, filePath)
					if err != nil {
						return err
					}

					ctx, cancel := context.WithTimeout(context.Background(), resolved.Timeout)
					defer cancel()

					payload := map[string]any{
						"action": "set",
						"sets":   sets,
					}

					if err := cli.doJSON(ctx, "POST", "/api/v1/settings", payload, nil, nil); err != nil {
						return err
					}

					fmt.Println("OK: settings updated")

					return nil
				}

				// New behavior: KEY VALUE positional arguments
				if len(args) < 2 {
					return errors.New("settings set requires KEY and VALUE (e.g., CacheSize 128MB)")
				}

				key := args[0]
				value := strings.Join(args[1:], " ")

				return cmdSettingsSetKeyValue(cli, resolved, key, value)
			})
		},
	}
	setCmd.Flags().String("json", "", "raw JSON object with BTSets fields")
	setCmd.Flags().String("file", "", "path to JSON file with BTSets fields")
	settingsCmd.AddCommand(setCmd)

	settingsCmd.AddCommand(&cobra.Command{
		Use:   "def",
		Short: "Сбросить настройки по умолчанию",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdSettingsDef(cli, resolved)
			})
		},
	})

	return settingsCmd
}

func newShutdownCmd(opts *globalOptions) *cobra.Command {
	var (
		mode   string
		reason string
	)

	shutdownCmd := &cobra.Command{
		Use:   "shutdown",
		Short: "Безопасно остановить сервер",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdShutdown(cli, resolved, []string{
					"--mode", mode,
					"--reason", reason,
				})
			})
		},
	}

	shutdownCmd.Flags().StringVar(&mode, "mode", "local", "shutdown mode: local|public")
	shutdownCmd.Flags().StringVar(&reason, "reason", "tsctl", "shutdown reason")

	return shutdownCmd
}

func newURLCmd(opts *globalOptions) *cobra.Command {
	var listFiles bool

	var fileQuery string

	urlCmd := &cobra.Command{
		Use:   "url [INDEX|NAME|HASH]",
		Short: "Получить ссылку на стрим для плеера",
		Long: `Получить прямую ссылку на стрим торрента.
По умолчанию выбирается самый большой файл (обычно кино).

Примеры:
  torrserver url 1                      # Ссылка на самый большой файл
  torrserver url 1 --file 3             # Ссылка на файл #3
  torrserver url "Beef"                 # Поиск по названию
  torrserver url 1 --list               # Список файлов торрента`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdURLWithFlags(cli, resolved, args, listFiles, fileQuery)
			})
		},
	}

	urlCmd.Flags().BoolVar(&listFiles, "list", false, "list files in torrent")
	urlCmd.Flags().StringVar(&fileQuery, "file", "", "file ID or name (substring) to stream")

	return urlCmd
}


func newAuthCmd(opts *globalOptions) *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Управление пользователями сервера",
		Long:  `Управление учетными записями для авторизации на сервере.`,
	}

	authCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Показать список пользователей",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdAuthList(cli, resolved)
			})
		},
	})

	var newPassword string

	addCmd := &cobra.Command{
		Use:   "add [USERNAME]",
		Short: "Добавить нового пользователя",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdAuthAdd(cli, resolved, args[0], newPassword)
			})
		},
	}
	addCmd.Flags().StringVar(&newPassword, "password", "", "password for new user (prompts interactively if omitted)")
	authCmd.AddCommand(addCmd)

	authCmd.AddCommand(&cobra.Command{
		Use:   "remove [USERNAME]",
		Short: "Удалить пользователя",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdAuthRemove(cli, resolved, args[0])
			})
		},
	})

	return authCmd
}

// readPasswordInteractive prompts for password without echoing (SEC5).
func readPasswordInteractive() (string, error) {
	fmt.Print("Password: ")
	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(pass), nil
}

func runWithClient(cmd *cobra.Command, opts *globalOptions, fn func(*apiClient, globalOptions) error) error {
	if opts == nil {
		return errors.New("global options are not initialized")
	}

	resolved := *opts
	resolved.insecureExplicit = isFlagChanged(cmd, "insecure")
	resolved.Output = strings.ToLower(strings.TrimSpace(resolved.Output))

	if resolved.Output != "table" && resolved.Output != "json" {
		return fmt.Errorf("invalid --output value: %s", resolved.Output)
	}

	if resolved.Timeout <= 0 {
		return fmt.Errorf("invalid --timeout value: %s", resolved.Timeout)
	}

	// SEC5: Support env vars for secure credential handling
	if resolved.User == "" {
		resolved.User = os.Getenv("TS_USER")
	}

	if resolved.Pass == "" {
		resolved.Pass = os.Getenv("TS_PASSWORD")
	}

	// Warn if password is passed via command line (visible in ps)
	if isFlagChanged(cmd, "pass") {
		fmt.Fprintln(os.Stderr, "Warning: --pass is visible in process list. Use TS_PASSWORD env var for security.")
	}

	// Prompt for password interactively if user is set but password is not
	if resolved.User != "" && resolved.Pass == "" && !isFlagChanged(cmd, "pass") {
		pass, err := readPasswordInteractive()
		if err != nil {
			return err
		}

		resolved.Pass = pass
	}

	resolved, err := applyContextToOptions(resolved)
	if err != nil {
		return err
	}

	cli, err := newAPIClient(resolved)
	if err != nil {
		return err
	}

	return fn(cli, resolved)
}

func isFlagChanged(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}

	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
		return true
	}

	if flag := cmd.InheritedFlags().Lookup(name); flag != nil && flag.Changed {
		return true
	}

	return false
}

func boolFlagArg(name string, val bool) string {
	if val {
		return "--" + name
	}

	return "--" + name + "=false"
}

// newCompletionCmd creates a command to generate shell completions.
func newCompletionCmd() *cobra.Command {
	completionCmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate completion script",
		Long: `To load completions:

Bash:
  $ source <(torrserver completion bash)
  # To load completions for each session, execute once:
  Linux:
    $ torrserver completion bash > /etc/bash_completion.d/torrserver
  macOS:
    $ torrserver completion bash > /usr/local/etc/bash_completion.d/torrserver

Zsh:
  # If shell completion is not already enabled in your environment:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  $ torrserver completion zsh > "${fpath[1]}/_torrserver"
  # You will need to start a new shell for this setup to take effect.

Fish:
  $ torrserver completion fish | source
  # To load completions for each session, execute once:
  $ torrserver completion fish > ~/.config/fish/completions/torrserver.fish

PowerShell:
  PS> torrserver completion powershell | Out-String | Invoke-Expression`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletion(os.Stdout)
			}

			return nil
		},
	}

	return completionCmd
}
