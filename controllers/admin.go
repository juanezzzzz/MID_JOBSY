package controllers

import (
	"net/http"

	"api_MID/services"
)

// AdminTiendaController es el panel de la tienda.
type AdminTiendaController struct {
	AdminController
}

// Estadisticas devuelve las metricas del panel (ventas, ingresos, etc.).
// GET /api/tienda/admin/estadisticas.
func (c *AdminTiendaController) Estadisticas() {
	est, err := svc.Admin.Estadisticas(c.Contexto())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, est)
}

// Ventas devuelve la serie de ventas del ano indicado en ?ano.
// GET /api/tienda/admin/ventas.
func (c *AdminTiendaController) Ventas() {
	items, err := svc.Admin.Ventas(c.Contexto(), c.Entero("ano"))
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// ProductosTop devuelve los productos mas vendidos (acotado por ?limit).
// GET /api/tienda/admin/productos-top.
func (c *AdminTiendaController) ProductosTop() {
	items, err := svc.Admin.ProductosTop(c.Contexto(), c.Entero("limit"))
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// PedidosRecientes devuelve los ultimos pedidos (acotado por ?limit).
// GET /api/tienda/admin/pedidos-recientes.
func (c *AdminTiendaController) PedidosRecientes() {
	items, err := svc.Admin.PedidosRecientes(c.Contexto(), c.Entero("limit"))
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// Productos lista todos los productos para administrarlos (incluye inactivos).
// GET /api/tienda/admin/productos.
func (c *AdminTiendaController) Productos() {
	items, err := svc.Tienda.AdminProductos(c.Contexto())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// CrearProducto da de alta un producto. Responde 201.
// POST /api/tienda/admin/productos.
func (c *AdminTiendaController) CrearProducto() {
	var p services.PeticionProducto
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	prod, err := svc.Tienda.AdminCrearProducto(c.Contexto(), p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, prod)
}

// ActualizarProducto modifica el producto :id. PUT /api/tienda/admin/productos/:id.
func (c *AdminTiendaController) ActualizarProducto() {
	var p services.PeticionProducto
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	prod, err := svc.Tienda.AdminActualizarProducto(c.Contexto(), c.ParamID(), p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, prod)
}

// BorrarProducto elimina (o desactiva) el producto :id. Responde 204.
// DELETE /api/tienda/admin/productos/:id.
func (c *AdminTiendaController) BorrarProducto() {
	if err := svc.Tienda.AdminBorrarProducto(c.Contexto(), c.ParamID()); err != nil {
		c.Fallo(err)
		return
	}
	c.SinContenido()
}

// AjustarStock suma (o resta, si es negativo) el delta al stock del producto
// :id. POST /api/tienda/admin/productos/:id/stock.
func (c *AdminTiendaController) AjustarStock() {
	var p struct {
		Delta int `json:"delta"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	prod, err := svc.Tienda.AdminAjustarStock(c.Contexto(), c.ParamID(), p.Delta)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, prod)
}

// Reembolsar deshace un pedido ya cobrado: devuelve el dinero y repone stock.
// POST /api/tienda/admin/pedidos/:id/reembolsar.
func (c *AdminTiendaController) Reembolsar() {
	var p struct {
		Motivo string `json:"motivo"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	res, err := svc.Tienda.Reembolsar(c.Contexto(), c.ParamID(), p.Motivo)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, res)
}

// CrearCategoria crea una categoria de tienda. Responde 201.
// POST /api/tienda/admin/categorias.
func (c *AdminTiendaController) CrearCategoria() {
	var p struct {
		Nombre string `json:"nombre"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Tienda.AdminCrearCategoria(c.Contexto(), p.Nombre)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, dato)
}

// RenombrarCategoria cambia el nombre de la categoria :id.
// PUT /api/tienda/admin/categorias/:id.
func (c *AdminTiendaController) RenombrarCategoria() {
	var p struct {
		Nombre string `json:"nombre"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Tienda.AdminRenombrarCategoria(c.Contexto(), c.ParamID(), p.Nombre)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// BorrarCategoria elimina la categoria :id. Responde 204.
// DELETE /api/tienda/admin/categorias/:id.
func (c *AdminTiendaController) BorrarCategoria() {
	if err := svc.Tienda.AdminBorrarCategoria(c.Contexto(), c.ParamID()); err != nil {
		c.Fallo(err)
		return
	}
	c.SinContenido()
}

// AdminSoporteController atiende los reportes de todos los usuarios.
type AdminSoporteController struct {
	AdminController
}

// Reportes lista los reportes de todos los usuarios.
// GET /api/soporte/admin/reportes.
func (c *AdminSoporteController) Reportes() {
	items, err := svc.Soporte.Todos(c.Contexto())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// Mensajes devuelve la conversacion del reporte :id (vista de soporte, sin
// exigir que sea propio). GET /api/soporte/admin/reportes/:id/mensajes.
func (c *AdminSoporteController) Mensajes() {
	items, err := svc.Soporte.MensajesAdmin(c.Contexto(), c.ParamID())
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// ResponderComoSoporte agrega un mensaje al reporte :id firmado como el agente
// de soporte del token. Responde 201.
// POST /api/soporte/admin/reportes/:id/mensajes.
func (c *AdminSoporteController) ResponderComoSoporte() {
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
	m, err := svc.Soporte.ResponderComoSoporte(c.Contexto(), id, c.ParamID(), p.Texto)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, m)
}

// CambiarEstado mueve el reporte :id a otro estado (abierto, en_revision,
// cerrado...). PUT /api/soporte/admin/reportes/:id/estado.
func (c *AdminSoporteController) CambiarEstado() {
	var p struct {
		Estado string `json:"estado"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	rep, err := svc.Soporte.CambiarEstado(c.Contexto(), c.ParamID(), p.Estado)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, rep)
}

// Retirar elimina (desactiva) el reporte :id. Responde 204.
// DELETE /api/soporte/admin/reportes/:id.
func (c *AdminSoporteController) Retirar() {
	if err := svc.Soporte.Retirar(c.Contexto(), c.ParamID()); err != nil {
		c.Fallo(err)
		return
	}
	c.SinContenido()
}
