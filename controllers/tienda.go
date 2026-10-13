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

// MetodosPago lista los metodos de pago disponibles (tarjeta, PSE, saldo...).
// GET /api/tienda/metodos-pago.
func (c *CatalogoController) MetodosPago() {
	items, err := svc.Pagos.MetodosDisponibles(c.Contexto())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// Planes lista los planes de suscripcion disponibles.
// GET /api/suscripciones/planes.
func (c *CatalogoController) Planes() {
	items, err := svc.Suscripciones.Planes(c.Contexto())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// TiendaController atiende el carrito, los pedidos, los pagos y el saldo del usuario.
type TiendaController struct {
	SesionController
}

// CrearPedido hace el checkout: crea el pedido y lo cobra con el metodo
// indicado. Responde 201 con el pedido. POST /api/tienda/pedidos.
func (c *TiendaController) CrearPedido() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p services.PeticionCheckout
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	pedido, err := svc.Tienda.Checkout(c.Contexto(), id, p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, pedido)
}

// MisPedidos lista los pedidos del usuario. GET /api/tienda/pedidos/mios.
func (c *TiendaController) MisPedidos() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.Tienda.MisPedidos(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// ValidarCupon comprueba un cupon contra un subtotal y devuelve el descuento
// aplicable. POST /api/tienda/cupones/validar.
func (c *TiendaController) ValidarCupon() {
	var p struct {
		Codigo   string  `json:"codigo"`
		Subtotal float64 `json:"subtotal"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Tienda.ValidarCupon(c.Contexto(), p.Codigo, p.Subtotal)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// MiCarrito devuelve el carrito abierto del usuario. GET /api/tienda/carrito.
func (c *TiendaController) MiCarrito() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Carrito.Mio(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// AgregarAlCarrito suma la cantidad indicada de un producto al carrito.
// POST /api/tienda/carrito.
func (c *TiendaController) AgregarAlCarrito() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		ProductoID string `json:"productoId"`
		Cantidad   int    `json:"cantidad"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Carrito.Agregar(c.Contexto(), id, p.ProductoID, p.Cantidad)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// QuitarDelCarrito elimina del carrito el producto :id por completo.
// DELETE /api/tienda/carrito/:id.
func (c *TiendaController) QuitarDelCarrito() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Carrito.Quitar(c.Contexto(), id, c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// FijarEnCarrito deja el producto :id con una cantidad exacta; cero lo quita.
// PUT /api/tienda/carrito/:id.
func (c *TiendaController) FijarEnCarrito() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		Cantidad int `json:"cantidad"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Carrito.Fijar(c.Contexto(), id, c.ParamID(), p.Cantidad)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// VaciarCarrito quita todas las lineas del carrito del usuario.
// DELETE /api/tienda/carrito.
func (c *TiendaController) VaciarCarrito() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Carrito.Vaciar(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// MisMetodosGuardados lista los metodos de pago que el usuario tiene guardados.
// GET /api/tienda/metodos-guardados.
func (c *TiendaController) MisMetodosGuardados() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.MetodosGuardados.Mios(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// GuardarMetodoPago guarda un metodo de pago del usuario (el numero se
// almacena enmascarado). Responde 201. POST /api/tienda/metodos-guardados.
func (c *TiendaController) GuardarMetodoPago() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		MetodoID     string `json:"metodoId"`
		Alias        string `json:"alias"`
		NumeroCuenta string `json:"numeroCuenta"`
		EsPrincipal  bool   `json:"esPrincipal"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	m, err := svc.MetodosGuardados.Guardar(c.Contexto(), id, p.MetodoID, p.Alias, p.NumeroCuenta, p.EsPrincipal)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, m)
}

// BorrarMetodoPago elimina el metodo guardado :id del usuario. Responde 204.
// DELETE /api/tienda/metodos-guardados/:id.
func (c *TiendaController) BorrarMetodoPago() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	if err := svc.MetodosGuardados.Borrar(c.Contexto(), id, c.ParamID()); err != nil {
		c.Fallo(err)
		return
	}
	c.SinContenido()
}

// MiSaldo devuelve el saldo interno (monedero) del usuario.
// GET /api/tienda/saldo.
func (c *TiendaController) MiSaldo() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	s, err := svc.Monedero.De(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, s)
}

// MisPagos lista el historial de pagos del usuario. GET /api/tienda/pagos/mios.
func (c *TiendaController) MisPagos() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.Pagos.Mios(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}