package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

type statusResponse struct {
	Webhook webhookStatus `json:"webhook"`
	API     apiStatus     `json:"api"`
}

type webhookStatus struct {
	Running    bool   `json:"running"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	PathPrefix string `json:"pathPrefix"`
}

type apiStatus struct {
	PathPrefix string `json:"pathPrefix"`
}

func (ctrl *Controller) Status(w http.ResponseWriter, r *http.Request) error {
	running, port := ctrl.core.Webhooks.Endpoint(r.Context())
	return server.JSON(w, http.StatusOK, statusResponse{
		Webhook: webhookStatus{
			Running:    running,
			Host:       ctrl.core.Webhooks.Host(),
			Port:       port,
			PathPrefix: webhook.PathPrefix,
		},
		API: apiStatus{PathPrefix: PathPrefix},
	})
}
