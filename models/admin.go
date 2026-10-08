
package models

type EstadisticasAdmin struct {
	VentasMes                float64 `json:"ventas_mes"`
	PedidosMes               int	 `json:"pedidos_mes"`
	ClientesRegistrados      int	 `json:"clientes_registrados"`
	CalificacionPromedio     float64 `json:"calificacion_promedio"`
	IngresosHoy				 float64 `json:"ingresos_hoy"`
	PedidosHoy				 int	 `json:"pedidos_hoy"`
	StockCritico			 int	 `json:"stock_critico"`
	Satisfaccion			 float64 `json:"satisfaccion"`

}

type PuntoVenta struct {
	Mes          string `json:"mes"`
	Ventas       float64 `json:"ventas"`
}