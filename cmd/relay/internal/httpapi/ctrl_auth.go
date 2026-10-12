package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/server"
)

type methodsResponse struct {
	EmailCode bool `json:"email_code"`
	Password  bool `json:"password"`
	APIKey    bool `json:"api_key"`
}

// signInMethods is fixed until Phase 4 adds email codes and passwords.
func signInMethods() methodsResponse {
	return methodsResponse{APIKey: true}
}

func (s *api) authMethods(w http.ResponseWriter, _ *http.Request) error {
	return server.JSON(w, http.StatusOK, signInMethods())
}
