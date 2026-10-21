package controllers

import (
	"net/http"
)

// SoporteController atiende los reportes del propio usuario.
type SoporteController struct {
	SesionController
}

// MisReportes lista los reportes abiertos por el propio usuario.
// GET /api/soporte/reportes.
func (c *SoporteController) MisReportes() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.Soporte.Mios(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// CrearReporte abre un reporte nuevo (tipo, asunto, descripcion) a nombre del
// usuario del token y responde 201. POST /api/soporte/reportes.
func (c *SoporteController) CrearReporte() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		Tipo        string `json:"tipo"`
		Asunto      string `json:"asunto"`
		Descripcion string `json:"descripcion"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	rep, err := svc.Soporte.Crear(c.Contexto(), id, p.Tipo, p.Asunto, p.Descripcion)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, rep)
}

// MensajesReporte devuelve la conversacion de un reporte del usuario. El
// servicio comprueba que el reporte sea suyo. GET /api/soporte/reportes/:id/mensajes.
func (c *SoporteController) MensajesReporte() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.Soporte.Mensajes(c.Contexto(), id, c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// ResponderReporte agrega un mensaje del usuario a su propio reporte y
// responde 201. POST /api/soporte/reportes/:id/mensajes.
func (c *SoporteController) ResponderReporte() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		Texto string `json:"texto"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	m, err := svc.Soporte.Responder(c.Contexto(), id, c.ParamID(), p.Texto)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, m)
}
