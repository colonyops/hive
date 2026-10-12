package mcpsrv

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/relay/internal/authctx"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/wire"
)

// noInput is the input of a tool without arguments. The SDK needs a struct so
// the inferred schema has type "object".
type noInput struct{}

type createSourceInput struct {
	Name   string `json:"name"             jsonschema:"Display name, 1 to 80 characters."`
	Secret string `json:"secret,omitempty" jsonschema:"Optional shared secret a sender must present, printable ASCII up to 128 characters. Omit for none."`
}

type updateSourceInput struct {
	ID     string  `json:"id"               jsonschema:"Source id from list_sources."`
	Name   *string `json:"name,omitempty"   jsonschema:"New display name, 1 to 80 characters. Omit to keep it."`
	Secret *string `json:"secret,omitempty" jsonschema:"New shared secret, printable ASCII up to 128 characters. An empty string clears it; omit to keep it."`
}

type sourceIDInput struct {
	ID string `json:"id" jsonschema:"Source id from list_sources."`
}

type listDeliveriesInput struct {
	SourceID string `json:"source_id"       jsonschema:"Source id from list_sources."`
	After    string `json:"after,omitempty" jsonschema:"Cursor: the next value of the previous page, or a delivery id. Omit to start at the oldest stored delivery."`
	Limit    int    `json:"limit,omitempty" jsonschema:"Page size, 1 to 500. Omit for 200."`
}

type createTokenInput struct {
	Name string `json:"name" jsonschema:"Label shown in list_tokens, 1 to 80 characters."`
}

type tokenIDInput struct {
	ID string `json:"id" jsonschema:"Token id from list_tokens."`
}

type setEncryptionInput struct {
	On bool `json:"on" jsonschema:"true seals later deliveries to the account's recipients; false stores them readable again. Stored deliveries keep their form."`
}

type addRecipientInput struct {
	Recipient string `json:"recipient"        jsonschema:"An age X25519 public key, age1..."`
	Device    string `json:"device,omitempty" jsonschema:"Label for the desktop that holds the private key, up to 80 characters."`
}

type recipientIDInput struct {
	ID string `json:"id" jsonschema:"Recipient id from list_recipients."`
}

type deleted struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

func (c *Controller) register(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "whoami",
		Title:       "Who am I",
		Description: "Report the account this key authenticates as: email, whether deliveries are encrypted, the relay's limits, current usage, the sign-in methods it offers, and the server version.",
	}, c.whoami)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_sources",
		Title:       "List sources",
		Description: "List the account's webhook sources, oldest first, with how many deliveries and bytes each holds. Ingress URLs are not listed: the relay keeps only their hash.",
	}, c.listSources)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_source",
		Title:       "Create a source",
		Description: "Create a webhook source and return it with its ingress_url, the URL a sender posts to. The URL is shown only here and by rotate_ingress, so hand it on now.",
	}, c.createSource)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_source",
		Title:       "Update a source",
		Description: "Rename a source or change its shared secret. Fields left out keep their value.",
	}, c.updateSource)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_source",
		Title:       "Delete a source",
		Description: "Delete a source and every delivery it holds. Its ingress URL stops accepting at once.",
	}, c.deleteSource)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "rotate_ingress",
		Title:       "Rotate an ingress URL",
		Description: "Replace a source's ingress URL and return the new one. The old URL stops accepting at once, so every sender must be updated.",
	}, c.rotateIngress)
	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_deliveries",
		Title: "List deliveries",
		Description: "Page through a source's stored deliveries, oldest first, after a cursor. Pass next as after to continue while more is true. " +
			"On an encrypted account only id, received_at, content_length and enc are returned: the relay cannot read the contents.",
	}, c.listDeliveries)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_tokens",
		Title:       "List API keys",
		Description: "List the account's device tokens and API keys. current marks the key this call used, so revoking it ends this session's access.",
	}, c.listTokens)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_token",
		Title:       "Create an API key",
		Description: "Create an API key and return its plaintext. The relay stores only a hash, so the key cannot be read again.",
	}, c.createToken)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "revoke_token",
		Title:       "Revoke an API key",
		Description: "Revoke a device token or API key. It stops authenticating at once; revoking the current one ends this session's access.",
	}, c.revokeToken)
	mcp.AddTool(srv, &mcp.Tool{
		Name:  "set_encryption",
		Title: "Turn encryption on or off",
		Description: "Turn encryption at rest on or off and return the account as whoami does. With it on, the relay seals each later delivery to every registered recipient and cannot read it. " +
			"Turning it on needs at least one recipient. Deliveries already stored keep the form they were written in.",
	}, c.setEncryption)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_recipients",
		Title:       "List recipients",
		Description: "List the age public keys deliveries are sealed to, oldest first. Each is one desktop.",
	}, c.listRecipients)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "add_recipient",
		Title:       "Add a recipient",
		Description: "Register an age X25519 public key that later sealed deliveries open with. Registering a key already listed updates its device label.",
	}, c.addRecipient)
	mcp.AddTool(srv, &mcp.Tool{
		Name:  "remove_recipient",
		Title: "Remove a recipient",
		Description: "Remove a recipient. Later deliveries are not sealed to it; ones already stored are not re-encrypted. " +
			"The last recipient cannot be removed while encryption is on.",
	}, c.removeRecipient)
}

