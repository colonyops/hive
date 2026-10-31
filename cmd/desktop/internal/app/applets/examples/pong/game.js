import { blip } from './sound.js'

// You play the busiest process on this machine. Its CPU use sets how fast it moves.

const WIN_SCORE = 5
const canvas = document.getElementById('court')
const context = canvas.getContext('2d')
const hudYou = document.getElementById('you')
const hudRival = document.getElementById('rival')

const state = {
  you: { y: 0.5, score: 0 },
  rival: { y: 0.5, score: 0, name: 'kernel_task', cpu: 10 },
  ball: { x: 0.5, y: 0.5, vx: 0, vy: 0 },
  keys: new Set(),
  sparks: [],
  wins: Number(localStorage.getItem('wins') ?? 0),
  serving: true,
  over: false,
}

const PADDLE = 0.18
const PADDLE_X = 0.03

function resize() {
  canvas.width = innerWidth * devicePixelRatio
  canvas.height = innerHeight * devicePixelRatio
}
addEventListener('resize', resize)
resize()

function hud() {
  hudYou.textContent = `YOU ${state.you.score}  ·  WINS ${state.wins}`
  hudRival.textContent = `${state.rival.name} (${state.rival.cpu.toFixed(0)}% CPU) ${state.rival.score}`
}

function serve() {
  const direction = Math.random() < 0.5 ? -1 : 1
  state.ball = { x: 0.5, y: 0.5, vx: 0.55 * direction, vy: (Math.random() - 0.5) * 0.7 }
  state.serving = false
  blip(660)
}

function spark(x, y, color) {
  for (let i = 0; i < 18; i++) {
    const angle = Math.random() * Math.PI * 2
    const speed = Math.random() * 0.5
    state.sparks.push({ x, y, vx: Math.cos(angle) * speed, vy: Math.sin(angle) * speed, life: 1, color })
  }
}

async function finish() {
  state.over = true
  state.serving = true
  const won = state.you.score > state.rival.score
  if (won) {
    state.wins++
    localStorage.setItem('wins', String(state.wins))
  }
  const line = `${new Date().toISOString()} ${state.you.score}-${state.rival.score} vs ${state.rival.name}\n`
  const past = await hive.readFile('matches.log').catch(() => '')
  await hive.writeFile('matches.log', past + line)
  await hive.toast(won ? `You beat ${state.rival.name}` : `${state.rival.name} beat you`, {
    severity: won ? 'success' : 'info',
  })
}

function bounce(paddle, side) {
  const { ball } = state
  const offset = (ball.y - paddle.y) / (PADDLE / 2)
  if (Math.abs(offset) > 1) return false
  ball.vx = Math.min(Math.abs(ball.vx) * 1.07, 2.2) * side
  ball.vy = offset * 1.1
  spark(ball.x, ball.y, side > 0 ? '#29f8ff' : '#ff2bd6')
  blip(side > 0 ? 440 : 330)
  return true
}

function step(dt) {
  const { you, rival, ball, keys } = state
  if (keys.has('w') || keys.has('arrowup')) you.y -= dt * 1.3
  if (keys.has('s') || keys.has('arrowdown')) you.y += dt * 1.3
  you.y = Math.max(PADDLE / 2, Math.min(1 - PADDLE / 2, you.y))

  const rivalSpeed = 0.35 + Math.min(state.rival.cpu, 200) / 200
  const gap = ball.y - rival.y
  rival.y += Math.sign(gap) * Math.min(Math.abs(gap), rivalSpeed * dt)

  if (!state.serving) {
    ball.x += ball.vx * dt
    ball.y += ball.vy * dt
    if (ball.y < 0 || ball.y > 1) {
      ball.vy *= -1
      ball.y = Math.max(0, Math.min(1, ball.y))
      blip(220, 0.03)
    }
    if (ball.vx < 0 && ball.x < PADDLE_X && ball.x > 0 && bounce(you, 1)) ball.x = PADDLE_X
    if (ball.vx > 0 && ball.x > 1 - PADDLE_X && ball.x < 1 && bounce(rival, -1)) ball.x = 1 - PADDLE_X
    if (ball.x < -0.02 || ball.x > 1.02) {
      const scorer = ball.x < 0 ? rival : you
      scorer.score++
      spark(0.5, 0.5, '#fff200')
      blip(scorer === you ? 880 : 110, 0.25, 'sawtooth')
      state.serving = true
      ball.x = 0.5
      ball.y = 0.5
      hud()
      if (scorer.score >= WIN_SCORE) void finish()
    }
  }

  for (const s of state.sparks) {
    s.x += s.vx * dt
    s.y += s.vy * dt
    s.life -= dt * 1.6
  }
  state.sparks = state.sparks.filter((s) => s.life > 0)
}

