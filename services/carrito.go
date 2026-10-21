package services

import (
	"context"
	"strconv"

	"api_MID/models"
)

// Carrito persiste el carrito del usuario en la base.
//
// El frontend lo guarda en localStorage con el producto entero embebido: se
// pierde al cambiar de dispositivo, y sus precios envejecen. Las tablas
// tienda.carritos y tienda.tr_carrito_items existen para esto y estaban sin
// usar.
//
// Los precios se recalculan siempre desde el catalogo, no desde lo guardado.

type Carrito struct {
	tienda   *ClienteUpstream
	catalogo *Catalogo
}

// NuevoCarrito crea el servicio de carrito sobre el upstream de tienda y el
// catalogo (para resolver precios y estados).
func NuevoCarrito(t *ClienteUpstream, cat *Catalogo) *Carrito {
	return &Carrito{tienda: t, catalogo: cat}
}

type filaCarrito struct {
	Id         int         `json:"Id"`
	IdUsuarios int         `json:"IdUsuarios"`
	IdEstado   int         `json:"IdEstado"`
	IdCupones  *refAnidada `json:"IdCupones"`
	Activo     bool        `json:"Activo"`
}

type filaCarritoItem struct {
	Id             int         `json:"Id"`
	IdCarritos     *refAnidada `json:"IdCarritos"`
	IdProductos    *refAnidada `json:"IdProductos"`
	Cantidad       int         `json:"Cantidad"`
	PrecioUnitario float64     `json:"PrecioUnitario"`
	Seleccionado   bool        `json:"Seleccionado"`
	Activo         bool        `json:"Activo"`
}

// abiertoDe devuelve el carrito activo del usuario, creandolo si hace falta.
func (s *Carrito) abiertoDe(ctx context.Context, idUsuario int) (*filaCarrito, error) {
	idActivo, err := s.catalogo.IDEstado(ctx, "carrito", "activo")
	if err != nil {
		return nil, err
	}

	var filas []filaCarrito
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(idUsuario),
		"IdEstado":   strconv.Itoa(idActivo),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "10")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.tienda.Get(ctx, "carritos", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) > 0 {
		return &filas[0], nil
	}

	var creado filaCarrito
	if err := s.tienda.Post(ctx, "carritos", map[string]any{
		"IdUsuarios": idUsuario,
		"IdEstado":   idActivo,
		"Activo":     true,
	}, &creado); err != nil {
		return nil, err
	}
	creado.IdUsuarios = idUsuario
	return &creado, nil
}

// Mio devuelve el carrito con sus lineas y totales recalculados.
func (s *Carrito) Mio(ctx context.Context, idUsuario string) (*models.Carrito, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	c, err := s.abiertoDe(ctx, id)
	if err != nil {
		return nil, err
	}

	items, err := s.lineasActivas(ctx, c.Id)
	if err != nil {
		return nil, err
	}

	out := &models.Carrito{Items: []models.LineaCarrito{}, Moneda: "COP"}
	if len(items) == 0 {
		return out, nil
	}

	// Una sola lectura del catalogo y cruce en memoria: pedir producto por
	// producto seria N+1.
	var productos []filaProducto
	op := &OpcionesUpstream{Query: map[string][]string{}}
	op.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
	op.Query.Set("limit", "1000")
	if err := s.tienda.Get(ctx, "productos", op, &productos); err != nil {
		return nil, err
	}
	porID := make(map[int]filaProducto, len(productos))
	for _, p := range productos {
		porID[p.Id] = p
	}

	for _, it := range items {
		if it.IdProductos == nil {
			continue
		}
		p, ok := porID[it.IdProductos.Id]
		if !ok {
			// El producto se retiro del catalogo: la linea ya no es valida.
			continue
		}
		linea := models.LineaCarrito{
			Producto: productoADominio(p),
			Cantidad: it.Cantidad,
		}
		// El precio sale del catalogo, no de lo guardado en la linea: entre
		// que se anadio al carrito y ahora el precio pudo cambiar.
		linea.Subtotal = p.Precio * float64(it.Cantidad)
		linea.PrecioCambio = it.PrecioUnitario > 0 && it.PrecioUnitario != p.Precio
		out.Items = append(out.Items, linea)
		out.Subtotal += linea.Subtotal
	}
	out.Envio = envioFijo
	out.Total = out.Subtotal + out.Envio
	return out, nil
}

