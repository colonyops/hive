package app

import (
	"github.com/colonyops/hive/pkg/executil"

	"github.com/colonyops/hive/cmd/desktop/internal/app/execenv"
)

// newEnvExecutor is Hive's shell executor running its children in the
// environment execenv resolves (ADR subprocess-environment). Session hooks are
// the user's own commands and reach it as `sh -c`, so without this they run
// with the PATH a desktop launch inherits and a session cannot be created at
// all on a machine whose tools came from a package manager.
//
// Hive streams hook output to io.Discard, so the error repeats the opening of
// stderr: a failed hook would otherwise surface in the jobs list as an exit
// status with nothing naming the command the shell could not find.
func newEnvExecutor(env *execenv.Resolver) executil.Executor {
	return &executil.RealExecutor{
		Env:                   env.Environ,
		LookPath:              env.LookPath,
		StreamDiagnosticBytes: maxDiagnosticBytes,
	}
}

// maxDiagnosticBytes is what of a failing command's stderr travels in the
// error. A shell's "command not found" is the first line; a compiler's wall of
// output is not worth carrying into a job record.
const maxDiagnosticBytes = 2 << 10
