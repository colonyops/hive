package relay

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// RecipientView is one age recipient registered on the account, one per
// desktop.
type RecipientView struct {
	ID        string    `json:"id"`
	Recipient string    `json:"recipient"`
	Device    string    `json:"device"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *Client) ListRecipients(ctx context.Context) ([]RecipientView, error) {
	var out struct {
		Recipients []RecipientView `json:"recipients"`
	}
	if err := c.call(ctx, http.MethodGet, "/v1/recipients", nil, &out); err != nil {
		return nil, err
	}
	return out.Recipients, nil
}

// AddRecipient is idempotent on the relay: registering a known recipient
// returns its existing row.
func (c *Client) AddRecipient(ctx context.Context, recipient, device string) (RecipientView, error) {
	var out RecipientView
	body := map[string]string{"recipient": recipient, "device": device}
	return out, c.call(ctx, http.MethodPost, "/v1/recipients", body, &out)
}

func (c *Client) RemoveRecipient(ctx context.Context, id string) error {
	return c.call(ctx, http.MethodDelete, "/v1/recipients/"+url.PathEscape(id), nil, nil)
}

// SetEncryption turns sealing on or off for later deliveries. The relay
// refuses "on" with ErrInvalid while the account has no recipient.
func (c *Client) SetEncryption(ctx context.Context, on bool) (Me, error) {
	state := "off"
	if on {
		state = "on"
	}
	var out Me
	return out, c.call(ctx, http.MethodPatch, "/v1/me", map[string]string{"encryption": state}, &out)
}