func email(ctx context.Context) string { return authctx.AccountFrom(ctx).Email }

func (c *Controller) whoami(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, wire.Me, error) {
	return c.me(ctx, authctx.AccountFrom(ctx))
}

func (c *Controller) me(ctx context.Context, acct relay.Account) (*mcp.CallToolResult, wire.Me, error) {
	n, err := c.core.Store.Sources().CountByEmail(ctx, acct.Email)
	if err != nil {
		return nil, wire.Me{}, c.toolError(fmt.Errorf("count sources: %w", err))
	}
	return nil, wire.MeFrom(acct, n, c.core.Limits, c.core.SignIn.Methods(), c.core.Version), nil
}

func (c *Controller) setEncryption(ctx context.Context, _ *mcp.CallToolRequest, in setEncryptionInput) (*mcp.CallToolResult, wire.Me, error) {
	acct, err := c.core.Accounts.SetEncryption(ctx, email(ctx), in.On)
	if err != nil {
		return nil, wire.Me{}, c.toolError(err)
	}
	return c.me(ctx, acct)
}

func (c *Controller) listRecipients(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, wire.Recipients, error) {
	rs, err := c.core.Accounts.ListRecipients(ctx, email(ctx))
	if err != nil {
		return nil, wire.Recipients{}, c.toolError(err)
	}
	return nil, wire.RecipientsFrom(rs), nil
}

func (c *Controller) addRecipient(ctx context.Context, _ *mcp.CallToolRequest, in addRecipientInput) (*mcp.CallToolResult, wire.Recipient, error) {
	if err := (wire.CreateRecipient{Recipient: in.Recipient, Device: in.Device}).Validate(); err != nil {
		return nil, wire.Recipient{}, c.toolError(invalid(err))
	}
	r, err := c.core.Accounts.AddRecipient(ctx, email(ctx), in.Recipient, in.Device)
	if err != nil {
		return nil, wire.Recipient{}, c.toolError(err)
	}
	return nil, wire.RecipientFrom(r), nil
}

func (c *Controller) removeRecipient(ctx context.Context, _ *mcp.CallToolRequest, in recipientIDInput) (*mcp.CallToolResult, deleted, error) {
	if err := c.core.Accounts.RemoveRecipient(ctx, email(ctx), in.ID); err != nil {
		return nil, deleted{}, c.toolError(err)
	}
	return nil, deleted{ID: in.ID, Deleted: true}, nil
}

func (c *Controller) listSources(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, wire.Sources, error) {
	views, err := c.core.Sources.List(ctx, email(ctx))
	if err != nil {
		return nil, wire.Sources{}, c.toolError(err)
	}
	return nil, wire.SourcesFrom(views), nil
}

func (c *Controller) createSource(ctx context.Context, _ *mcp.CallToolRequest, in createSourceInput) (*mcp.CallToolResult, wire.CreatedSource, error) {
	if err := (wire.CreateSource{Name: in.Name, Secret: in.Secret}).Validate(); err != nil {
		return nil, wire.CreatedSource{}, c.toolError(invalid(err))
	}
	src, token, err := c.core.Sources.Create(ctx, email(ctx), in.Name, in.Secret)
	if err != nil {
		return nil, wire.CreatedSource{}, c.toolError(err)
	}
	return nil, wire.CreatedSource{
		Source:     wire.SourceFrom(relay.SourceView{Source: src}),
		IngressURL: c.core.Sources.IngressURL(token),
	}, nil
}

