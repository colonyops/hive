package app

import (
	"context"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/prompts"
	"github.com/colonyops/hive/cmd/desktop/internal/app/skills"
)

// SkillsService presents the paste-ready prompts as agent skills: what this
// build ships, and one skill rendered as a SKILL.md. It installs nothing —
// a workspace declares the skills it carries and the workspace generator
// writes them (ADR skills-are-declared-by-a-workspace).
type SkillsService struct {
	prompts *PromptsService
}

func newSkillsService(p *PromptsService) *SkillsService {
	return &SkillsService{prompts: p}
}

// ShippedSkills lists every skill this build ships as a workspace can enable
// it: the installed slug plus the labels the skill catalogue shows. A prompt
// that cannot render without caller-supplied context is absent from the
// listing, so the catalogue never offers a skill a workspace open could not
// resolve.
func (s *SkillsService) ShippedSkills(ctx context.Context) ([]agentws.ShippedSkill, error) {
	list, err := s.prompts.Catalog(ctx, prompts.Input{})
	if err != nil {
		return nil, err
	}
	out := make([]agentws.ShippedSkill, 0, len(list))
	for _, p := range list {
		out = append(out, agentws.ShippedSkill{
			Slug:        skillSlug(p.ID),
			Title:       p.Title,
			Description: p.Description,
		})
	}
	return out, nil
}

// RenderSkill renders one listed prompt as a SKILL.md for installation into a
// workspace, returning the slug it installs under and the file contents.
func (s *SkillsService) RenderSkill(ctx context.Context, id string) (name, body string, err error) {
	p, err := s.prompts.Render(ctx, id, prompts.Input{})
	if err != nil {
		return "", "", err
	}
	skill := toSkill(p)
	if err := skills.ValidateSkill(skill); err != nil {
		return "", "", Wrap(err, KindInternal, "rendering skill %q", id)
	}
	rendered, err := skills.Render(skill)
	if err != nil {
		return "", "", Wrap(err, KindInternal, "rendering skill %q", id)
	}
	return skill.Name, rendered, nil
}

// toSkill maps a rendered prompt onto a SKILL.md: a namespaced slug, a
// trigger-oriented description, and the prompt text as the body.
func toSkill(p prompts.Prompt) skills.Skill {
	return skills.Skill{
		ID:          p.ID,
		Name:        skillSlug(p.ID),
		Description: skillDescription(p),
		Body:        p.Text,
	}
}

func skillSlug(id string) string { return "hive-" + id }

// skillDescription builds the SKILL.md frontmatter description: what the skill
// does (the prompt's own description) plus where it applies, clamped to the
// standard's 1024-character limit.
func skillDescription(p prompts.Prompt) string {
	desc := strings.TrimSpace(p.Description)
	if target := strings.TrimSpace(p.Target); target != "" {
		desc += " Use when configuring this in Hive Desktop (" + target + ")."
	}
	if len(desc) > 1024 {
		desc = strings.TrimSpace(string([]rune(desc)[:min(len([]rune(desc)), 1024)]))
	}
	return desc
}
