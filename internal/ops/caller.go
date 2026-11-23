package ops

// CallerRule says which callers an operation with a caller input accepts. A
// chat caller passes both rules: the desktop resolves chats for its own
// operations, and an engine operation acts for the chat's id.
//
// ENUM(none, session, session-or-global)
type CallerRule string
