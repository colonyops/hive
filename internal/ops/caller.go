package ops

// CallerRule says which callers an operation with a caller input accepts. A
// chat caller passes both rules on a desktop-owned operation, because the
// desktop resolves chats.
//
// ENUM(none, session, session-or-global)
type CallerRule string
