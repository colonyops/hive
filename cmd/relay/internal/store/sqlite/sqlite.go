// Package sqlite is the relay.Store on one SQLite file, through sqlc queries.
package sqlite

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/store/sqlite/db"
	platform "github.com/colonyops/hive/internal/platform/sqlite"
	"github.com/colonyops/hive/internal/store/migrate"
)

//go:embed migrations/*.up.sql
var migrationsFS embed.FS

type store struct {
	conn *sql.DB
	q    *db.Queries
}

// Open opens or creates the database at path and applies pending migrations.
func Open(ctx context.Context, path string) (relay.Store, error) {
	// One connection makes this process a single writer, so the atomic cap in
	// Append never races another connection here. Other processes (the
	// operator commands) still contend on the file: _txlock=immediate and
	// busy_timeout make them wait instead of failing.
	conn, err := platform.Open(ctx, path, platform.Options{
		MaxOpenConns: 1,
		MaxIdleConns: 1,
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err == nil {
		err = migrate.Up(ctx, conn, sub)
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("sqlite: migrate: %w", err), conn.Close())
	}
	return &store{conn: conn, q: db.New(conn)}, nil
}

func (s *store) Accounts() relay.AccountStore     { return accounts{s} }
func (s *store) Tokens() relay.TokenStore         { return tokens{s} }
func (s *store) Sources() relay.SourceStore       { return sources{s} }
func (s *store) Deliveries() relay.DeliveryStore  { return deliveries{s} }
func (s *store) AuthCodes() relay.AuthCodeStore   { return authCodes{s} }
func (s *store) Outbox() relay.OutboxStore        { return outbox{s} }
func (s *store) Recipients() relay.RecipientStore { return recipients{s} }
func (s *store) Close() error                     { return s.conn.Close() }

// tx runs fn in one write transaction with queries bound to it. With a single
// pooled connection, a query on s.q inside fn would wait forever for the
// connection fn holds, so fn must use the q it is given.
func (s *store) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	return platform.WithinTx(ctx, s.conn, func(_ context.Context, tx *sql.Tx) error {
		return fn(s.q.WithTx(tx))
	})
}

// wrap maps sql.ErrNoRows to relay.ErrNotFound and gives any other driver
// error the operation's context.
func wrap(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return relay.ErrNotFound
	default:
		return fmt.Errorf("sqlite: %s: %w", op, err)
	}
}

func notFoundUnless(op string, rows int64, err error) error {
	if err != nil {
		return wrap(op, err)
	}
	if rows == 0 {
		return relay.ErrNotFound
	}
	return nil
}

