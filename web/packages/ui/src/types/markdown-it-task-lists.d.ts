// markdown-it-task-lists ships no types; declare the minimal plugin shape.
declare module 'markdown-it-task-lists' {
  import type MarkdownIt from 'markdown-it'
  const taskLists: (md: MarkdownIt, options?: { enabled?: boolean; label?: boolean; labelAfter?: boolean }) => void
  export default taskLists
}
