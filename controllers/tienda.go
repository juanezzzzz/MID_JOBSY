package controllers

import (
	"net/http"

	"MID_JOBSY/services"
)

// CatalogoController expone lo que se puede ver sin sesion: productos,
// categorias, metodos de pago y planes.
type CatalogoController struct {
	BaseController
}

// Productos devuelve productos paginados, filtrando por ?categoria.
// GET /api/tienda/productos.
func (c *CatalogoController) Productos() {
	pag, err := svc.Tienda.Productos(c.Contexto(), c.Ctx.Request.URL.Query().Get("categoria"), c.Entero("page"), c.Entero("pageSize"))
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, pag)
}

// DetalleProducto devuelve un producto por su :id.
// GET /api/tienda/productos/:id.
func (c *CatalogoController) DetalleProducto() {
	p, err := svc.Tienda.ProductoPorID(c.Contexto(), c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, p)
}

// Categorias lista las categorias de la tienda. GET /api/catalogo/categorias.
func (c *CatalogoController) Categorias() {
	cats, err := svc.Tienda.Categorias(c.Contexto())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, cats)
}
}