// lineasActivas devuelve las lineas vivas de un carrito.
//
// Un fallo aqui tiene que subir: si se tomara por "sin lineas", Agregar
// crearia una linea duplicada en vez de acumular sobre la existente.
func (s *Carrito) lineasActivas(ctx context.Context, idCarrito int) ([]filaCarritoItem, error) {
	var items []filaCarritoItem
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdCarritos": strconv.Itoa(idCarrito),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "100")
	if err := s.tienda.Get(ctx, "tr_carrito_items", o, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// escribirLinea guarda una linea con la cantidad dada.
//
// Cantidad cero desactiva la linea en vez de borrarla: el DELETE del CRUD es
// fisico y asi se conserva el rastro. La cantidad guardada se deja como
// estaba, porque la tabla exige que sea mayor que cero
// (ck_carrito_items_cantidad); lo que marca que la linea ya no cuenta es
// Activo.
func (s *Carrito) escribirLinea(ctx context.Context, idCarrito int, it filaCarritoItem, prodID, cantidad int, precio float64) error {
	activa := cantidad > 0
	if !activa {
		cantidad = it.Cantidad
		if cantidad < 1 {
			cantidad = 1
		}
	}
	return s.tienda.Fijar(ctx, "tr_carrito_items/"+strconv.Itoa(it.Id), map[string]any{
		"IdCarritos":     map[string]any{"Id": idCarrito},
		"IdProductos":    map[string]any{"Id": prodID},
		"Cantidad":       cantidad,
		"PrecioUnitario": precio,
		"Seleccionado":   activa,
		"Activo":         activa,
	})
}

// productoVendible lee un producto activo del catalogo.
func (s *Carrito) productoVendible(ctx context.Context, idProducto string) (*filaProducto, error) {
	prodID, err := IDNumerico(idProducto)
	if err != nil {
		return nil, models.Validacion("Identificador de producto invalido.")
	}
	var prods []filaProducto
	op := &OpcionesUpstream{Query: map[string][]string{}}
	op.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(prodID)}))
	op.Query.Set("limit", "1")
	if err := s.tienda.Get(ctx, "productos", op, &prods); err != nil {
		return nil, err
	}
	if len(prods) == 0 || !prods[0].Activo {
		return nil, models.NoEncontrado("ese producto")
	}
	return &prods[0], nil
}

// Agregar suma unidades de un producto al carrito.
func (s *Carrito) Agregar(ctx context.Context, idUsuario, idProducto string, cantidad int) (*models.Carrito, error) {
	if cantidad <= 0 {
		return nil, models.Validacion("La cantidad debe ser mayor que cero.")
	}
	return s.cambiar(ctx, idUsuario, idProducto, func(actual int) int { return actual + cantidad })
}

// Fijar deja un producto con una cantidad exacta. Cero lo quita.
//
// Existe aparte de Agregar porque los botones de mas y menos del carrito
// mandan la cantidad final, y restar con Agregar exigiria cantidades
// negativas.
func (s *Carrito) Fijar(ctx context.Context, idUsuario, idProducto string, cantidad int) (*models.Carrito, error) {
	if cantidad < 0 {
		return nil, models.Validacion("La cantidad no puede ser negativa.")
	}
	return s.cambiar(ctx, idUsuario, idProducto, func(int) int { return cantidad })
}

// Quitar elimina un producto del carrito.
func (s *Carrito) Quitar(ctx context.Context, idUsuario, idProducto string) (*models.Carrito, error) {
	return s.cambiar(ctx, idUsuario, idProducto, func(int) int { return 0 })
}

// cambiar aplica a la linea de un producto la cantidad que calcule nueva a
// partir de la actual (cero si el producto no estaba).
func (s *Carrito) cambiar(ctx context.Context, idUsuario, idProducto string, nueva func(actual int) int) (*models.Carrito, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	prod, err := s.productoVendible(ctx, idProducto)
	if err != nil {
		return nil, err
	}
	c, err := s.abiertoDe(ctx, id)
	if err != nil {
		return nil, err
	}
	lineas, err := s.lineasActivas(ctx, c.Id)
	if err != nil {
		return nil, err
	}

	var existente *filaCarritoItem
	for i := range lineas {
		if lineas[i].IdProductos != nil && lineas[i].IdProductos.Id == prod.Id {
			existente = &lineas[i]
			break
		}
	}
	actual := 0
	if existente != nil {
		actual = existente.Cantidad
	}
	cantidad := nueva(actual)

	if cantidad > prod.Stock {
		return nil, models.Conflicto("No hay stock suficiente: quedan " + strconv.Itoa(prod.Stock) + ".")
	}

	switch {
	case existente != nil:
		if err := s.escribirLinea(ctx, c.Id, *existente, prod.Id, cantidad, prod.Precio); err != nil {
			return nil, err
		}
	case cantidad > 0:
		var creada map[string]any
		if err := s.tienda.Post(ctx, "tr_carrito_items", map[string]any{
			"IdCarritos":     map[string]any{"Id": c.Id},
			"IdProductos":    map[string]any{"Id": prod.Id},
			"Cantidad":       cantidad,
			"PrecioUnitario": prod.Precio,
			"Seleccionado":   true,
			"Activo":         true,
		}, &creada); err != nil {
			return nil, err
		}
	}
	return s.Mio(ctx, idUsuario)
}

// Vaciar desactiva todas las lineas del carrito. El frontend lo llama tras un
// pago aceptado; sin esto el carrito guardado reapareceria con lo ya
// comprado.
func (s *Carrito) Vaciar(ctx context.Context, idUsuario string) (*models.Carrito, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	c, err := s.abiertoDe(ctx, id)
	if err != nil {
		return nil, err
	}
	lineas, err := s.lineasActivas(ctx, c.Id)
	if err != nil {
		return nil, err
	}
	for _, it := range lineas {
		prodID := 0
		if it.IdProductos != nil {
			prodID = it.IdProductos.Id
		}
		if err := s.escribirLinea(ctx, c.Id, it, prodID, 0, it.PrecioUnitario); err != nil {
			return nil, err
		}
	}
	return s.Mio(ctx, idUsuario)
}

// ---- Metodos de pago guardados ----

// MetodosGuardados son las cuentas y tarjetas que el usuario dejo guardadas.
//
// La pantalla de metodos de pago del frontend mostraba datos escritos en el
// codigo y sus botones no hacian nada.
