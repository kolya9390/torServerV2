package cliapp

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"server/internal/apiclient"
	buildversion "server/version"
)

func executeCLI(args []string, stdout, stderr io.Writer) int {
	return Run(Invocation{Args: args, Stdout: stdout, Stderr: stderr}, DefaultDependencies())
}

func newRootCmdWithBuildInfo(info buildversion.Info) *cobra.Command {
	dependencies := DefaultDependencies()
	dependencies.BuildInfo = info

	return newRootCmd(newRuntimeDeps(dependencies))
}

func newRootCmd(runtime *runtimeDeps) *cobra.Command {
	opts := &globalOptions{runtime: runtime}
	info := runtime.buildInfo
	short, long, examples := rootCommandDescription(runtime.programName)

	root := &cobra.Command{
		Use:           runtime.programName,
		Short:         short,
		SilenceUsage:  true,
		SilenceErrors: true,
		Long:          long,
		Example:       examples,
		Version:       buildversion.Concise(runtime.programName, info),
	}
	root.SetVersionTemplate("{{.Version}}\n")

	root.PersistentFlags().StringVar(
		&opts.Context,
		"context",
		strings.TrimSpace(runtime.getenv(envContext)),
		"context name",
	)
	root.PersistentFlags().StringVar(&opts.Server, "server", "", "base server URL (overrides context)")
	root.PersistentFlags().StringVar(&opts.User, "user", "", "basic auth username")
	root.PersistentFlags().StringVar(&opts.Pass, "pass", "", "basic auth password")
	root.PersistentFlags().StringVar(&opts.Token, "token", "", "shutdown token (public shutdown or config set)")
	root.PersistentFlags().DurationVar(&opts.Timeout, "timeout", defaultTimeout, "HTTP timeout, e.g. 15s")
	root.PersistentFlags().BoolVar(&opts.Insecure, "insecure", false, "skip TLS certificate verification")
	root.PersistentFlags().StringVar(&opts.Output, "output", defaultOutput, "output format: table|json")

	root.AddCommand(newContextCmd(opts))
	root.AddCommand(newCompletionCmd())
	root.AddCommand(newVersionCmd(opts, info))
	root.AddCommand(newStatusCmd(opts))
	root.AddCommand(newTorrentsCmd(opts))
	root.AddCommand(newURLCmd(opts))
	root.AddCommand(newSettingsCmd(opts))
	root.AddCommand(newAuthCmd(opts))
	root.AddCommand(newConfigCmd(opts))
	root.AddCommand(newShutdownCmd(opts))
	rewriteCommandProgram(root, runtime.programName)

	return root
}

func rootCommandDescription(programName string) (string, string, string) {
	if programName == "torrctl" {
		return "CLI для управления TorrServer", "Клиент управления TorrServer через HTTP API. " +
				"Не запускает torrent-движок и не требует локальной конфигурации сервера.", strings.Join([]string{
				"  torrctl status                                 # Проверить статус",
				"  torrctl torrents list                          # Список торрентов",
				"  torrctl torrents add 'magnet:...' --save        # Добавить magnet",
				"  torrctl torrents add ./movie.torrent --save     # Загрузить .torrent",
				"  torrctl url 1                                  # Получить stream URL",
				"  torrctl settings get                           # Получить настройки",
				"  torrctl settings set CacheSize 128MB           # Изменить кэш",
			}, "\n")
	}

	return "TorrServer — торрент стриминг сервер и CLI",
		"TorrServer — минималистичный сервер для стриминга торрентов.\n\n" +
			"Без аргументов запускает сервер.\nС аргументами работает как CLI для управления.",
		strings.Join([]string{
			"  torrserver                                        # Запуск сервера",
			"  torrserver status                                 # Проверить статус",
			"  torrserver torrents list                          # Список торрентов",
			"  torrserver torrents add 'magnet:...' --save        # Добавить magnet",
			"  torrserver torrents add ./movie.torrent --save     # Загрузить .torrent",
			"  torrserver url 1                                  # Получить stream URL",
			"  torrserver settings get                           # Получить настройки",
			"  torrserver settings set CacheSize 128MB           # Изменить кэш",
		}, "\n")
}

