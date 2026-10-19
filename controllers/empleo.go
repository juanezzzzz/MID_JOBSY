
package controllers

import (
	"net/http"
	"strconv"

	"api_MID/models"
	"api_MID/services"
)

// EmpleadosController es el buscador de empleados, que el front no protege
// con sesion.
type EmpleadosController struct {
	BaseController
}

// Listar devuelve empleados paginados filtrando por el texto ?busqueda.
// GET /api/usuarios/empleados.
func (c *EmpleadosController) Listar() {
	pag, err := svc.Empleados.Listar(c.contexto(), c.Ctx.Request.URL.Query().Get("busqueda"), c.Entero("page"), c.Entero("pageSize"))
	if err != nil {
		c.fallo(err)
		return
	}
	c.Responder(http.StatusOK, pag)
}

// Destacados devuelve los primeros empleados para la portada. El ?limit se
// acota a [1,50] y por defecto es 6. GET /api/usuarios/empleados/destacados.
func (c *EmpleadosController) Destacados() {
	limite := c.Entero("limit")
	if limite < 1 || limite > 50 {
		limite = 6
	}
	pag, err := svc.Empleados.Listar(c.Contexto(), "", 1, limite)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, pag.Items)
}

// Detalle devuelve el perfil de un empleado por su :id.
// GET /api/usuarios/empleados/:id.
func (c *EmpleadosController) Detalle() {
	emp, err := svc.Empleados.PorID(c.Contexto(), c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, emp)
}

// Resenas lista las resenas recibidas por el empleado :id.
// GET /api/usuarios/empleados/:id/resenas.
func (c *EmpleadosController) Resenas() {
	items, err := svc.Resenas.De(c.Contexto(), c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// Disponibilidad lista las franjas horarias del empleado :id.
// GET /api/usuarios/empleados/:id/disponibilidad.

func (c *EmpleadosController) Disponibilidad() {
	items, err := svc.PerfilEmpleado.Disponibilidad(c.contexto(), c.ParamID())
	if err !=nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