function glowRect(x, y, w, h, color) {
  context.shadowColor = color
  context.shadowBlur = 24 * devicePixelRatio
  context.fillStyle = color
  context.fillRect(x, y, w, h)
  context.shadowBlur = 0
}

function draw(now) {
  const W = canvas.width
  const H = canvas.height
  context.clearRect(0, 0, W, H)

  context.strokeStyle = 'rgb(255 255 255 / 0.05)'
  context.lineWidth = 1
  const cell = 64 * devicePixelRatio
  for (let x = (now / 40) % cell; x < W; x += cell) {
    context.beginPath()
    context.moveTo(x, 0)
    context.lineTo(x, H)
    context.stroke()
  }

  context.setLineDash([12 * devicePixelRatio, 18 * devicePixelRatio])
  context.strokeStyle = 'rgb(255 255 255 / 0.25)'
  context.beginPath()
  context.moveTo(W / 2, 0)
  context.lineTo(W / 2, H)
  context.stroke()
  context.setLineDash([])

  const paddleW = 12 * devicePixelRatio
  const paddleH = H * PADDLE
  glowRect(W * PADDLE_X - paddleW, H * state.you.y - paddleH / 2, paddleW, paddleH, '#29f8ff')
  glowRect(W * (1 - PADDLE_X), H * state.rival.y - paddleH / 2, paddleW, paddleH, '#ff2bd6')

  const size = 14 * devicePixelRatio
  glowRect(W * state.ball.x - size / 2, H * state.ball.y - size / 2, size, size, '#fff200')

  for (const s of state.sparks) {
    context.globalAlpha = s.life
    context.fillStyle = s.color
    context.fillRect(W * s.x, H * s.y, 4 * devicePixelRatio, 4 * devicePixelRatio)
  }
  context.globalAlpha = 1

  if (state.serving) {
    context.textAlign = 'center'
    context.font = `${20 * devicePixelRatio}px "Press Start 2P", monospace`
    context.fillStyle = '#fff'
    const message = state.over ? (state.you.score > state.rival.score ? 'YOU WIN' : 'GAME OVER') : 'PRESS SPACE'
    context.fillText(message, W / 2, H * 0.42)
  }
}

let last = performance.now()
function frame(now) {
  step(Math.min((now - last) / 1000, 0.05))
  draw(now)
  last = now
  requestAnimationFrame(frame)
}
requestAnimationFrame(frame)

addEventListener('mousemove', (event) => {
  state.you.y = event.clientY / innerHeight
})

addEventListener('keydown', (event) => {
  const key = event.key.toLowerCase()
  state.keys.add(key)
  if (key === ' ' && state.serving) {
    if (state.over) {
      state.you.score = 0
      state.rival.score = 0
      state.over = false
      hud()
    }
    serve()
  }
  if (key === 't') void hive.terminal('top')
})
addEventListener('keyup', (event) => state.keys.delete(event.key.toLowerCase()))

// A long-lived process feeding the game: one line of "<cpu> <name>" every two seconds.
const busiest = `while :; do
  ps -Aceo pcpu=,comm= 2>/dev/null | sort -rn | head -1 || true
  sleep 2
done`
let buffered = ''
await hive.spawn(busiest, {
  onStdout(text) {
    buffered += text
    const lines = buffered.split('\n')
    buffered = lines.pop()
    const match = lines.at(-1)?.trim().match(/^([\d.]+)\s+(.+)$/)
    if (!match) return
    state.rival.cpu = Number(match[1])
    state.rival.name = match[2].split('/').pop().slice(0, 24)
    hud()
  },
})
hud()
