package commands

// ExitError ends the hive program with Code and prints Msg when it is not
// empty. It is not a cli.ExitCoder on purpose: urfave/cli calls os.Exit for
// those inside Run, which would end a test that drives a command.
type ExitError struct {
	Code int
	Msg  string
}

func (e ExitError) Error() string { return e.Msg }

// errAlreadyReported exits 1 after the command wrote its own error output.
var errAlreadyReported = ExitError{Code: 1}
