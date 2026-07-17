package cliapp

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"server/internal/apiclient"
	buildversion "server/version"
)

// Invocation contains all process input and output needed for one CLI run.
type Invocation struct {
	Context context.Context
	Args    []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// ClientFactory creates the management API transport used by remote commands.
type ClientFactory func(apiclient.Options) (*apiclient.Client, error)

// PasswordReader reads one secret from the supplied input without echoing it
// when the input is an interactive terminal.
type PasswordReader func(io.Reader, io.Writer) (string, error)

// Dependencies contains replaceable process adapters. It is safe to leave
// fields nil; Run fills them with production defaults without mutating deps.
type Dependencies struct {
	ProgramName     string
	Getenv          func(string) string
	FileSystem      FileSystem
	NewClient       ClientFactory
	IsTerminal      func(io.Reader) bool
	ReadPassword    PasswordReader
	ReadNewPassword PasswordReader
	BuildInfo       buildversion.Info
}

type runtimeDeps struct {
	programName     string
	getenv          func(string) string
	fileSystem      FileSystem
	contextStore    contextStore
	newClient       ClientFactory
	isTerminal      func(io.Reader) bool
	readPassword    PasswordReader
	readNewPassword PasswordReader
	buildInfo       buildversion.Info
}

// DefaultDependencies returns production adapters without performing I/O.
func DefaultDependencies() Dependencies {
	return Dependencies{
		ProgramName:     defaultProgramName,
		Getenv:          os.Getenv,
		FileSystem:      osFileSystem{},
		NewClient:       apiclient.New,
		IsTerminal:      readerIsTerminal,
		ReadPassword:    readPasswordInteractive,
		ReadNewPassword: readPasswordInteractively,
		BuildInfo:       buildversion.Current(),
	}
}

// Run executes one management CLI invocation and returns a deterministic
// process exit code. It never calls os.Exit.
func Run(invocation Invocation, dependencies Dependencies) int {
	runtime := newRuntimeDeps(dependencies)
	root := newRootCmd(runtime)
	root.SetArgs(append([]string(nil), invocation.Args...))
	root.SetIn(readerOrEmpty(invocation.Stdin))
	root.SetOut(writerOrDiscard(invocation.Stdout))
	root.SetErr(writerOrDiscard(invocation.Stderr))
	root.SetContext(contextOrBackground(invocation.Context))

	if err := root.Execute(); err != nil {
		stderr := root.ErrOrStderr()
		if requestedJSONOutput(invocation.Args) {
			if encodeErr := writeJSONError(stderr, err); encodeErr != nil {
				writeFallbackError(stderr, runtime.programName, encodeErr)
			}
		} else {
			writeFallbackError(stderr, runtime.programName, err)
		}

		return 1
	}

	return 0
}

func newRuntimeDeps(dependencies Dependencies) *runtimeDeps {
	defaults := DefaultDependencies()

	programName := normalizeProgramName(dependencies.ProgramName)
	if programName == "" {
		programName = defaults.ProgramName
	}

	getenv := dependencies.Getenv
	if getenv == nil {
		getenv = defaults.Getenv
	}

	fileSystem := dependencies.FileSystem
	if fileSystem == nil {
		fileSystem = defaults.FileSystem
	}

	newClient := dependencies.NewClient
	if newClient == nil {
		newClient = defaults.NewClient
	}

	isTerminal := dependencies.IsTerminal
	if isTerminal == nil {
		isTerminal = defaults.IsTerminal
	}

	readPassword := dependencies.ReadPassword
	if readPassword == nil {
		readPassword = defaults.ReadPassword
	}

	readNewPassword := dependencies.ReadNewPassword
	if readNewPassword == nil {
		readNewPassword = defaults.ReadNewPassword
	}

	info := dependencies.BuildInfo
	if info.Version == "" {
		info = defaults.BuildInfo
	}

	return &runtimeDeps{
		programName:     programName,
		getenv:          getenv,
		fileSystem:      fileSystem,
		contextStore:    newFileContextStore(fileSystem, getenv),
		newClient:       newClient,
		isTerminal:      isTerminal,
		readPassword:    readPassword,
		readNewPassword: readNewPassword,
		buildInfo:       info,
	}
}

func readerOrEmpty(reader io.Reader) io.Reader {
	if reader != nil {
		return reader
	}

	return strings.NewReader("")
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}

	return io.Discard
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}

	return context.Background()
}

func writeFallbackError(writer io.Writer, programName string, err error) {
	prefix := "Error"
	if programName != defaultProgramName {
		prefix = programName + ": error"
	}

	_, _ = fmt.Fprintf(writer, "%s: %v\n", prefix, err)
}
