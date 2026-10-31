//go:build server

package app

// The headless server's asset handler and Wails runtime are an unauthenticated TCP
// listener, so serving pages here would hand a command runner and an applet's files
// to anything that can reach the port, including any website through a browser
// (ADR applets-are-declared-in-yaml-and-rendered-natively).
const buildServesAppletPages = false