// Zero times store as 0 so a zero time.Time survives the round trip;
// UnixNano of the zero time overflows int64.
func toNanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func fromNanos(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

func toNullNanos(t time.Time) sql.NullInt64 {
	return sql.NullInt64{Int64: t.UnixNano(), Valid: !t.IsZero()}
}

func fromNullNanos(n sql.NullInt64) time.Time {
	if !n.Valid {
		return time.Time{}
	}
	return fromNanos(n.Int64)
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func sum(sizes []int64) int64 {
	var total int64
	for _, n := range sizes {
		total += n
	}
	return total
}

type accounts struct{ s *store }

func toAccount(a db.Account) relay.Account {
	return relay.Account{
		Email:      a.Email,
		Encryption: a.Encryption != 0,
		BytesUsed:  a.BytesUsed,
		CreatedAt:  fromNanos(a.CreatedAt),
	}
}

func (a accounts) Get(ctx context.Context, email string) (relay.Account, error) {
	row, err := a.s.q.GetAccount(ctx, email)
	if err != nil {
		return relay.Account{}, wrap("get account", err)
	}
	return toAccount(row), nil
}

func (a accounts) Put(ctx context.Context, acct relay.Account) error {
	return wrap("put account", a.s.q.PutAccount(ctx, db.PutAccountParams{
		Email:      acct.Email,
		Encryption: boolInt(acct.Encryption),
		BytesUsed:  acct.BytesUsed,
		CreatedAt:  toNanos(acct.CreatedAt),
	}))
}

func (a accounts) Delete(ctx context.Context, email string) error {
	n, err := a.s.q.DeleteAccount(ctx, email)
	return notFoundUnless("delete account", n, err)
}

func (a accounts) List(ctx context.Context) ([]relay.Account, error) {
	rows, err := a.s.q.ListAccounts(ctx)
	if err != nil {
		return nil, wrap("list accounts", err)
	}
	out := make([]relay.Account, len(rows))
	for i, r := range rows {
		out[i] = toAccount(r)
	}
	return out, nil
}

func (a accounts) AddBytes(ctx context.Context, email string, delta int64) (int64, error) {
	n, err := a.s.q.AddAccountBytes(ctx, db.AddAccountBytesParams{Delta: delta, Email: email})
	if err != nil {
		return 0, wrap("add account bytes", err)
	}
	return n, nil
}

type tokens struct{ s *store }

func toToken(t db.Token) relay.Token {
	return relay.Token{
		ID:         t.ID,
		Email:      t.Email,
		Hash:       t.Hash,
		Kind:       relay.TokenKind(t.Kind),
		Device:     t.Device,
		CreatedAt:  fromNanos(t.CreatedAt),
		LastUsedAt: fromNullNanos(t.LastUsedAt),
	}
}

func (t tokens) GetByHash(ctx context.Context, hash string) (relay.Token, error) {
	row, err := t.s.q.GetTokenByHash(ctx, hash)
	if err != nil {
		return relay.Token{}, wrap("get token", err)
	}
	return toToken(row), nil
}

func (t tokens) Put(ctx context.Context, tok relay.Token) error {
	return wrap("put token", t.s.q.PutToken(ctx, db.PutTokenParams{
		ID:         tok.ID,
		Email:      tok.Email,
		Hash:       tok.Hash,
		Kind:       string(tok.Kind),
		Device:     tok.Device,
		CreatedAt:  toNanos(tok.CreatedAt),
		LastUsedAt: toNullNanos(tok.LastUsedAt),
	}))
}

func (t tokens) Delete(ctx context.Context, email, id string) error {
	n, err := t.s.q.DeleteToken(ctx, db.DeleteTokenParams{Email: email, ID: id})
	return notFoundUnless("delete token", n, err)
}

func (t tokens) ListByEmail(ctx context.Context, email string) ([]relay.Token, error) {
	rows, err := t.s.q.ListTokensByEmail(ctx, email)
	if err != nil {
		return nil, wrap("list tokens", err)
	}
	out := make([]relay.Token, len(rows))
	for i, r := range rows {
		out[i] = toToken(r)
	}
	return out, nil
}

func (t tokens) Touch(ctx context.Context, id string, at time.Time) error {
	n, err := t.s.q.TouchToken(ctx, db.TouchTokenParams{LastUsedAt: toNullNanos(at), ID: id})
	return notFoundUnless("touch token", n, err)
}

type sources struct{ s *store }

func toSource(r db.Source) relay.Source {
	return relay.Source{
		ID:          r.ID,
		Email:       r.Email,
		Name:        r.Name,
		IngressHash: r.IngressHash,
		SecretHash:  r.SecretHash,
		CreatedAt:   fromNanos(r.CreatedAt),
	}
}

func (r sources) Get(ctx context.Context, email, id string) (relay.Source, error) {
	row, err := r.s.q.GetSource(ctx, db.GetSourceParams{Email: email, ID: id})
	if err != nil {
		return relay.Source{}, wrap("get source", err)
	}
	return toSource(row), nil
}

func (r sources) GetByIngressHash(ctx context.Context, hash string) (relay.Source, error) {
	row, err := r.s.q.GetSourceByIngressHash(ctx, hash)
	if err != nil {
		return relay.Source{}, wrap("get source by ingress", err)
	}
	return toSource(row), nil
}

func (r sources) Put(ctx context.Context, src relay.Source) error {
	return wrap("put source", r.s.q.PutSource(ctx, db.PutSourceParams{
		ID:          src.ID,
		Email:       src.Email,
		Name:        src.Name,
		IngressHash: src.IngressHash,
		SecretHash:  src.SecretHash,
		CreatedAt:   toNanos(src.CreatedAt),
	}))
}

func (r sources) Delete(ctx context.Context, email, id string) error {
	return r.s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.GetSource(ctx, db.GetSourceParams{Email: email, ID: id}); err != nil {
			return wrap("delete source", err)
		}
		stats, err := q.SourceDeliveryStats(ctx, id)
		if err != nil {
			return wrap("delete source", err)
		}
		if _, err := q.DeleteSource(ctx, db.DeleteSourceParams{Email: email, ID: id}); err != nil {
			return wrap("delete source", err)
		}
		return credit(ctx, q, email, stats.Bytes)
	})
}

func (r sources) ListByEmail(ctx context.Context, email string) ([]relay.Source, error) {
	rows, err := r.s.q.ListSourcesByEmail(ctx, email)
	if err != nil {
		return nil, wrap("list sources", err)
	}
	out := make([]relay.Source, len(rows))
	for i, row := range rows {
		out[i] = toSource(row)
	}
	return out, nil
}

func (r sources) CountByEmail(ctx context.Context, email string) (int, error) {
	n, err := r.s.q.CountSourcesByEmail(ctx, email)
	if err != nil {
		return 0, wrap("count sources", err)
	}
	return int(n), nil
}

