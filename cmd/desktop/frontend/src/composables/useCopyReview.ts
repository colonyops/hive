import { useChangesReview, type ChangesComment } from '../stores/useChangesReview'
import { useToasts } from './useToasts'

/** Copies review comments as the text an agent would get, and says how that went. */
export function useCopyReview(): (comments: readonly ChangesComment[]) => Promise<void> {
  const { showToast } = useToasts()
  const store = useChangesReview()
  return async (comments) => {
    const failure = await store.copy(comments)
    if (failure) showToast(failure, { severity: 'error' })
    else showToast(comments.length === 1 ? 'Copied 1 comment' : `Copied ${comments.length} comments`)
  }
}
