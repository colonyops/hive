package app

import (
	"github.com/colonyops/hive/pkg/executil"

	"github.com/colonyops/hive/internal/platform/execenv"
)

// newEnvExecutor runs Hive's children in the environment execenv resolves
// (ADR subprocess-environment), so session hooks do not get the bare PATH a
// desktop launch inherits.
//
// Hive streams hook output to io.Discard, so the error repeats the opening of
// stderr. Without it a failed hook shows only an exit status.
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
