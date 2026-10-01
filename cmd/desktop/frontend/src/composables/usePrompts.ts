import { ref } from 'vue'
import { Catalog, Render } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/promptsservice'
import type { Prompt } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/prompts/models'

// The frontend never builds prompt text. Every prompt — the settings catalog
// and the context-scoped ones offered from an editor — is assembled by
// cmd/desktop/internal/app/prompts from templates that compose shared fragments, so
// wording is written once and each prompt names this install's real config
// paths rather than a placeholder.
//
// This module is transport and nothing else: the only caller-supplied context
// left is a webhook node's own path and sample, which its editor holds.

/** The settings catalog: every prompt that needs no per-instance context. */
export function usePromptCatalog() {
  const prompts = ref<Prompt[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      prompts.value = (await Catalog({ webhookPath: '', webhookSample: '' })) ?? []
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Could not load prompts.'
      prompts.value = []
    } finally {
      loading.value = false
    }
  }

  return { prompts, loading, error, refresh }
}

/**
 * Renders one prompt by id. Context-scoped prompts (a webhook node's transform
 * prompt) pass their instance data through `context`; catalog prompts pass
 * nothing. Returns null on failure so callers can surface a copy error rather
 * than putting a half-built prompt on the clipboard.
 */
export async function renderPrompt(
  id: string,
  context: { webhookPath?: string; webhookSample?: string } = {},
): Promise<string | null> {
  try {
    const prompt = await Render(id, {
      webhookPath: context.webhookPath ?? '',
      webhookSample: context.webhookSample ?? '',
    })
    return prompt.text
  } catch (error) {
    console.warn(`Unable to render the "${id}" prompt`, error)
    return null
  }
}
