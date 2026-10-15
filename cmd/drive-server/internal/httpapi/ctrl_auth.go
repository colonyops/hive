package httpapi

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/guard"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/internal/web/extractors"
)

const (
	maxEmail     = 254
	authLimitKey = 10_000
	authIdle     = 2 * time.Hour
)

var codeRE = regexp.MustCompile(`^\d{8}$`)

type authHandlers struct {
	svc            Services
	cookies        cookies
	startPerEmail  *guard.Limiter
	startPerIP     *guard.Limiter
	verifyPerEmail *guard.Limiter
}

func registerAuthRoutes(mux *http.ServeMux, chain *errchain.ErrChain, sessionOnly func(errchain.HandlerFunc) http.HandlerFunc, svc Services, c cookies) {
	h := authHandlers{
		svc:            svc,
		cookies:        c,
		startPerEmail:  guard.NewLimiter(core.StartPerEmail, core.StartPerEmailWindow, authLimitKey, authIdle, nil),
		startPerIP:     guard.NewLimiter(core.StartPerIP, core.StartPerIPWindow, authLimitKey, authIdle, nil),
		verifyPerEmail: guard.NewLimiter(core.VerifyPerEmail, core.VerifyPerEmailWindow, authLimitKey, authIdle, nil),
	}
	mux.HandleFunc("POST /api/v1/auth/code", chain.ToHandlerFunc(h.start))
	mux.HandleFunc("POST /api/v1/auth/verify", chain.ToHandlerFunc(h.verify))
	mux.HandleFunc("POST /api/v1/auth/sign-out", sessionOnly(h.signOut))
}

func allow(w http.ResponseWriter, l *guard.Limiter, key string) error {
	ok, wait := l.Allow(key)
	if ok {
		return nil
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
	return core.NewError(core.KindRateLimited, "too many sign-in attempts")
}

type startRequest driveapi.SignInStart

func (b startRequest) Validate() error { return validEmail(b.Email) }

type verifyRequest driveapi.SignInVerify

func (b verifyRequest) Validate() error {
	return criterio.ValidateStruct(
		validEmail(b.Email),
		criterio.Run("code", strings.TrimSpace(b.Code), criterio.StrMatches(codeRE)),
	)
}

func validEmail(email string) error {
	return criterio.Run("email", strings.TrimSpace(email), criterio.StrMax(maxEmail), criterio.StrEmail)
}

func (h authHandlers) start(w http.ResponseWriter, r *http.Request) error {
	ip := guard.ClientIP(r, h.svc.TrustedProxies)
	if err := allow(w, h.startPerIP, ip.String()); err != nil {
		return err
	}
	body, err := extractors.Body[startRequest](w, r)
	if err != nil {
		return err
	}
	if err := allow(w, h.startPerEmail, core.NormalizeEmail(body.Email)); err != nil {
		return err
	}
	if err := h.svc.SignIn.Start(r.Context(), body.Email); err != nil {
		return err
	}
	return server.JSON(w, http.StatusAccepted, struct{}{})
}

func (h authHandlers) verify(w http.ResponseWriter, r *http.Request) error {
	body, err := extractors.Body[verifyRequest](w, r)
	if err != nil {
		return err
	}
	if err := allow(w, h.verifyPerEmail, core.NormalizeEmail(body.Email)); err != nil {
		return err
	}
	value, err := h.svc.SignIn.Verify(r.Context(), body.Email, strings.TrimSpace(body.Code))
	if err != nil {
		return err
	}
	h.cookies.set(w, value)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h authHandlers) signOut(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(h.cookies.name); err == nil {
		if err := h.svc.Sessions.Revoke(r.Context(), c.Value); err != nil {
			return err
		}
	}
	h.cookies.clear(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}
