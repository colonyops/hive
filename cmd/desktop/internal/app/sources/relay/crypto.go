package relay

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"

	"filippo.io/age"
	"filippo.io/age/armor"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relayformat"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// AgeProvider names the keychain entry holding an account's age identity. It
// shares the account half with the relay credential.
const AgeProvider = "relay-age"

const (
	ageMagic = "age-encryption.org/v1"
	// maxOpened bounds a decrypted record: the relay caps a body at 256 KiB,
	// and a record carries it base64-encoded beside the headers.
	maxOpened = 4 << 20

	placeholderTitle = "Encrypted delivery, no key on this device"
	placeholderKind  = "Encrypted"
)

// errUnopenable marks a delivery this device holds no key for. It is shown as
// a placeholder item rather than dropped, so the gap is visible.
var errUnopenable = errors.New("relay: delivery cannot be opened on this device")

func ageRef(account string) credentials.Ref {
	return credentials.Ref{Provider: AgeProvider, Account: account}
}

// record mirrors the relay's Record, the plaintext of a sealed delivery.
type record struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	Query       url.Values          `json:"query"`
	Headers     map[string][]string `json:"headers"`
	Body        []byte              `json:"body"`
	ContentType string              `json:"content_type"`
}

// open decrypts an age file, binary or armored, with identity.
func open(identity *age.X25519Identity, blob []byte) ([]byte, error) {
	if identity == nil {
		return nil, errUnopenable
	}
	var src io.Reader = bytes.NewReader(blob)
	if bytes.HasPrefix(blob, []byte(armor.Header)) {
		src = armor.NewReader(src)
	}
	plain, err := age.Decrypt(src, identity)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUnopenable, err)
	}
	out, err := io.ReadAll(io.LimitReader(plain, maxOpened+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUnopenable, err)
	}
	if len(out) > maxOpened {
		return nil, fmt.Errorf("%w: decrypted delivery exceeds %d bytes", errUnopenable, maxOpened)
	}
	return out, nil
}

func senderSealed(body []byte) bool {
	return bytes.HasPrefix(body, []byte(ageMagic)) || bytes.HasPrefix(body, []byte(armor.Header))
}

// decode turns a wire row into a delivery, opening a relay-sealed record or a
// sender-sealed body with identity. A row it cannot open returns
// errUnopenable; any other error is a malformed row.
func decode(identity *age.X25519Identity, view DeliveryView) (relayformat.Delivery, error) {
	var (
		d   relayformat.Delivery
		err error
	)
	if view.Enc != "" {
		d, err = openRecord(identity, view)
	} else {
		d, err = toDelivery(view)
	}
	if err != nil {
		return relayformat.Delivery{}, err
	}
	if senderSealed(d.Body) {
		body, err := open(identity, d.Body)
		if err != nil {
			return relayformat.Delivery{}, err
		}
		d.Body = body
	}
	return d, nil
}

func openRecord(identity *age.X25519Identity, view DeliveryView) (relayformat.Delivery, error) {
	if view.Enc != "age" {
		return relayformat.Delivery{}, fmt.Errorf("%w: unknown encryption %q", errUnopenable, view.Enc)
	}
	blob, err := base64.StdEncoding.DecodeString(view.Blob)
	if err != nil {
		return relayformat.Delivery{}, fmt.Errorf("%w: decode blob: %w", errUnopenable, err)
	}
	plain, err := open(identity, blob)
	if err != nil {
		return relayformat.Delivery{}, err
	}
	var rec record
	if err := json.Unmarshal(plain, &rec); err != nil {
		return relayformat.Delivery{}, fmt.Errorf("%w: decode record: %w", errUnopenable, err)
	}
	return relayformat.Delivery{
		ID:          view.ID,
		ReceivedAt:  view.ReceivedAt,
		Method:      rec.Method,
		Path:        rec.Path,
		Query:       rec.Query,
		Headers:     rec.Headers,
		Body:        rec.Body,
		ContentType: rec.ContentType,
	}, nil
}

func placeholder(view DeliveryView) item {
	return item{
		payload:    map[string]any{"id": view.ID, "title": placeholderTitle, "kind": placeholderKind},
		receivedAt: view.ReceivedAt,
	}
}
