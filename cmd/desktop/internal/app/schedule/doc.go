// Package schedule launches recurring agent chats. A Spec is one entry in a
// workspace manifest's schedules: list; a Run is one execution of it.
//
// The package is a leaf: cron parsing, prompt templating, the planner and the
// loop are testable without tmux, a database, or the agentws package that
// imports this one. Everything the Scheduler needs from the rest of the app
// arrives through the ports declared in scheduler.go.
//
// Evaluate looks at the window between the cursor and now rather than at a
// timer, so a schedule due while the app was closed fires on the next launch.
package schedule
