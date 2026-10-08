package models


type Cupon struct {
	ID         string  `json:"id"`
	Codigo     string  `json:"codigo"`
	Descuento  float64 `json:"descuento"`
	Porcentaje int     `json:"porcentaje,omitempty"`
}

type LineaCarrito struct {
	Producto Producto `json:"product"`
	Cantidad int      `json:"cantidad"`
	Subtotal float64  `json:"subtotal"`
	PrecioCambio bool `json:"precioCambio,omitempty"`
}

type Carrito struct {
	Items    []LineaCarrito `json:"items"`
	Subtotal float64        `json:"subtotal"`
	Envio    float64        `json:"envio"`
	Total    float64        `json:"total"`
	Moneda   string         `json:"moneda"`
}

type MetodoGuardado struct {
	ID string `json:"id"`
	MetodoID string `json:"metodoId"`
	Alias string `json:"alias"`
	Enmascarado string `json:"enmascarado"`
	Esprincipal bool `json:"esprincipal"`
}

type Producto struct {
	ID           string  `json:"id"`
	SKU          string  `json:"sku"`
	Nombre       string  `json:"nombre"`
	Categoria    string  `json:"categoria"`
	Descripcion  string  `json:"descripcion"`
	Precio       float64 `json:"precio"`
	Moneda       string  `json:"moneda"`
	Stock        int     `json:"stock"`
	ImagenURL    string  `json:"imagenUrl,omitempty"`
	Calificacion float64 `json:"calificacion"`
	Estado       string  `json:"estado"`
}

type Categoria struct {
	ID                string `json:"id"`
	Nombre            string `json:"nombre"`
	Slug              string `json:"slug"`
	CantidadProductos int    `json:"cantidadProductos"`
}

type ItemPedido struct {
	ProductoID string  `json:"productId"`
	Nombre     string  `json:"nombre"`
	Cantidad   int     `json:"cantidad"`
	Precio     float64 `json:"precio"`
}

type Pedido struct {
	ID         string       `json:"id"`
	Numero     string       `json:"numero"`
	Items      []ItemPedido `json:"items"`
	Total      float64      `json:"total"`
	Estado     string       `json:"estado"`
	MetodoPago string       `json:"metodoPago"`
	Fecha      string       `json:"fecha"`


	Subtotal       float64 `json:"subtotal,omitempty"`
	Descuento      float64 `json:"descuento,omitempty"`
	Envio          float64 `json:"envio,omitempty"`
	PagoID         string  `json:"pagoId,omitempty"`
	EstadoPago     string  `json:"estadoPago,omitempty"`
	URLRedireccion string  `json:"urlRedireccion,omitempty"`
}
