export const serverPort = 8941
export const canaryPort = 9141
export const runDir = process.env.DRIVE_E2E_RUN_DIR ?? '/tmp/drive-server-e2e'
export const serverOrigin = `http://127.0.0.1:${serverPort}`
export const canaryOrigin = `http://127.0.0.1:${canaryPort}`
