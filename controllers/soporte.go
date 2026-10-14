package controllers

import (
	"net/http"

	"api_MID/services"
)

// soporte es el servicio que usa este controlador. Beego crea un controlador
// nuevo en cada peticion, asi que la dependencia se fija una vez al arrancar
// con ConfigurarSoporte.
var soporte *services.Soporte

// ConfigurarSoporte deja listo el servicio de soporte. Se llama desde main.
func ConfigurarSoporte(s *services.Soporte) {
	soporte = s
}

// SoporteController atiende los reportes del propio usuario. Embebe
// SesionController, asi que todas sus rutas exigen sesion.
type SoporteController struct {
	SesionController
}

// MisReportes lista los reportes del usuario. GET /api/soporte/reportes.
func (c *SoporteController) MisReportes() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	reportes, err := soporte.MisReportes(id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, reportes)
}

// CrearReporte abre un reporte nuevo. Responde 201 con el reporte.
// POST /api/soporte/reportes.
func (c *SoporteController) CrearReporte() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p services.PeticionReporte
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	reporte, err := soporte.CrearReporte(id, p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, reporte)
}

// Mensajes devuelve el hilo de un reporte del usuario.
// GET /api/soporte/reportes/:id/mensajes.
func (c *SoporteController) Mensajes() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	mensajes, err := soporte.Mensajes(id, c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, mensajes)
}

// ResponderReporte agrega un mensaje del usuario a su reporte. Responde 201.
// POST /api/soporte/reportes/:id/mensajes.
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
	mensaje, err := soporte.Responder(id, c.ParamID(), p.Texto)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, mensaje)
}
