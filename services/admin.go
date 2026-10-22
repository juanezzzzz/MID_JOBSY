package services

import (
	"context"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// Admin arma los paneles de administracion. No tiene datos propios: agrega
// informacion de la tienda y del core (ventas, pedidos, usuarios, productos).
type Admin struct {
	tienda    *ClienteUpstream
	core      *ClienteUpstream
	catalogo  *Catalogo
	tiendaSvc *Tienda
}

// NuevoAdmin construye el servicio de administracion con los upstreams de
// tienda y core, el catalogo y el servicio de tienda para reusar su logica.
func NuevoAdmin(t, core *ClienteUpstream, cat *Catalogo, tiendaSvc *Tienda) *Admin {
	return &Admin{tienda: t, core: core, catalogo: cat, tiendaSvc: tiendaSvc}
}

// cobrados filtra los pedidos que cuentan como venta: los pagados y los que
// ya salieron. Un PSE pendiente de confirmar no es dinero recibido.
func (a *Admin) cobrados(ctx context.Context, pedidos []filaPedido) []filaPedido {
	out := make([]filaPedido, 0, len(pedidos))
	for _, p := range pedidos {
		n, err := a.catalogo.EstadoDesdeID(ctx, p.IdEstado)
		if err != nil {
			continue
		}
		switch n {
		case "pagado", "enviado", "entregado":
			out = append(out, p)
		}
	}
	return out
}

// Estadisticas calcula las metricas del panel (ventas del mes, ingresos de
// hoy, clientes, stock critico, satisfaccion) pidiendo en paralelo pedidos,
// usuarios y productos y agregandolos. Solo cuenta pedidos cobrados.
func (a *Admin) Estadisticas(ctx context.Context) (*models.EstadisticasAdmin, error) {
	var (
		pedidos  []filaPedido
		usuarios []filaUsuario
		prods    []filaProducto
	)

	err := EnParalelo(
		func() error {
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
			o.Query.Set("limit", "1000")
			return a.tienda.Get(ctx, "facturacion", o, &pedidos)
		},
		func() error {
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
			o.Query.Set("limit", "1000")
			return a.core.Get(ctx, "usuarios", o, &usuarios)
		},
		func() error {
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
			o.Query.Set("limit", "1000")
			return a.tienda.Get(ctx, "productos", o, &prods)
		},
	)
	if err != nil {
		return nil, err
	}

	// Jobsy_API lee y escribe las fechas en UTC, asi que CreadoEn llega en UTC
	// y "hoy" y "este mes" se calculan en la misma zona.
	hoy := time.Now().UTC().Format("2006-01-02")
	mes := time.Now().UTC().Format("2006-01")
	pedidos = a.cobrados(ctx, pedidos)

	est := &models.EstadisticasAdmin{ClientesRegistrados: len(usuarios)}
	sumaRating, conRating := 0.0, 0

	for _, p := range pedidos {
		if strings.HasPrefix(p.CreadoEn, mes) {
			est.VentasMes += p.Total
			est.PedidosMes++
		}
		if strings.HasPrefix(p.CreadoEn, hoy) {
			est.IngresosHoy += p.Total
			est.PedidosHoy++
		}
	}
	for _, pr := range prods {
		if pr.Stock < 10 {
			est.StockCritico++
		}
		if pr.RatingPromedio > 0 {
			sumaRating += pr.RatingPromedio
			conRating++
		}
	}
	if conRating > 0 {
		est.CalificacionPromedio = redondear(sumaRating/float64(conRating), 2)
		// La satisfaccion se expresa en porcentaje sobre cinco estrellas.
		est.Satisfaccion = redondear(est.CalificacionPromedio/5*100, 1)
	}
	return est, nil
}

// redondear ajusta v al numero de decimales indicado, para no arrastrar ruido
// de coma flotante en los importes y porcentajes del panel.
func redondear(v float64, decimales int) float64 {
	p := 1.0
	for i := 0; i < decimales; i++ {
		p *= 10
	}
	return float64(int(v*p+0.5)) / p
}

// Ventas agrega los pedidos por mes del ano pedido.
func (a *Admin) Ventas(ctx context.Context, ano int) ([]models.PuntoVenta, error) {
	if ano <= 0 {
		ano = time.Now().UTC().Year()
	}

	var pedidos []filaPedido
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
	o.Query.Set("limit", "5000")
	if err := a.tienda.Get(ctx, "facturacion", o, &pedidos); err != nil {
		return nil, err
	}

	pedidos = a.cobrados(ctx, pedidos)

	meses := []string{"Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Sep", "Oct", "Nov", "Dic"}
	porMes := make([]float64, 12)
	prefijo := strconv.Itoa(ano) + "-"

	for _, p := range pedidos {
		if !strings.HasPrefix(p.CreadoEn, prefijo) || len(p.CreadoEn) < 7 {
			continue
		}
		m, err := strconv.Atoi(p.CreadoEn[5:7])
		if err != nil || m < 1 || m > 12 {
			continue
		}
		porMes[m-1] += p.Total
	}

	out := make([]models.PuntoVenta, 0, 12)
	for i, m := range meses {
		out = append(out, models.PuntoVenta{Mes: m, Ventas: porMes[i]})
	}
	return out, nil
}

// ProductosTop ordena por unidades vendidas.
func (a *Admin) ProductosTop(ctx context.Context, limite int) ([]models.Producto, error) {
	if limite < 1 || limite > 50 {
		limite = 5
	}

	var (
		items   []filaPedidoItem
		pedidos []filaPedido
	)
	err := EnParalelo(
		func() error {
			oi := &OpcionesUpstream{Query: map[string][]string{}}
			oi.Query.Set("limit", "5000")
			return a.tienda.Get(ctx, "pedido_items", oi, &items)
		},
		func() error {
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("limit", "5000")
			return a.tienda.Get(ctx, "facturacion", o, &pedidos)
		},
	)
	if err != nil {
		return nil, err
	}

	// Solo cuentan las unidades de pedidos cobrados: las de un pedido anulado
	// no se vendieron.
	valido := map[int]bool{}
	for _, p := range a.cobrados(ctx, pedidos) {
		valido[p.Id] = true
	}
	vendidas := map[int]int{}
	for _, it := range items {
		if it.IdProductos != nil && it.IdPedidos != nil && valido[it.IdPedidos.Id] {
			vendidas[it.IdProductos.Id] += it.Cantidad
		}
	}

	var prods []filaProducto
	op := &OpcionesUpstream{Query: map[string][]string{}}
	op.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
	op.Query.Set("limit", "1000")
	if err := a.tienda.Get(ctx, "productos", op, &prods); err != nil {
		return nil, err
	}

	// Orden descendente por unidades; sin ventas, por id.
	for i := 0; i < len(prods); i++ {
		for j := i + 1; j < len(prods); j++ {
			if vendidas[prods[j].Id] > vendidas[prods[i].Id] {
				prods[i], prods[j] = prods[j], prods[i]
			}
		}
	}
	if len(prods) > limite {
		prods = prods[:limite]
	}

	out := make([]models.Producto, 0, len(prods))
	for _, p := range prods {
		out = append(out, productoADominio(p))
	}
	return out, nil
}

// PedidosRecientes devuelve los ultimos pedidos.
func (a *Admin) PedidosRecientes(ctx context.Context, limite int) ([]models.Pedido, error) {
	if limite < 1 || limite > 50 {
		limite = 5
	}

	var pedidos []filaPedido
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
	o.Query.Set("limit", strconv.Itoa(limite))
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := a.tienda.Get(ctx, "facturacion", o, &pedidos); err != nil {
		return nil, err
	}

	return a.tiendaSvc.PedidosADominio(ctx, pedidos)
}
