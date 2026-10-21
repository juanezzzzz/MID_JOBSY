package controllers

import (
	"net/http"
)

// SaludController responde al healthcheck.
type SaludController struct {
	BaseController
}

// Salud responde 200 con el estado y el entorno. Lo usa el healthcheck del
// despliegue para saber si el mid esta vivo.
func (c *SaludController) Salud() {
	c.Responder(http.StatusOK, map[string]any{"estado": "ok", "entorno": svc.Cfg.Entorno})
}
