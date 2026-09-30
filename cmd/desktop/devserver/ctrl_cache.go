package main

import (
	"net/http"

	"github.com/hay-kot/httpkit/server"
)

type purgeResponse struct {
	Purged bool `json:"purged"`
}

func (c *Control) PurgeCache(w http.ResponseWriter, _ *http.Request) error {
	if err := c.cache.Purge(); err != nil {
		return err
	}
	c.logger.Info().Msg("cache purged")
	return server.JSON(w, http.StatusOK, purgeResponse{Purged: true})
}
