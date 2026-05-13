package settings

// ArgsProvider defines access to current execution/runtime arguments.
type ArgsProvider interface {
	Get() *ExecArgs
}

// DefaultArgsProvider is the default runtime arguments provider.
var DefaultArgsProvider ArgsProvider = argsProvider{}

type argsProvider struct{}

func (argsProvider) Get() *ExecArgs {
	return GetArgs()
}

type noopArgsProvider struct{}

// NewNoopArgsProvider returns a safe inert args provider for non-runtime paths.
// It avoids silently binding callers to the process-global exec args singleton.
func NewNoopArgsProvider() ArgsProvider {
	return noopArgsProvider{}
}

func (noopArgsProvider) Get() *ExecArgs {
	return nil
}
