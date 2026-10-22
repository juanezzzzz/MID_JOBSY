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

// Listar devuelve empleados paginados, filtrando por el texto ?busqueda.
// GET /api/usuarios/empleados.
func (c *EmpleadosController) Listar() {
	pag, err := svc.Empleados.Listar(c.Contexto(), c.Ctx.Request.URL.Query().Get("busqueda"), c.Entero("page"), c.Entero("pageSize"))
	if err != nil {
		c.Fallo(err)
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
	items, err := svc.PerfilEmpleado.Disponibilidad(c.Contexto(), c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// OfertasController es el buscador publico de ofertas.
type OfertasController struct {
	BaseController
}

// Listar devuelve ofertas paginadas aplicando los filtros del query string
// (ciudad, busqueda, precio, tamano, modalidad). Angular repite la clave para
// los arrays, por eso se recorren q["tamano"] y q["modalidad"].
// GET /api/empleo/ofertas.
func (c *OfertasController) Listar() {
	q := c.Ctx.Request.URL.Query()
	precioMin, _ := strconv.ParseFloat(q.Get("precioMin"), 64)
	precioMax, _ := strconv.ParseFloat(q.Get("precioMax"), 64)

	f := models.FiltrosOferta{
		Ciudad:    q.Get("ciudad"),
		Busqueda:  q.Get("busqueda"),
		PrecioMin: precioMin,
		PrecioMax: precioMax,
		Pagina:    c.Entero("page"),
		TamanoPag: c.Entero("pageSize"),
	}
	// HttpParams de Angular repite la clave para los arrays.
	for _, v := range q["tamano"] {
		f.Tamano = append(f.Tamano, models.Tamano(v))
	}
	for _, v := range q["modalidad"] {
		f.Modalidad = append(f.Modalidad, models.Modalidad(v))
	}

	pag, err := svc.Ofertas.Listar(c.Contexto(), f)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, pag)
}

// Destacadas devuelve las ofertas para la portada (acotadas por ?limit).
// GET /api/empleo/ofertas/destacadas.
func (c *OfertasController) Destacadas() {
	items, err := svc.Ofertas.Destacadas(c.Contexto(), c.Entero("limit"))
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// Detalle devuelve una oferta por su :id. GET /api/empleo/ofertas/:id.
func (c *OfertasController) Detalle() {
	of, err := svc.Ofertas.PorID(c.Contexto(), c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, of)
}

// EmpleoController publica ofertas, gestiona postulaciones y contrataciones.
type EmpleoController struct {
	SesionController
}

// CrearOferta publica una oferta a nombre del empleador del token. Responde
// 201. POST /api/empleo/ofertas.
func (c *EmpleoController) CrearOferta() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var of models.Oferta
	if err := c.LeerJSON(&of); err != nil {
		c.Fallo(err)
		return
	}
	creada, err := svc.Ofertas.Crear(c.Contexto(), id, of)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, creada)
}

// MisPostulaciones lista las postulaciones hechas por el usuario.
// GET /api/empleo/postulaciones/mias.
func (c *EmpleoController) MisPostulaciones() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.Postulaciones.Mias(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// Postular crea una postulacion del empleado a una oferta. Responde 201.
// POST /api/empleo/postulaciones.
func (c *EmpleoController) Postular() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p services.PeticionPostular
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	creada, err := svc.Postulaciones.Crear(c.Contexto(), id, p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, creada)
}

// RetirarPostulacion cancela la postulacion :id del usuario. Responde 204.
// DELETE /api/empleo/postulaciones/:id.
func (c *EmpleoController) RetirarPostulacion() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	if err := svc.Postulaciones.Retirar(c.Contexto(), id, c.ParamID()); err != nil {
		c.Fallo(err)
		return
	}
	c.SinContenido()
}

// MisContrataciones lista las contrataciones del usuario, tanto como empleado
// como empleador. GET /api/empleo/contrataciones/mias.
func (c *EmpleoController) MisContrataciones() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.Contrataciones.Mias(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// Contratar formaliza una contratacion a partir de una postulacion aceptada.
// Responde 201. POST /api/empleo/contrataciones.
func (c *EmpleoController) Contratar() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p services.PeticionContratar
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Contrataciones.Crear(c.Contexto(), id, p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, dato)
}

// ContratarDirecto contrata a alguien del buscador sin oferta previa: el
// servicio crea por dentro la oferta y la postulacion necesarias. Responde
// 201. POST /api/empleo/contrataciones/directa.
func (c *EmpleoController) ContratarDirecto() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p services.PeticionContratarDirecto
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Contrataciones.CrearDirecta(c.Contexto(), id, p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, dato)
}

// DetalleContratacion devuelve la contratacion :id si pertenece al usuario.
// GET /api/empleo/contrataciones/:id.
func (c *EmpleoController) DetalleContratacion() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Contrataciones.PorID(c.Contexto(), id, c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// FinalizarContratacion cierra la contratacion :id (servicio completado).
// POST /api/empleo/contrataciones/:id/finalizar.
func (c *EmpleoController) FinalizarContratacion() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Contrataciones.Finalizar(c.Contexto(), id, c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}