func rewriteCommandProgram(command *cobra.Command, programName string) {
	command.Long = commandText(programName, command.Long)
	command.Example = commandText(programName, command.Example)

	for _, child := range command.Commands() {
		rewriteCommandProgram(child, programName)
	}
}

func newConfigCmd(opts *globalOptions) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Управление конфигурацией сервера",
	}
	tokenCmd := &cobra.Command{
		Use:   "shutdown-token",
		Short: "Управление токеном публичного shutdown",
	}

	tokenCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Проверить наличие shutdown token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdShutdownTokenStatus(cli, resolved)
			})
		},
	})

	var generateYes bool

	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Сгенерировать и сохранить новый shutdown token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := confirmCommand(cmd, opts, "Rotate the shutdown token", generateYes); err != nil {
				return err
			}

			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdGenerateShutdownToken(cli, resolved)
			})
		},
	}
	generateCmd.Flags().BoolVar(&generateYes, "yes", false, "confirm token rotation without an interactive prompt")
	tokenCmd.AddCommand(generateCmd)

	var setYes bool

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Сохранить TS_SHUTDOWN_TOKEN или global --token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := confirmCommand(cmd, opts, "Replace the shutdown token", setYes); err != nil {
				return err
			}

			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdSetShutdownToken(cli, resolved)
			})
		},
	}
	setCmd.Flags().BoolVar(&setYes, "yes", false, "confirm token replacement without an interactive prompt")
	tokenCmd.AddCommand(setCmd)

	configCmd.AddCommand(tokenCmd)

	return configCmd
}

func newContextCmd(opts *globalOptions) *cobra.Command {
	contextCmd := &cobra.Command{
		Use:   "context",
		Short: "Управление контекстами (несколько серверов)",
	}

	contextCmd.AddCommand(newContextListCmd(opts))
	contextCmd.AddCommand(newContextCurrentCmd(opts))
	contextCmd.AddCommand(newContextAddCmd(opts))
	contextCmd.AddCommand(newContextUseCmd(opts))
	contextCmd.AddCommand(newContextRemoveCmd(opts))

	return contextCmd
}

func newContextListCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Показать все контексты",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := opts.runtime.contextStore.Load()
			if err != nil {
				return err
			}

			return runWithOutput(cmd, opts, func(resolved globalOptions) error {
				return contextList(cfg, resolved)
			})
		},
	}
}

func newContextCurrentCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Показать текущий контекст",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := opts.runtime.contextStore.Load()
			if err != nil {
				return err
			}

			return runWithOutput(cmd, opts, func(resolved globalOptions) error {
				return contextCurrent(cfg, resolved)
			})
		},
	}
}

func newContextAddCmd(opts *globalOptions) *cobra.Command {
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
		Args: requireContextFlags(
			requiredContextFlag{name: "name", value: &addName},
			requiredContextFlag{name: "server", value: &addServer},
		),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := opts.runtime.contextStore.Load()
			if err != nil {
				return err
			}

			return runWithOutput(cmd, opts, func(resolved globalOptions) error {
				return contextAdd(cfg, resolved, addName, contextEntry{
					Server:   addServer,
					User:     addUser,
					Pass:     addPass,
					Token:    addToken,
					Insecure: addInsecure,
				})
			})
		},
	}
	addCmd.Flags().StringVar(&addName, "name", "", "context name")
	addCmd.Flags().StringVar(&addServer, "server", "", "server URL")
	addCmd.Flags().StringVar(&addUser, "user", "", "basic auth user")
	addCmd.Flags().StringVar(&addPass, "pass", "", "basic auth password")
	addCmd.Flags().StringVar(&addToken, "token", "", "shutdown token")
	addCmd.Flags().BoolVar(&addInsecure, "insecure", false, "skip TLS verification")

	return addCmd
}

