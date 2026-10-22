package controllers

import (
	"net/http"

	"api_MID/models"
)

// MisOfertas lista las ofertas del empleador con su estado, fotos y cuantas
// postulaciones tienen. GET /api/empleo/ofertas/mias.
func (c *EmpleoController) MisOfertas() {
	id, err := c.UsuarioConRol(models.RolEmpleador)
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.Empleador.MisOfertas(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// PostulacionesRecibidas lista quienes se postularon a una oferta propia.
// GET /api/empleo/ofertas/:id/postulaciones.
func (c *EmpleoController) PostulacionesRecibidas() {
	id, err := c.UsuarioConRol(models.RolEmpleador)
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.Empleador.Recibidas(c.Contexto(), id, c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// EstadoOferta activa, pausa o cierra una oferta propia.
// PUT /api/empleo/ofertas/:id/estado.
func (c *EmpleoController) EstadoOferta() {
	id, err := c.UsuarioConRol(models.RolEmpleador)
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		Estado string `json:"estado"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Empleador.CambiarEstadoOferta(c.Contexto(), id, c.ParamID(), p.Estado)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// SubirFotoOferta recibe una foto de la casa (multipart "foto") y la asocia a
// la oferta propia. Responde 201 con la oferta actualizada.
// POST /api/empleo/ofertas/:id/fotos.
func (c *EmpleoController) SubirFotoOferta() {
	id, err := c.UsuarioConRol(models.RolEmpleador)
	if err != nil {
		c.Fallo(err)
		return
	}
	url, err := c.recibirArchivo("foto", "ofertas")
	if err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Empleador.AgregarFoto(c.Contexto(), id, c.ParamID(), url)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, dato)
}

// EstadoPostulacion pasa a revision o rechaza una postulacion a una oferta
// propia. PUT /api/empleo/postulaciones/:id/estado.
func (c *EmpleoController) EstadoPostulacion() {
	id, err := c.UsuarioConRol(models.RolEmpleador)
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		Estado string `json:"estado"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Empleador.CambiarEstadoPostulacion(c.Contexto(), id, c.ParamID(), p.Estado)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}
