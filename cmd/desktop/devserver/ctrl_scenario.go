package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/desktop/internal/web/extractors"
)

// scenarioRequest is the body of POST /_ctl/scenario: a lifecycle sequence
// composed by the caller and run in the background. There are no configured
// scenarios — every scenario is built and driven through this endpoint.
type scenarioRequest struct {
	Steps []stepInput `json:"steps"`
}

// stepInput is one step: an action or a raw set on repo/num, and a wait. Wait
// is a string because JSON has no duration type.
type stepInput struct {
	Repo   string    `json:"repo,omitempty"`
	Num    int       `json:"num,omitempty"`
	Action string    `json:"action,omitempty"`
	Set    Mutations `json:"set"`
	Wait   string    `json:"wait,omitempty"`
}

// ScenarioStep is one validated step the runner applies: a mutation on an item,
// a wait, or both.
type ScenarioStep struct {
	Match Matcher
	Set   Mutations
	Wait  time.Duration
}

type scenarioResponse struct {
	Scenario string `json:"scenario"`
	Steps    int    `json:"steps"`
}

func (c *Control) RunScenario(w http.ResponseWriter, r *http.Request) error {
	req, err := extractors.Body[scenarioRequest](w, r)
	if err != nil {
		return err
	}
	steps, err := buildSteps(req.Steps)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.seq++
	name := fmt.Sprintf("scenario-%d", c.seq)
	c.scenarios[name] = len(steps)
	c.mu.Unlock()

	// A scenario's waits outlive the request, so it runs on a background context:
	// the response returns immediately and cancelling it must not abort the run.
	go c.runScenario(context.Background(), name, steps)
	return server.JSON(w, http.StatusAccepted, scenarioResponse{Scenario: name, Steps: len(steps)})
}

// buildSteps validates a scenario and converts it to the ScenarioStep sequence
// the runner takes, up front so a bad step is a field error the caller can read
// rather than a background failure it cannot.
func buildSteps(reqSteps []stepInput) ([]ScenarioStep, error) {
	if len(reqSteps) == 0 {
		return nil, criterio.NewFieldErrors("steps", errors.New("scenario needs at least one step"))
	}
	steps := make([]ScenarioStep, 0, len(reqSteps))
	for i, s := range reqSteps {
		field := fmt.Sprintf("steps[%d]", i)
		var step ScenarioStep
		if s.Wait != "" {
			wait, err := time.ParseDuration(s.Wait)
			if err != nil {
				return nil, criterio.NewFieldErrors(field, fmt.Errorf("invalid wait %q: %w", s.Wait, err))
			}
			if wait < 0 {
				return nil, criterio.NewFieldErrors(field, errors.New("wait cannot be negative"))
			}
			step.Wait = wait
		}

		hasAction, hasSet := s.Action != "", !s.Set.Empty()
		if hasAction && hasSet {
			return nil, criterio.NewFieldErrors(field, errors.New("provide either action or set, not both"))
		}
		var set Mutations
		switch {
		case hasAction:
			if err := knownAction(s.Action); err != nil {
				return nil, criterio.NewFieldErrors(field, err)
			}
			set = quickActions[s.Action].apply()
		case hasSet:
			if err := validateMutations(s.Set); err != nil {
				return nil, criterio.NewFieldErrors(field, err)
			}
			set = s.Set
		}

		if set.Empty() && step.Wait == 0 {
			return nil, criterio.NewFieldErrors(field, errors.New("does nothing (no action or set, no wait)"))
		}
		if !set.Empty() {
			if err := criterio.Nest(field, matcherErrors(Matcher{Repo: s.Repo, Num: s.Num})); err != nil {
				return nil, err
			}
			step.Match = Matcher{Repo: s.Repo, Num: s.Num}
			step.Set = set
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func (c *Control) runScenario(ctx context.Context, name string, steps []ScenarioStep) {
	defer func() {
		c.mu.Lock()
		delete(c.scenarios, name)
		c.mu.Unlock()
	}()

	c.logger.Info().Str("scenario", name).Int("steps", len(steps)).Msg("scenario started")
	for i, step := range steps {
		if !step.Set.Empty() {
			c.store.Apply(step.Match.Key(), step.Set)
			c.logger.Info().Str("scenario", name).Int("step", i).
				Str("item", step.Match.Key()).Msg("scenario step applied")
		}
		if step.Wait > 0 {
			select {
			case <-ctx.Done():
				c.logger.Warn().Str("scenario", name).Msg("scenario cancelled")
				return
			case <-time.After(step.Wait):
			}
		}
	}
	c.logger.Info().Str("scenario", name).Msg("scenario finished")
}