func newContextUseCmd(opts *globalOptions) *cobra.Command {
	var useName string

	useCmd := &cobra.Command{
		Use:   "use",
		Short: "Сделать контекст текущим",
		Args:  requireContextFlags(requiredContextFlag{name: "name", value: &useName}),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := opts.runtime.contextStore.Load()
			if err != nil {
				return err
			}

			return runWithOutput(cmd, opts, func(resolved globalOptions) error {
				return contextUse(cfg, resolved, useName)
			})
		},
	}
	useCmd.Flags().StringVar(&useName, "name", "", "context name")

	return useCmd
}

func newContextRemoveCmd(opts *globalOptions) *cobra.Command {
	var removeName string

	removeCmd := &cobra.Command{
		Use:   "remove",
		Short: "Удалить контекст",
		Args:  requireContextFlags(requiredContextFlag{name: "name", value: &removeName}),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := opts.runtime.contextStore.Load()
			if err != nil {
				return err
			}

			return runWithOutput(cmd, opts, func(resolved globalOptions) error {
				return contextRemove(cfg, resolved, removeName)
			})
		},
	}
	removeCmd.Flags().StringVar(&removeName, "name", "", "context name")

	return removeCmd
}

type requiredContextFlag struct {
	name  string
	value *string
}

func requireContextFlags(flags ...requiredContextFlag) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return err
		}

		for _, flag := range flags {
			if flag.value == nil || strings.TrimSpace(*flag.value) == "" {
				return fmt.Errorf("required flag --%s is not set", flag.name)
			}
		}

		return nil
	}
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

	torrentsCmd.AddCommand(newTorrentsListCmd(opts))
	torrentsCmd.AddCommand(newTorrentsGetCmd(opts))
	torrentsCmd.AddCommand(newTorrentsAddCmd(opts))
	torrentsCmd.AddCommand(newTorrentsRemoveCmd(opts))
	torrentsCmd.AddCommand(newTorrentsDropCmd(opts))
	torrentsCmd.AddCommand(newTorrentsWipeCmd(opts))

	return torrentsCmd
}

func newTorrentsListCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Список торрентов (с индексом для поиска)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsList(cli, resolved)
			})
		},
	}
}

func newTorrentsGetCmd(opts *globalOptions) *cobra.Command {
	var getHash string

	getCmd := &cobra.Command{
		Use:   "get [INDEX|NAME|HASH]",
		Short: "Получить статус торрента",
		Long: `Получить информацию о торренте.

Принимает:
  - Числовой индекс (1-based, из списка torrents list)
  - Частичное название (case-insensitive поиск)
  - Полный 40-символьный hash

Примеры:
  torrserver torrents get 1        # Первый торрент из списка
  torrserver torrents get "Beef"   # Поиск по названию
  torrserver torrents get ef9c...  # По хэшу`,
		Args: validateTorrentIdentifierArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			identifier := torrentIdentifierArg(args, getHash)

			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsGet(cli, resolved, identifier)
			})
		},
	}
	getCmd.Flags().StringVar(&getHash, "hash", "", "torrent hash, name, or index (compatibility flag)")

	return getCmd
}