func credit(ctx context.Context, q *db.Queries, email string, bytes int64) error {
	if bytes == 0 {
		return nil
	}
	_, err := q.AddAccountBytes(ctx, db.AddAccountBytesParams{Delta: -bytes, Email: email})
	return wrap("credit account bytes", err)
}

type deliveries struct{ s *store }

func (r deliveries) Append(ctx context.Context, email string, d relay.Delivery, maxAccountBytes int64, keepPerSource int) error {
	return r.s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.GetSource(ctx, db.GetSourceParams{Email: email, ID: d.SourceID}); err != nil {
			return wrap("append: get source", err)
		}
		used, err := q.AddAccountBytes(ctx, db.AddAccountBytesParams{Delta: d.Size, Email: email})
		if err != nil {
			return wrap("append: debit account", err)
		}
		if used > maxAccountBytes {
			return &relay.Error{
				Kind:    relay.KindOverCap,
				Message: fmt.Sprintf("account is at its cap of %d bytes", maxAccountBytes),
			}
		}
		err = q.InsertDelivery(ctx, db.InsertDeliveryParams{
			ID:         d.ID,
			SourceID:   d.SourceID,
			ReceivedAt: toNanos(d.ReceivedAt),
			Enc:        boolInt(d.Enc),
			Size:       d.Size,
			Record:     d.Record,
		})
		if err != nil {
			return wrap("append: insert delivery", err)
		}
		if keepPerSource <= 0 {
			return nil
		}
		_, _, err = trim(ctx, q, email, d.SourceID, keepPerSource)
		return err
	})
}

func trim(ctx context.Context, q *db.Queries, email, sourceID string, keep int) (int, int64, error) {
	sizes, err := q.DeleteDeliveriesBeyondNewest(ctx, db.DeleteDeliveriesBeyondNewestParams{
		SourceID: sourceID,
		Keep:     int64(max(keep, 0)),
	})
	if err != nil {
		return 0, 0, wrap("trim deliveries", err)
	}
	freed := sum(sizes)
	if err := credit(ctx, q, email, freed); err != nil {
		return 0, 0, err
	}
	return len(sizes), freed, nil
}

func (r deliveries) ListAfter(ctx context.Context, sourceID, afterID string, limit int) ([]relay.Delivery, bool, error) {
	maxRows := int64(-1)
	if limit >= 0 {
		maxRows = int64(limit) + 1
	}
	rows, err := r.s.q.ListDeliveriesAfter(ctx, db.ListDeliveriesAfterParams{
		SourceID: sourceID,
		AfterID:  afterID,
		MaxRows:  maxRows,
	})
	if err != nil {
		return nil, false, wrap("list deliveries", err)
	}
	more := false
	if limit >= 0 && len(rows) > limit {
		rows, more = rows[:limit], true
	}
	out := make([]relay.Delivery, len(rows))
	for i, row := range rows {
		out[i] = relay.Delivery{
			ID:         row.ID,
			SourceID:   row.SourceID,
			ReceivedAt: fromNanos(row.ReceivedAt),
			Enc:        row.Enc != 0,
			Size:       row.Size,
			Record:     row.Record,
		}
	}
	return out, more, nil
}

func (r deliveries) DeleteBefore(ctx context.Context, email, sourceID string, before time.Time) (int, int64, error) {
	var (
		n     int
		freed int64
	)
	err := r.s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.GetSource(ctx, db.GetSourceParams{Email: email, ID: sourceID}); err != nil {
			return wrap("delete before: get source", err)
		}
		sizes, err := q.DeleteDeliveriesBefore(ctx, db.DeleteDeliveriesBeforeParams{
			SourceID: sourceID,
			Before:   toNanos(before),
		})
		if err != nil {
			return wrap("delete before", err)
		}
		n, freed = len(sizes), sum(sizes)
		return credit(ctx, q, email, freed)
	})
	if err != nil {
		return 0, 0, err
	}
	return n, freed, nil
}

func (r deliveries) TrimTo(ctx context.Context, email, sourceID string, keep int) (int, int64, error) {
	var (
		n     int
		freed int64
	)
	err := r.s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.GetSource(ctx, db.GetSourceParams{Email: email, ID: sourceID}); err != nil {
			return wrap("trim: get source", err)
		}
		var err error
		n, freed, err = trim(ctx, q, email, sourceID, keep)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	return n, freed, nil
}

func (r deliveries) Stats(ctx context.Context, sourceID string) (int, int64, error) {
	row, err := r.s.q.SourceDeliveryStats(ctx, sourceID)
	if err != nil {
		return 0, 0, wrap("delivery stats", err)
	}
	return int(row.Count), row.Bytes, nil
}

type authCodes struct{ s *store }