func (c *Controller) updateSource(ctx context.Context, _ *mcp.CallToolRequest, in updateSourceInput) (*mcp.CallToolResult, wire.Source, error) {
	if err := (wire.UpdateSource{Name: in.Name, Secret: in.Secret}).Validate(); err != nil {
		return nil, wire.Source{}, c.toolError(invalid(err))
	}
	if _, err := c.core.Sources.Update(ctx, email(ctx), in.ID, in.Name, in.Secret); err != nil {
		return nil, wire.Source{}, c.toolError(err)
	}
	v, err := c.core.Sources.Get(ctx, email(ctx), in.ID)
	if err != nil {
		return nil, wire.Source{}, c.toolError(err)
	}
	return nil, wire.SourceFrom(v), nil
}

func (c *Controller) deleteSource(ctx context.Context, _ *mcp.CallToolRequest, in sourceIDInput) (*mcp.CallToolResult, deleted, error) {
	if err := c.core.Sources.Delete(ctx, email(ctx), in.ID); err != nil {
		return nil, deleted{}, c.toolError(err)
	}
	return nil, deleted{ID: in.ID, Deleted: true}, nil
}

func (c *Controller) rotateIngress(ctx context.Context, _ *mcp.CallToolRequest, in sourceIDInput) (*mcp.CallToolResult, wire.IngressURL, error) {
	token, err := c.core.Sources.Rotate(ctx, email(ctx), in.ID)
	if err != nil {
		return nil, wire.IngressURL{}, c.toolError(err)
	}
	return nil, wire.IngressURL{IngressURL: c.core.Sources.IngressURL(token)}, nil
}

func (c *Controller) listDeliveries(ctx context.Context, _ *mcp.CallToolRequest, in listDeliveriesInput) (*mcp.CallToolResult, relay.Page, error) {
	q := wire.DeliveriesQuery{After: in.After, Limit: in.Limit}
	if err := q.Validate(); err != nil {
		return nil, relay.Page{}, c.toolError(invalid(err))
	}
	acct := authctx.AccountFrom(ctx)
	page, err := c.core.Feed.ListAfter(ctx, acct.Email, in.SourceID, q.After, q.PageSize())
	if err != nil {
		return nil, relay.Page{}, c.toolError(err)
	}
	if acct.Encryption {
		for i, d := range page.Deliveries {
			page.Deliveries[i] = relay.DeliveryView{ID: d.ID, ReceivedAt: d.ReceivedAt, ContentLength: d.ContentLength, Enc: d.Enc}
		}
	}
	return nil, page, nil
}

func (c *Controller) listTokens(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, wire.Tokens, error) {
	toks, err := c.core.Auth.ListTokens(ctx, email(ctx))
	if err != nil {
		return nil, wire.Tokens{}, c.toolError(err)
	}
	return nil, wire.TokensFrom(toks, authctx.TokenFrom(ctx).ID), nil
}

func (c *Controller) createToken(ctx context.Context, _ *mcp.CallToolRequest, in createTokenInput) (*mcp.CallToolResult, wire.CreatedToken, error) {
	if err := (wire.CreateToken{Name: in.Name}).Validate(); err != nil {
		return nil, wire.CreatedToken{}, c.toolError(invalid(err))
	}
	plaintext, tok, err := c.core.Auth.MintToken(ctx, email(ctx), in.Name, relay.TokenAPI)
	if err != nil {
		return nil, wire.CreatedToken{}, c.toolError(err)
	}
	return nil, wire.CreatedTokenFrom(plaintext, tok), nil
}

func (c *Controller) revokeToken(ctx context.Context, _ *mcp.CallToolRequest, in tokenIDInput) (*mcp.CallToolResult, deleted, error) {
	if err := c.core.Auth.RevokeToken(ctx, email(ctx), in.ID); err != nil {
		return nil, deleted{}, c.toolError(err)
	}
	return nil, deleted{ID: in.ID, Deleted: true}, nil
}
