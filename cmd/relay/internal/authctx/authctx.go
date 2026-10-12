// Package authctx carries the authenticated account and token on a request
// context, so the HTTP API and the MCP server read them the same way.
package authctx

import (
	"context"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

type ctxKey int

const (
	accountKey ctxKey = iota
	tokenKey
)

func WithAccount(ctx context.Context, acct relay.Account, tok relay.Token) context.Context {
	ctx = context.WithValue(ctx, accountKey, acct)
	return context.WithValue(ctx, tokenKey, tok)
}

func AccountFrom(ctx context.Context) relay.Account {
	a, _ := ctx.Value(accountKey).(relay.Account)
	return a
}

func TokenFrom(ctx context.Context) relay.Token {
	t, _ := ctx.Value(tokenKey).(relay.Token)
	return t
}
