import { reactive } from 'vue'
import { defineStore } from './defineStore'

// What the user has marked viewed, per session, as the file version they saw.
// A later edit moves the version, so the file comes back unviewed. It lives for
// the app's run: the mark is about this sitting's work, which a relaunch ends.
export const useChangesViewed = defineStore('changesViewed', () => {
  const sessions = reactive(new Map<string, Map<string, string>>())

  function isViewed(sessionId: string, path: string, version: string): boolean {
    return sessions.get(sessionId)?.get(path) === version
  }

  function setViewed(sessionId: string, path: string, version: string, viewed: boolean): void {
    let paths = sessions.get(sessionId)
    if (!paths) {
      paths = new Map()
      sessions.set(sessionId, paths)
    }
    if (viewed) paths.set(path, version)
    else paths.delete(path)
  }

  return { isViewed, setViewed }
})
