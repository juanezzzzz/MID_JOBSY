package controllers

import (
	"net/http"
)

// SuscripcionesController consulta y contrata planes.
type SuscripcionesController struct {
	SesionController
}

// MiSuscripcion devuelve la suscripcion activa del usuario del token, o null
// si no tiene ninguna. GET /api/suscripciones/mi-suscripcion.
func (c *SuscripcionesController) MiSuscripcion() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	s, err := svc.Suscripciones.Mia(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	// El frontend tipa Subscription | null: sin suscripcion se devuelve null,
	// no un 404.
	c.Responder(http.StatusOK, s)
}

// Suscribir da de alta al usuario en el plan indicado en el cuerpo (planId) y
// responde 201 con la suscripcion creada. POST /api/suscripciones/suscribir.
func (c *SuscripcionesController) Suscribir() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		PlanID string `json:"planId"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	s, err := svc.Suscripciones.Suscribir(c.Contexto(), id, p.PlanID)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, s)
}
