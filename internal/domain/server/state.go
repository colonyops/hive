package server

// State is how a client last found a server. Connecting means no check has
// answered yet. Incompatible means the daemon answered with a version this
// client cannot use.
//
// ENUM(connecting, connected, offline, incompatible)
type State string