func newTorrentsAddCmd(opts *globalOptions) *cobra.Command {
	var (
		addLink     string
		addFile     string
		addTitle    string
		addPoster   string
		addCategory string
		addData     string
		addSave     bool
	)

	addCmd := &cobra.Command{
		Use:   "add [MAGNET|HASH|FILE]",
		Short: "Добавить торрент",
		Long: `Добавить торрент по magnet-ссылке, хэшу или загрузить локальный .torrent-файл.

Примеры:
	  torrserver torrents add "magnet:?xt=..." --save
	  torrserver torrents add d41d8cd98f00b204e9800998ecf8427e
	  torrserver torrents add ./movie.torrent --save
	  torrserver torrents add --file ./movie.torrent --title "My Movie"

Локальный файл загружается на выбранный сервер через multipart API.
Флаг --link сохранён для совместимости.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			addOpts, err := resolveTorrentAddOptions(args, torrentAddOptions{
				Link:       addLink,
				File:       addFile,
				Title:      addTitle,
				Poster:     addPoster,
				Category:   addCategory,
				Data:       addData,
				Save:       addSave,
				fileSystem: opts.runtime.fileSystem,
			})
			if err != nil {
				return err
			}

			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsAdd(cli, resolved, addOpts)
			})
		},
	}
	addCmd.Flags().StringVar(&addLink, "link", "", "magnet, hash, remote link, or local .torrent path")
	addCmd.Flags().StringVar(&addFile, "file", "", "local .torrent file to upload")
	addCmd.Flags().StringVar(&addTitle, "title", "", "title")
	addCmd.Flags().StringVar(&addPoster, "poster", "", "poster URL")
	addCmd.Flags().StringVar(&addCategory, "category", "", "category")
	addCmd.Flags().StringVar(&addData, "data", "", "custom data")
	addCmd.Flags().BoolVar(&addSave, "save", false, "save torrent to db")

	return addCmd
}

func newTorrentsRemoveCmd(opts *globalOptions) *cobra.Command {
	var removeHash string

	remCmd := &cobra.Command{
		Use:   "rem [INDEX|NAME|HASH]",
		Short: "Удалить торрент",
		Long: `Удалить торрент из базы данных.

Примеры:
  torrserver torrents rem 1           # По индексу
  torrserver torrents rem "Beef"      # По названию
  torrserver torrents rem ef9c...     # По хэшу`,
		Args: validateTorrentIdentifierArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			identifier := torrentIdentifierArg(args, removeHash)

			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsHashAction(cli, resolved, "rem", identifier)
			})
		},
	}
	remCmd.Flags().StringVar(&removeHash, "hash", "", "torrent hash, name, or index (compatibility flag)")

	return remCmd
}

func newTorrentsDropCmd(opts *globalOptions) *cobra.Command {
	var dropHash string

	dropCmd := &cobra.Command{
		Use:   "drop [INDEX|NAME|HASH]",
		Short: "Выгрузить торрент из памяти",
		Long: `Выгрузить торрент из активной памяти (без удаления из БД).

Примеры:
  torrserver torrents drop 1          # По индексу
  torrserver torrents drop "Beef"     # По названию`,
		Args: validateTorrentIdentifierArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			identifier := torrentIdentifierArg(args, dropHash)

			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsHashAction(cli, resolved, "drop", identifier)
			})
		},
	}
	dropCmd.Flags().StringVar(&dropHash, "hash", "", "torrent hash, name, or index (compatibility flag)")

	return dropCmd
}

func newTorrentsWipeCmd(opts *globalOptions) *cobra.Command {
	var wipeYes bool

	wipeCmd := &cobra.Command{
		Use:   "wipe",
		Short: "Удалить все торренты",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := confirmCommand(cmd, opts, "Delete all torrents", wipeYes); err != nil {
				return err
			}

			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdTorrentsWipe(cli, resolved)
			})
		},
	}
	wipeCmd.Flags().BoolVar(&wipeYes, "yes", false, "confirm deletion without an interactive prompt")

	return wipeCmd
}

func validateTorrentIdentifierArgs(cmd *cobra.Command, args []string) error {
	hash, err := cmd.Flags().GetString("hash")
	if err != nil {
		return fmt.Errorf("read --hash: %w", err)
	}

	hasHash := strings.TrimSpace(hash) != ""
	if hasHash && len(args) > 0 {
		return errors.New("provide the torrent identifier either positionally or with --hash, not both")
	}

	if !hasHash {
		return cobra.ExactArgs(1)(cmd, args)
	}

	return cobra.NoArgs(cmd, args)
}

func torrentIdentifierArg(args []string, hash string) string {
	if strings.TrimSpace(hash) != "" {
		return hash
	}

	return args[0]
}

func newSettingsCmd(opts *globalOptions) *cobra.Command {
	settingsCmd := &cobra.Command{
		Use:   "settings",
		Short: "Операции с настройками",
	}

	settingsCmd.AddCommand(newSettingsGetCmd(opts))
	settingsCmd.AddCommand(newSettingsSetCmd(opts))
	settingsCmd.AddCommand(newSettingsDefaultsCmd(opts))

	return settingsCmd
}

