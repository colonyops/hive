package runtime

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

// functionProcessor is a function node: user-authored script, evaluated once
// per message through the language's ScriptRuntime.
//
// The instance is created lazily and re-created after a reset, which is what
// makes "terminate, respawn" cheap: the node's state object goes with it, so a
// node that timed out starts the next message from a clean slate rather than
// from whatever half-finished state it was interrupted in.
type functionProcessor struct {
	rt      ScriptRuntime
	src     string
	config  *flow.FunctionConfig
	outputs int

	instance ScriptInstance
}

// newFunctionNode resolves a function node's language and compiles its
// script. Compiling here rather than on the first message means a flow with a
// syntax error is reported when it is deployed, not when a message finally
// reaches the node.
func newFunctionNode(r *Runner, nodeID string, config flow.NodeConfig) (processor, error) {
	cfg, ok := config.(*flow.FunctionConfig)
	if !ok {
		return nil, fmt.Errorf("function: unexpected config type %T", config)
	}
	if r.opts.Scripts == nil {
		return nil, fmt.Errorf("function: node %q needs a script runtime and none is registered", nodeID)
	}
	rt, ok := r.opts.Scripts.Lookup(DefaultScriptLanguage)
	if !ok {
		return nil, fmt.Errorf("function: no %s runtime is registered", DefaultScriptLanguage)
	}
	if err := rt.Check(cfg.OnMessage); err != nil {
		return nil, err
	}
	return &functionProcessor{
		rt:      rt,
		src:     cfg.OnMessage,
		config:  cfg,
		outputs: cfg.Outputs(),
	}, nil
}

func (p *functionProcessor) process(ctx context.Context, msg models.Msg, kv NodeKV, console ConsoleSink) ([][]models.Msg, error) {
	if p.instance == nil {
		instance, err := p.rt.New(p.src, p.outputs)
		if err != nil {
			return nil, err
		}
		p.instance = instance
	}
	return p.instance.OnMessage(ctx, msg, p.config, kv, console)
}

// reset drops the instance so the next message compiles a fresh one with a
// fresh state object.
func (p *functionProcessor) reset() {
	if p.instance == nil {
		return
	}
	p.instance.Close()
	p.instance = nil
}

func (p *functionProcessor) close() { p.reset() }
