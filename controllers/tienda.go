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
//MetodosPago Lista los metodos de pago disponibles (tarjetas,PSE, salgo Jobsy)
// GET /api/tienda/metodos-pago

func (c *CatalogoController) MetodosPago() {
	items, err := svc.Pagos.MetodosDisponibles(c.Contexto())
	if err != nil{
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
	}

//Planes Lista los planes de suscripcion disponibles.
// GET /api/tienda/planes

func (c *CatalogoController) Planes() {
	items, err := svc.Tienda.Planes(c.Contexto())
	if err != nil{
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

//TiendaController atiende el carrito, los pedidos, los pagos y el saldo 
type TiendaController struct {
	SesionController	
}

//CrearPedido hace el checkout: crea el pedido y lo cobra con el metodo
//Indicado. Responde 201 con el pedido. POST /api/tienda/pedidos.

func (c *TiendaController) CrearPedido() {
	id, err := c.UsuarioActual()
	if err != nil{
		c.Fallo(err)
		return
	}
	var p services.PeticionCheckout
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	pedido, err := svc.Tienda.CrearPedido(c.Contexto (), id, p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, pedido)
}