func newSettingsGetCmd(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
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
	}
}

func newSettingsSetCmd(opts *globalOptions) *cobra.Command {
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

Единицы применяются только к совместимым полям:
  CacheSize: KB, MB, GB (например 128MB)
  Поля времени: ms, s, m, h (например StreamQueueWaitSec 30s)

EnableDebug и остальные debug-параметры задаются через config.yml и требуют перезапуска.`,
		Args: validateSettingsSetArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				jsonRaw, filePath, err := settingsPayloadFlags(cmd)
				if err != nil {
					return err
				}

				if jsonRaw != "" || filePath != "" {
					sets, err := readSettingsPayload(opts.runtime.fileSystem, jsonRaw, filePath)
					if err != nil {
						return err
					}

					ctx, cancel := resolved.timeoutContext(resolved.Timeout)
					defer cancel()

					if err := cli.SetSettings(ctx, apiclient.SettingsPatch(sets)); err != nil {
						return err
					}

					return writeCommandResult(
						resolved,
						map[string]any{"action": "settings_updated", "settings": sets},
						"OK: settings updated",
					)
				}

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
	setCmd.MarkFlagsMutuallyExclusive("json", "file")

	return setCmd
}

func newSettingsDefaultsCmd(opts *globalOptions) *cobra.Command {
	var defaultsYes bool

	defaultsCmd := &cobra.Command{
		Use:   "def",
		Short: "Сбросить настройки по умолчанию",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := confirmCommand(cmd, opts, "Reset all settings to defaults", defaultsYes); err != nil {
				return err
			}

			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdSettingsDef(cli, resolved)
			})
		},
	}
	defaultsCmd.Flags().BoolVar(&defaultsYes, "yes", false, "confirm reset without an interactive prompt")

	return defaultsCmd
}

func validateSettingsSetArgs(cmd *cobra.Command, args []string) error {
	jsonRaw, filePath, err := settingsPayloadFlags(cmd)
	if err != nil {
		return err
	}

	if strings.TrimSpace(jsonRaw) != "" || strings.TrimSpace(filePath) != "" {
		return cobra.MaximumNArgs(0)(cmd, args)
	}

	return cobra.MinimumNArgs(1)(cmd, args)
}

func settingsPayloadFlags(cmd *cobra.Command) (string, string, error) {
	jsonRaw, err := cmd.Flags().GetString("json")
	if err != nil {
		return "", "", fmt.Errorf("read --json: %w", err)
	}

	filePath, err := cmd.Flags().GetString("file")
	if err != nil {
		return "", "", fmt.Errorf("read --file: %w", err)
	}

	return jsonRaw, filePath, nil
}

func newShutdownCmd(opts *globalOptions) *cobra.Command {
	var (
		mode string
	)

	shutdownCmd := &cobra.Command{
		Use:   "shutdown",
		Short: "Безопасно остановить сервер",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithClient(cmd, opts, func(cli *apiClient, resolved globalOptions) error {
				return cmdShutdown(cli, resolved, mode, defaultReason)
			})
		},
	}

	shutdownCmd.Flags().StringVar(&mode, "mode", "local", "shutdown mode: local|public")

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
				return cmdURLWithFlags(cli, resolved, args[0], listFiles, fileQuery)
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
func readPasswordInteractive(input io.Reader, output io.Writer) (string, error) {
	_, _ = fmt.Fprint(output, "Password: ")

	fd, ok := readerFileDescriptor(input)
	if !ok {
		return "", errors.New("password input is not a terminal; set TS_PASSWORD instead")
	}

	pass, err := term.ReadPassword(fd)

	_, _ = fmt.Fprintln(output)

	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}

	return string(pass), nil
}

func runWithClient(cmd *cobra.Command, opts *globalOptions, fn func(*apiClient, globalOptions) error) error {
	return runWithOutput(cmd, opts, func(base globalOptions) error {
		resolved, err := resolveClientOptions(cmd, base)
		if err != nil {
			return err
		}

		if isFlagChanged(cmd, "pass") && resolved.Output == outputTable {
			_, _ = fmt.Fprintln(
				resolved.stderrWriter(),
				"Warning: --pass is visible in process list. Use "+envPassword+" env var for security.",
			)
		}

		resolved, err = resolveClientPassword(
			resolved,
			resolved.isTerminal(),
			func() (string, error) { return resolved.promptPassword(resolved.stderrWriter()) },
		)
		if err != nil {
			return err
		}

		cli, err := resolved.runtime.newClient(apiclient.Options{
			BaseURL:  resolved.Server,
			User:     resolved.User,
			Password: resolved.Pass,
			Timeout:  resolved.Timeout,
			Insecure: resolved.Insecure,
		})
		if err != nil {
			return err
		}

		return fn(cli, resolved)
	})
}

func runWithOutput(cmd *cobra.Command, opts *globalOptions, fn func(globalOptions) error) error {
	if opts == nil {
		return errors.New("global options are not initialized")
	}

	resolved := *opts
	resolved.Output = strings.ToLower(strings.TrimSpace(resolved.Output))
	resolved.stdout = cmd.OutOrStdout()
	resolved.stderr = cmd.ErrOrStderr()
	resolved.stdin = cmd.InOrStdin()

	resolved.ctx = cmd.Context()
	if resolved.runtime == nil {
		resolved.runtime = newRuntimeDeps(DefaultDependencies())
	}

	if resolved.isTerminal == nil {
		resolved.isTerminal = func() bool { return resolved.runtime.isTerminal(resolved.stdin) }
	}

	if resolved.readPassword == nil {
		resolved.readPassword = func(output io.Writer) (string, error) {
			return resolved.runtime.readPassword(resolved.stdin, output)
		}
	}

	if resolved.readNewPassword == nil {
		resolved.readNewPassword = func(output io.Writer) (string, error) {
			return resolved.runtime.readNewPassword(resolved.stdin, output)
		}
	}

	if resolved.Output != outputTable && resolved.Output != outputJSON {
		return fmt.Errorf(
			"invalid --output value: %s (valid: %v)",
			resolved.Output,
			ValidOutputFormats(),
		)
	}

	return fn(resolved)
}

func resolveClientOptions(cmd *cobra.Command, opts globalOptions) (globalOptions, error) {
	resolved := opts
	if resolved.runtime == nil {
		resolved.runtime = newRuntimeDeps(DefaultDependencies())
	}

	resolved.insecureExplicit = isFlagChanged(cmd, "insecure")
	resolved.Output = strings.ToLower(strings.TrimSpace(resolved.Output))

	if resolved.Output != outputTable && resolved.Output != outputJSON {
		return globalOptions{}, fmt.Errorf(
			"invalid --output value: %s (valid: %v)",
			resolved.Output,
			ValidOutputFormats(),
		)
	}

	if resolved.Timeout <= 0 {
		return globalOptions{}, fmt.Errorf("invalid --timeout value: %s", resolved.Timeout)
	}

	if resolved.User == "" {
		resolved.User = strings.TrimSpace(resolved.runtime.getenv(envUser))
	}

	if resolved.Pass == "" {
		resolved.Pass = resolved.runtime.getenv(envPassword)
	}

	if resolved.Token == "" {
		resolved.Token = strings.TrimSpace(resolved.runtime.getenv(envToken))
	}

	return applyContextToOptions(resolved)
}

func resolveClientPassword(
	opts globalOptions,
	interactive bool,
	readPassword func() (string, error),
) (globalOptions, error) {
	if opts.User == "" || opts.Pass != "" {
		return opts, nil
	}

	if !interactive {
		return globalOptions{}, errors.New(
			"password is required for the selected user; set TS_PASSWORD or configure the context password",
		)
	}

	pass, err := readPassword()
	if err != nil {
		return globalOptions{}, err
	}

	opts.Pass = pass

	return opts, nil
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
			output := cmd.OutOrStdout()

			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(output)
			case "zsh":
				return cmd.Root().GenZshCompletion(output)
			case "fish":
				return cmd.Root().GenFishCompletion(output, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletion(output)
			}

			return nil
		},
	}

	return completionCmd
}
