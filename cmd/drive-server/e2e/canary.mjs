// Stands in for an attacker's server. Anything that reaches it, over HTTP or
// as a bare TCP connection, is an escape from the document frame.
import { createServer } from 'node:http'

const port = Number(process.env.DRIVE_E2E_CANARY_PORT)
const hits = []
let connections = 0
let harnessRequests = 0

const server = createServer((req, res) => {
  if (req.url === '/__hits' || req.url === '/__reset') {
    // The test opens one connection per control request, so the surplus
    // connections are the ones a document opened.
    harnessRequests++
    if (req.url === '/__reset') {
      hits.length = 0
      connections = 0
      harnessRequests = 0
    }
    res.setHeader('content-type', 'application/json')
    res.end(JSON.stringify({ hits, strayConnections: connections - harnessRequests }))
    return
  }
  hits.push({ kind: 'http', method: req.method, url: req.url })
  // Permissive CORS so a request that gets through also succeeds visibly in
  // the document, instead of failing on a missing header and looking blocked.
  res.setHeader('access-control-allow-origin', '*')
  res.setHeader('content-type', 'text/css')
  res.end('/* canary */')
})
server.on('upgrade', (req, socket) => {
  hits.push({ kind: 'upgrade', url: req.url })
  socket.destroy()
})
server.on('connection', () => {
  connections++
})
server.listen(port, '127.0.0.1')
