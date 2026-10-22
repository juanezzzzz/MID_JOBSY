package models

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

	// Desglose y resultado del cobro. Son campos de mas respecto al modelo
	// Order del frontend, que los ignora; existen para que la pantalla de
	// pago pueda mostrar el detalle y seguir una redireccion si la hay.
	Subtotal       float64 `json:"subtotal,omitempty"`
	Descuento      float64 `json:"descuento,omitempty"`
	Envio          float64 `json:"envio,omitempty"`
	PagoID         string  `json:"pagoId,omitempty"`
	EstadoPago     string  `json:"estadoPago,omitempty"`
	URLRedireccion string  `json:"urlRedireccion,omitempty"`

	// Entrega son los datos para llevar el pedido ("envio" ya es su costo).
	Entrega *DatosEntrega `json:"entrega,omitempty"`
}

// DatosEntrega es a quien y a donde se lleva un pedido de la tienda.
type DatosEntrega struct {
	Nombre    string `json:"nombre"`
	Telefono  string `json:"telefono"`
	Direccion string `json:"direccion"`
	// Detalle de la vivienda: apartamento, torre, casa, interior.
	Detalle string `json:"detalle,omitempty"`
	Barrio  string `json:"barrio,omitempty"`
	Ciudad  string `json:"ciudad"`
	// Notas son indicaciones para quien entrega (porteria, horario...).
	Notas string `json:"notas,omitempty"`
}

// Cupon es el descuento resuelto para un subtotal concreto.
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

	// PrecioCambio avisa de que el precio subio o bajo desde que la linea se
	// anadio. El frontend puede senalarlo antes de que el usuario pague algo
	// distinto de lo que creia.
	PrecioCambio bool `json:"precioCambio,omitempty"`
}

type Carrito struct {
	Items    []LineaCarrito `json:"items"`
	Subtotal float64        `json:"subtotal"`
	Envio    float64        `json:"envio"`
	Total    float64        `json:"total"`
	Moneda   string         `json:"moneda"`
}

// MetodoGuardado es una cuenta o tarjeta del usuario. El numero completo no
// sale nunca del servidor.
type MetodoGuardado struct {
	ID          string `json:"id"`
	MetodoID    string `json:"metodoId"`
	Alias       string `json:"alias"`
	Enmascarado string `json:"enmascarado"`
	EsPrincipal bool   `json:"esPrincipal"`
}
