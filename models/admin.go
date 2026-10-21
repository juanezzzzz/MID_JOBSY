package models

type EstadisticasAdmin struct {
	VentasMes            float64 `json:"ventasMes"`
	PedidosMes           int     `json:"pedidosMes"`
	ClientesRegistrados  int     `json:"clientesRegistrados"`
	CalificacionPromedio float64 `json:"calificacionPromedio"`
	IngresosHoy          float64 `json:"ingresosHoy"`
	PedidosHoy           int     `json:"pedidosHoy"`
	StockCritico         int     `json:"stockCritico"`
	Satisfaccion         float64 `json:"satisfaccion"`
}

type PuntoVenta struct {
	Mes    string  `json:"mes"`
	Ventas float64 `json:"ventas"`
}