func (a authCodes) Put(ctx context.Context, email, hash string, expires time.Time) error {
	return wrap("put auth code", a.s.q.PutAuthCode(ctx, db.PutAuthCodeParams{
		Email:     email,
		Hash:      hash,
		ExpiresAt: toNanos(expires),
	}))
}

func (a authCodes) Consume(ctx context.Context, email, hash string, now time.Time, maxAttempts int) (bool, int, error) {
	var (
		ok   bool
		left int
	)
	err := a.s.tx(ctx, func(q *db.Queries) error {
		row, err := q.GetAuthCode(ctx, email)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return wrap("consume auth code", err)
		}
		if !now.Before(fromNanos(row.ExpiresAt)) {
			return wrap("consume auth code: delete expired", q.DeleteAuthCode(ctx, email))
		}
		if subtle.ConstantTimeCompare([]byte(row.Hash), []byte(hash)) == 1 {
			ok = true
			return wrap("consume auth code: delete", q.DeleteAuthCode(ctx, email))
		}
		attempts, err := q.IncrementAuthCodeAttempts(ctx, email)
		if err != nil {
			return wrap("consume auth code: count attempt", err)
		}
		left = maxAttempts - int(attempts)
		if left <= 0 {
			left = 0
			return wrap("consume auth code: delete spent", q.DeleteAuthCode(ctx, email))
		}
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	return ok, left, nil
}

func (a authCodes) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	n, err := a.s.q.DeleteExpiredAuthCodes(ctx, toNanos(now))
	if err != nil {
		return 0, wrap("delete expired auth codes", err)
	}
	return int(n), nil
}

type outbox struct{ s *store }

func (o outbox) Append(ctx context.Context, m relay.Mail) error {
	return wrap("append mail", o.s.q.AppendMail(ctx, db.AppendMailParams{
		ID:        m.ID,
		Recipient: m.To,
		Subject:   m.Subject,
		Body:      m.Body,
		CreatedAt: toNanos(m.CreatedAt),
		NextAt:    toNanos(m.NextAt),
		Attempts:  int64(m.Attempts),
		SentAt:    toNullNanos(m.SentAt),
	}))
}

func (o outbox) Due(ctx context.Context, now time.Time, limit int) ([]relay.Mail, error) {
	rows, err := o.s.q.DueMail(ctx, db.DueMailParams{Now: toNanos(now), MaxRows: int64(limit)})
	if err != nil {
		return nil, wrap("due mail", err)
	}
	out := make([]relay.Mail, len(rows))
	for i, r := range rows {
		out[i] = relay.Mail{
			ID:        r.ID,
			To:        r.Recipient,
			Subject:   r.Subject,
			Body:      r.Body,
			CreatedAt: fromNanos(r.CreatedAt),
			NextAt:    fromNanos(r.NextAt),
			Attempts:  int(r.Attempts),
			SentAt:    fromNullNanos(r.SentAt),
		}
	}
	return out, nil
}

func (o outbox) MarkSent(ctx context.Context, id string, at time.Time) error {
	n, err := o.s.q.MarkMailSent(ctx, db.MarkMailSentParams{SentAt: toNullNanos(at), ID: id})
	return notFoundUnless("mark mail sent", n, err)
}

func (o outbox) MarkFailed(ctx context.Context, id string, next time.Time) error {
	n, err := o.s.q.MarkMailFailed(ctx, db.MarkMailFailedParams{NextAt: toNanos(next), ID: id})
	return notFoundUnless("mark mail failed", n, err)
}

type recipients struct{ s *store }

func (r recipients) Put(ctx context.Context, rec relay.Recipient) error {
	return wrap("put recipient", r.s.q.PutRecipient(ctx, db.PutRecipientParams{
		ID:        rec.ID,
		Email:     rec.Email,
		Recipient: rec.Recipient,
		Device:    rec.Device,
		CreatedAt: toNanos(rec.CreatedAt),
	}))
}

func (r recipients) Delete(ctx context.Context, email, id string) error {
	n, err := r.s.q.DeleteRecipient(ctx, db.DeleteRecipientParams{Email: email, ID: id})
	return notFoundUnless("delete recipient", n, err)
}

func (r recipients) ListByEmail(ctx context.Context, email string) ([]relay.Recipient, error) {
	rows, err := r.s.q.ListRecipientsByEmail(ctx, email)
	if err != nil {
		return nil, wrap("list recipients", err)
	}
	out := make([]relay.Recipient, len(rows))
	for i, row := range rows {
		out[i] = relay.Recipient{
			ID:        row.ID,
			Email:     row.Email,
			Recipient: row.Recipient,
			Device:    row.Device,
			CreatedAt: fromNanos(row.CreatedAt),
		}
	}
	return out, nil
}
