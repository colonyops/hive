package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/prompts"
)

// PromptsService exposes the paste-ready LLM prompts to the frontend: the
// "LLM prompts" settings section lists Catalog(), and context-scoped surfaces
// call Render() with instance data.
//
// Prompt text lives in cmd/desktop/internal/app/prompts, not here and not in any Vue
// component — this is transport only.
type PromptsService struct {
	prompts *app.PromptsService
}

func NewPromptsService(p *app.PromptsService) *PromptsService {
	return &PromptsService{prompts: p}
}

func (s *PromptsService) Catalog(ctx context.Context, input prompts.Input) ([]prompts.Prompt, error) {
	return s.prompts.Catalog(ctx, input)
}

func (s *PromptsService) Render(ctx context.Context, id string, input prompts.Input) (prompts.Prompt, error) {
	return s.prompts.Render(ctx, id, input)
}
