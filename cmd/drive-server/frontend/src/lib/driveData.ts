import { inject, reactive, ref, type InjectionKey, type Ref } from 'vue'
import { api } from '../api'
import type { DriveDocument, SpaceSummary } from '../types'
import { spaceFromKey } from './space'

export interface DriveData {
  spaces: Ref<SpaceSummary[] | null>
  spacesError: Ref<string | null>
  /** Documents by space key, once loaded. */
  documents: Record<string, DriveDocument[] | undefined>
  errors: Record<string, string | undefined>
  loadSpaces(): Promise<void>
  /** Loads a space's documents; a loaded space is kept unless `force` is set. */
  load(space: string, force?: boolean): Promise<void>
}

export const driveDataKey: InjectionKey<DriveData> = Symbol('driveData')

function message(cause: unknown, fallback: string): string {
  return cause instanceof Error ? cause.message : fallback
}

/** One cache of the spaces and their document lists, so the nav, folders, and Recent share requests. */
export function createDriveData(): DriveData {
  const spaces = ref<SpaceSummary[] | null>(null)
  const spacesError = ref<string | null>(null)
  const documents = reactive<Record<string, DriveDocument[] | undefined>>({})
  const errors = reactive<Record<string, string | undefined>>({})

  async function loadSpaces(): Promise<void> {
    try {
      spaces.value = await api.spaces()
      spacesError.value = null
    } catch (cause) {
      spacesError.value = message(cause, 'Could not load spaces.')
    }
  }

  async function load(space: string, force = false): Promise<void> {
    if (documents[space] && !force) return
    try {
      documents[space] = await api.documents(spaceFromKey(space).apiPrefix)
      errors[space] = undefined
    } catch (cause) {
      errors[space] = message(cause, 'Could not load documents.')
    }
  }

  return { spaces, spacesError, documents, errors, loadSpaces, load }
}

export function useDriveData(): DriveData {
  const data = inject(driveDataKey)
  if (!data) throw new Error('useDriveData needs the app layout.')
  return data
}
