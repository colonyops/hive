package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/relay/internal/guard"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/internal/web/extractors"
)

const (
	maxEmail     = 254
	maxPassword  = 1024
	maxDevice    = 80
	authLimitKey = 10_000
	authIdle     = 2 * time.Hour
)

var codeRE = regexp.MustCompile(`^\d{8}$`)

// authLimits guards the unauthenticated sign-in routes. Buckets are per route,
// so asking for a code does not spend the attempts that verifying it needs.
type authLimits struct {
	perEmail *guard.Limiter
	perIP    *guard.Limiter
}

func newAuthLimits(l relay.Limits) authLimits {
	return authLimits{
		perEmail: guard.NewLimiter(l.AuthPerEmail, authLimitKey, authIdle, nil),
		perIP:    guard.NewLimiter(l.AuthPerIP, authLimitKey, authIdle, nil),
	}
}

func rateLimited(wait time.Duration) error {
	return &relay.Error{Kind: relay.KindRateLimited, Message: "too many sign-in attempts", RetryAfter: wait}
}

func (s *api) allowAuthIP(r *http.Request, route string) error {
	ip := guard.ClientIP(r, s.trusted)
	if ok, wait := s.authLimits.perIP.Allow(route + "|" + ip.String()); !ok {
		return rateLimited(wait)
	}
	return nil
}

func (s *api) allowAuthEmail(route, email string) error {
	if ok, wait := s.authLimits.perEmail.Allow(route + "|" + relay.NormalizeEmail(email)); !ok {
		return rateLimited(wait)
	}
	return nil
}

func validEmail(email string) error {
	return criterio.Run("email", strings.TrimSpace(email), criterio.StrMax(maxEmail), criterio.StrEmail)
}

func validDevice(device string) error {
	return criterio.Run("device", strings.TrimSpace(device), criterio.StrMax(maxDevice))
}

type startRequest struct {
	Email string `json:"email"`
}

func (b startRequest) Validate() error { return validEmail(b.Email) }

type verifyRequest struct {
	Email  string `json:"email"`
	Code   string `json:"code"`
	Device string `json:"device"`
}

func (b verifyRequest) Validate() error {
	return criterio.ValidateStruct(
		validEmail(b.Email),
		criterio.Run("code", strings.TrimSpace(b.Code), criterio.StrMatches(codeRE)),
		validDevice(b.Device),
	)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Device   string `json:"device"`
}

func (b loginRequest) Validate() error {
	return criterio.ValidateStruct(
		validEmail(b.Email),
		criterio.Run("password", b.Password, criterio.StrBetween(1, maxPassword)),
		validDevice(b.Device),
	)
}

type signedInResponse struct {
	Token     string          `json:"token"`
	ID        string          `json:"id"`
	Email     string          `json:"email"`
	Kind      relay.TokenKind `json:"kind"`
	Device    string          `json:"device"`
	CreatedAt time.Time       `json:"created_at"`
}

func signedIn(w http.ResponseWriter, plaintext string, tok relay.Token) error {
	return server.JSON(w, http.StatusOK, signedInResponse{
		Token: plaintext, ID: tok.ID, Email: tok.Email, Kind: tok.Kind, Device: tok.Device, CreatedAt: tok.CreatedAt,
	})
}

func (s *api) authMethods(w http.ResponseWriter, _ *http.Request) error {
	return server.JSON(w, http.StatusOK, s.core.SignIn.Methods())
}

func (s *api) authStart(w http.ResponseWriter, r *http.Request) error {
	const route = "start"
	if err := s.allowAuthIP(r, route); err != nil {
		return err
	}
	body, err := extractors.Body[startRequest](w, r)
	if err != nil {
		return err
	}
	if err := s.allowAuthEmail(route, body.Email); err != nil {
		return err
	}
	if err := s.core.SignIn.Start(r.Context(), body.Email); err != nil {
		return err
	}
	return server.JSON(w, http.StatusAccepted, struct{}{})
}

func (s *api) authVerify(w http.ResponseWriter, r *http.Request) error {
	const route = "verify"
	if err := s.allowAuthIP(r, route); err != nil {
		return err
	}
	body, err := extractors.Body[verifyRequest](w, r)
	if err != nil {
		return err
	}
	if err := s.allowAuthEmail(route, body.Email); err != nil {
		return err
	}
	plaintext, tok, err := s.core.SignIn.Verify(r.Context(), body.Email, strings.TrimSpace(body.Code), body.Device)
	if err != nil {
		return err
	}
	return signedIn(w, plaintext, tok)
}

func (s *api) authLogin(w http.ResponseWriter, r *http.Request) error {
	const route = "login"
	if err := s.allowAuthIP(r, route); err != nil {
		return err
	}
	body, err := extractors.Body[loginRequest](w, r)
	if err != nil {
		return err
	}
	if err := s.allowAuthEmail(route, body.Email); err != nil {
		return err
	}
	plaintext, tok, err := s.core.SignIn.Login(r.Context(), body.Email, body.Password, body.Device)
	if err != nil {
		return err
	}
	return signedIn(w, plaintext, tok)
}
