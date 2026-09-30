// Wire types re-exported from the generated Wails bindings, mirroring the
// types/feed.ts seam: UI code imports from here, never from bindings/.
import type { Integration as WireIntegration } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

export type { WireIntegration }

/**
 * An integration as UI code sees it.
 *
 * The generator types every Go slice as nullable, because a nil one marshals
 * to null. The service never returns nil (TestIntegrationAccountsAreNeverNil
 * pins that), so useIntegrations normalizes at the seam and everything
 * downstream gets a list it can read without a guard.
 */
export type Integration = Omit<WireIntegration, 'accounts'> & { accounts: string[] }
