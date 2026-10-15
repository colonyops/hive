package httpapi

import (
	"net/http"
	"strings"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

const (
	hostCookieName  = "__Host-hive_drive_session"
	plainCookieName = "hive_drive_session"
)

// The __Host- prefix needs Secure, so a plain-http loopback base_url (local
// development) drops both.
type cookies struct {
	name   string
	secure bool
}

func newCookies(baseURL string) cookies {
	if strings.HasPrefix(baseURL, "http://") {
		return cookies{name: plainCookieName}
	}
	return cookies{name: hostCookieName, secure: true}
}

func (c cookies) set(w http.ResponseWriter, value string) {
	http.SetCookie(w, c.cookie(value, int(core.SessionTTL.Seconds())))
}

func (c cookies) clear(w http.ResponseWriter) {
	http.SetCookie(w, c.cookie("", -1))
}

func (c cookies) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     c.name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	}
}
