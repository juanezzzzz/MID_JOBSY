package services

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// Tienda implementa el catalogo de productos y el checkout.
//
// El backend no calcula ni valida nada: los importes de facturacion son
// columnas que el cliente rellena, el stock no se decrementa y los cupones no
// se comprueban. Aqui se calcula el total desde los precios vigentes de la
// base, se verifica el stock y se descuenta.
type Tienda struct {
	tienda   *ClienteUpstream
	catalogo *Catalogo
	pagos    *Pagos
}

// NuevoTienda crea el servicio de tienda sobre su upstream, el catalogo y el
// servicio de pagos.
func NuevoTienda(t *ClienteUpstream, cat *Catalogo, pagos *Pagos) *Tienda {
	return &Tienda{tienda: t, catalogo: cat, pagos: pagos}
}

type filaCupon struct {
	Id            int     `json:"Id"`
	Codigo        string  `json:"Codigo"`
	DescuentoPct  int     `json:"DescuentoPct"`
	DescuentoFijo float64 `json:"DescuentoFijo"`
	UsosMaximos   int     `json:"UsosMaximos"`
	UsosActuales  int     `json:"UsosActuales"`
	Activo        bool    `json:"Activo"`
	ExpiraEn      string  `json:"ExpiraEn"`
}

// ValidarCupon comprueba un codigo y devuelve el descuento que aplicaria.
//
// El backend no valida nada de esto: ni la vigencia, ni el limite de usos, ni
// siquiera incrementa usos_actuales. Sin esta comprobacion un cupon caducado
// se aplicaria igual y uno de un solo uso serviria infinitas veces.
func (s *Tienda) ValidarCupon(ctx context.Context, codigo string, subtotal float64) (*models.Cupon, error) {
	codigo = strings.ToUpper(strings.TrimSpace(codigo))
	if codigo == "" {
		return nil, models.Validacion("Escribe un codigo de cupon.").ConCampo("codigo", "Obligatorio.")
	}

	var filas []filaCupon
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Codigo": codigo}))
	o.Query.Set("limit", "5")
	if err := s.tienda.Get(ctx, "cupones", o, &filas); err != nil {
		return nil, err
	}

	var c *filaCupon
	for i := range filas {
		if strings.EqualFold(strings.TrimSpace(filas[i].Codigo), codigo) {
			c = &filas[i]
			break
		}
	}
	if c == nil || !c.Activo {
		return nil, models.NoEncontrado("ese cupon")
	}
	if c.ExpiraEn != "" {
		if t, err := time.Parse(time.RFC3339, c.ExpiraEn); err == nil && t.Before(time.Now()) {
			return nil, models.Conflicto("Ese cupon ya expiro.")
		}
	}
	if c.UsosMaximos > 0 && c.UsosActuales >= c.UsosMaximos {
		return nil, models.Conflicto("Ese cupon ya alcanzo su limite de usos.")
	}

	descuento := c.DescuentoFijo
	if c.DescuentoPct > 0 {
		descuento = subtotal * float64(c.DescuentoPct) / 100
	}
	// Un descuento no puede superar el subtotal: dejaria el total negativo.
	if descuento > subtotal {
		descuento = subtotal
	}

	return &models.Cupon{
		ID:         strconv.Itoa(c.Id),
		Codigo:     c.Codigo,
		Descuento:  redondear(descuento, 2),
		Porcentaje: c.DescuentoPct,
	}, nil
}

// consumirCupon incrementa el contador de usos. Nadie lo hacia.
func (s *Tienda) consumirCupon(ctx context.Context, idCupon int) error {
	_, err := s.tienda.Parchear(ctx, "cupones/"+strconv.Itoa(idCupon), func(f map[string]any) error {
		usos, _ := f["UsosActuales"].(float64)
		f["UsosActuales"] = int(usos) + 1
		return nil
	})
	return err
}

type filaCategoria struct {
	Id     int    `json:"Id"`
	Nombre string `json:"Nombre"`
	Icono  string `json:"Icono"`
	Activa bool   `json:"Activa"`
}

type filaProducto struct {
	Id                 int            `json:"Id"`
	IdCategoriasTienda *filaCategoria `json:"IdCategoriasTienda"`
	Nombre             string         `json:"Nombre"`
	Descripcion        string         `json:"Descripcion"`
	Precio             float64        `json:"Precio"`
	DescuentoPct       int            `json:"DescuentoPct"`
	Stock              int            `json:"Stock"`
	ImagenUrl          string         `json:"ImagenUrl"`
	EsNuevo            bool           `json:"EsNuevo"`
	Destacado          bool           `json:"Destacado"`
	RatingPromedio     float64        `json:"RatingPromedio"`
	Activo             bool           `json:"Activo"`
}

type filaPedido struct {
	Id         int         `json:"Id"`
	IdUsuarios int         `json:"IdUsuarios"`
	IdCarritos *refAnidada `json:"IdCarritos"`
	IdEstado   int         `json:"IdEstado"`
	Subtotal   float64     `json:"Subtotal"`
	Envio      float64     `json:"Envio"`
	Total      float64     `json:"Total"`
	Activo     bool        `json:"Activo"`
	CreadoEn   string      `json:"CreadoEn"`
}

type filaPedidoItem struct {
	Id             int         `json:"Id"`
	IdPedidos      *refAnidada `json:"IdPedidos"`
	IdProductos    *refAnidada `json:"IdProductos"`
	Cantidad       int         `json:"Cantidad"`
	PrecioUnitario float64     `json:"PrecioUnitario"`
	Subtotal       float64     `json:"Subtotal"`
}

// estadoProducto deriva el estado que pinta el frontend, que la base no guarda
// como tal: lo infiere del stock y de la marca de destacado.
func estadoProducto(f filaProducto) string {
	switch {
	case !f.Activo:
		return "borrador"
	case f.Stock <= 0:
		return "agotado"
	case f.Destacado:
		return "destacado"
	default:
		return "publicado"
	}
}

// productoADominio convierte la fila del producto en el producto que ve el
// front.
func productoADominio(f filaProducto) models.Producto {
	cat := ""
	if f.IdCategoriasTienda != nil {
		cat = f.IdCategoriasTienda.Nombre
	}
	return models.Producto{
		ID: strconv.Itoa(f.Id),
		// La base no tiene columna de SKU; se deriva del id para que el
		// frontend tenga algo estable que mostrar.
		SKU:          fmt.Sprintf("JBS-%05d", f.Id),
		Nombre:       f.Nombre,
		Categoria:    cat,
		Descripcion:  f.Descripcion,
		Precio:       f.Precio,
		Moneda:       "COP",
		Stock:        f.Stock,
		ImagenURL:    f.ImagenUrl,
		Calificacion: f.RatingPromedio,
		Estado:       estadoProducto(f),
	}
}

// Productos lista el catalogo, opcionalmente filtrado por categoria.
func (s *Tienda) Productos(ctx context.Context, categoria string, pagina, tamanoPag int) (models.Pagina[models.Producto], error) {
	var vacia models.Pagina[models.Producto]

	if pagina < 1 {
		pagina = 1
	}
	if tamanoPag < 1 || tamanoPag > 100 {
		tamanoPag = 12
	}

	filtros := map[string]string{"Activo": "true"}
	// El frontend manda el NOMBRE de la categoria, no su id: su modelo
	// Product.categoria es texto. Hay que resolverlo a id antes de filtrar.
	if c := strings.TrimSpace(categoria); c != "" {
		id, err := s.idCategoria(ctx, c)
		if err != nil {
			// Una categoria que no existe no es un error del servidor: es un
			// listado vacio.
			return models.NuevaPagina([]models.Producto{}, pagina, tamanoPag, 0, false), nil
		}
		filtros["IdCategoriasTienda"] = strconv.Itoa(id)
	}

	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(filtros))
	o.Query.Set("limit", strconv.Itoa(tamanoPag+1))
	o.Query.Set("offset", strconv.Itoa((pagina-1)*tamanoPag))
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")

	var filas []filaProducto
	if err := s.tienda.Get(ctx, "productos", o, &filas); err != nil {
		return vacia, err
	}

	hayMas := len(filas) > tamanoPag
	if hayMas {
		filas = filas[:tamanoPag]
	}

	items := make([]models.Producto, 0, len(filas))
	for _, f := range filas {
		items = append(items, productoADominio(f))
	}
	total := (pagina-1)*tamanoPag + len(items)
	return models.NuevaPagina(items, pagina, tamanoPag, total, hayMas), nil
}

// ProductoPorID devuelve un producto activo por id; los retirados se tratan
// como no encontrados.
func (s *Tienda) ProductoPorID(ctx context.Context, idProducto string) (*models.Producto, error) {
	id, err := IDNumerico(idProducto)
	if err != nil {
		return nil, err
	}

	var filas []filaProducto
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.tienda.Get(ctx, "productos", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return nil, models.NoEncontrado("ese producto")
	}
	p := productoADominio(filas[0])
	return &p, nil
}

// Categorias devuelve las categorias con su conteo de productos.
func (s *Tienda) Categorias(ctx context.Context) ([]models.Categoria, error) {
	var cats []filaCategoria
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "200")
	if err := s.tienda.Get(ctx, "categorias_tienda", o, &cats); err != nil {
		return nil, err
	}

	// El conteo por categoria no lo da el backend. Se lee la pagina de
	// productos una vez y se cuenta en memoria: una peticion por categoria
	// seria N+1.
	var prods []filaProducto
	op := &OpcionesUpstream{Query: map[string][]string{}}
	op.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
	op.Query.Set("limit", "1000")
	_ = s.tienda.Get(ctx, "productos", op, &prods)

	conteo := map[int]int{}
	for _, p := range prods {
		if p.IdCategoriasTienda != nil {
			conteo[p.IdCategoriasTienda.Id]++
		}
	}

	out := make([]models.Categoria, 0, len(cats))
	for _, c := range cats {
		if !c.Activa {
			continue
		}
		out = append(out, models.Categoria{
			ID:                strconv.Itoa(c.Id),
			Nombre:            c.Nombre,
			Slug:              babosa(c.Nombre),
			CantidadProductos: conteo[c.Id],
		})
	}
	return out, nil
}

// babosa convierte un nombre en un identificador apto para una URL.
func babosa(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	reemplazos := map[rune]rune{'á': 'a', 'é': 'e', 'í': 'i', 'ó': 'o', 'ú': 'u', 'ñ': 'n'}
	var b strings.Builder
	for _, r := range s {
		if v, ok := reemplazos[r]; ok {
			r = v
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// idCategoria resuelve el id de una categoria activa por nombre.
func (s *Tienda) idCategoria(ctx context.Context, nombre string) (int, error) {
	var cats []filaCategoria
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "200")
	if err := s.tienda.Get(ctx, "categorias_tienda", o, &cats); err != nil {
		return 0, err
	}
	// Solo entre las activas: una categoria retirada conserva su nombre en la
	// tabla, y si otra nueva lo reutiliza, buscar sin mirar Activa devolvia la
	// vieja. El producto acababa en una categoria que ya no existe para nadie.
	for _, c := range cats {
		if c.Activa && strings.EqualFold(strings.TrimSpace(c.Nombre), strings.TrimSpace(nombre)) {
			return c.Id, nil
		}
	}
	// El frontend tambien puede mandar el id directamente.
	if n, err := strconv.Atoi(nombre); err == nil && n > 0 {
		return n, nil
	}
	return 0, models.NoEncontrado("esa categoria")
}

// ItemCarrito es una linea del checkout tal como la manda el frontend.
//
// El carrito del front vive en localStorage con el Product entero embebido, no
// solo su id, asi que hay que declarar todos sus campos: el lector de JSON
// rechaza los desconocidos y si no, el carrito real daria un 400.
//
// De todo esto solo se confia en el id. El precio y el stock se releen de la
// base en el checkout, porque lo que llega del cliente pudo caducar o estar
// manipulado.
type ProductoCliente struct {
	ID           string  `json:"id"`
	SKU          string  `json:"sku,omitempty"`
	Nombre       string  `json:"nombre,omitempty"`
	Categoria    string  `json:"categoria,omitempty"`
	Descripcion  string  `json:"descripcion,omitempty"`
	Precio       float64 `json:"precio,omitempty"`
	Moneda       string  `json:"moneda,omitempty"`
	Stock        int     `json:"stock,omitempty"`
	ImagenURL    string  `json:"imagenUrl,omitempty"`
	Calificacion float64 `json:"calificacion,omitempty"`
	Estado       string  `json:"estado,omitempty"`
}

type ItemCarrito struct {
	Producto ProductoCliente `json:"product"`
	Cantidad int             `json:"cantidad"`
}

// PeticionCheckout es el cuerpo de POST /tienda/pedidos.
//
// Los campos del instrumento de pago son opcionales porque no todos los
// metodos los usan: una tarjeta necesita titular y ultimos digitos, PSE
// necesita el banco, y el saldo interno no necesita nada.
type PeticionCheckout struct {
	Items      []ItemCarrito `json:"items"`
	MetodoPago string        `json:"metodoPago"`
	Cupon      string        `json:"cupon,omitempty"`
	Titular    string        `json:"titular,omitempty"`
	Ultimos4   string        `json:"ultimos4,omitempty"`
	Banco      string        `json:"banco,omitempty"`
	TipoCuenta string        `json:"tipoCuenta,omitempty"`
	Correo     string        `json:"correo,omitempty"`
}

const envioFijo = 8000.0

// Checkout crea el pedido.
//
// El precio NUNCA se toma del cliente: se relee de la base. El carrito del
// frontend vive en localStorage con el producto entero embebido, asi que su
// precio puede estar caducado o manipulado.
func (s *Tienda) Checkout(ctx context.Context, idUsuario string, p PeticionCheckout) (*models.Pedido, error) {
	usuario, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	if len(p.Items) == 0 {
		return nil, models.Validacion("El carrito esta vacio.")
	}
	if strings.TrimSpace(p.MetodoPago) == "" {
		return nil, models.Validacion("Falta el metodo de pago.").ConCampo("metodoPago", "Obligatorio.")
	}
	// Con tarjeta el comprobante lleva titular y ultimos digitos: tienen que
	// ser un nombre y cuatro digitos, no cualquier texto.
	if strings.EqualFold(strings.TrimSpace(p.MetodoPago), "tarjeta") {
		if err := ValidarNombrePersona("titular", "El titular de la tarjeta", p.Titular); err != nil {
			return nil, err
		}
		if err := ValidarUltimos4("ultimos4", p.Ultimos4); err != nil {
			return nil, err
		}
	}
	// En PSE "correo" es el correo del comprobante; las billeteras mandan ahi
	// el celular asociado, que tiene que ser un celular.
	if c := strings.TrimSpace(p.Correo); c != "" {
		switch strings.ToLower(strings.TrimSpace(p.MetodoPago)) {
		case "pse":
			if err := ValidarEmail(c); err != nil {
				return nil, err
			}
		case "nequi", "daviplata":
			if _, err := ValidarTelefono("correo", c); err != nil {
				return nil, err
			}
		}
	}

	type linea struct {
		prod     filaProducto
		cantidad int
	}
	lineas := make([]linea, 0, len(p.Items))
	subtotal := 0.0

	for _, it := range p.Items {
		if it.Cantidad <= 0 {
			return nil, models.Validacion("Las cantidades deben ser mayores que cero.")
		}
		id, err := IDNumerico(it.Producto.ID)
		if err != nil {
			return nil, models.Validacion("Hay un producto invalido en el carrito.")
		}

		var filas []filaProducto
		o := &OpcionesUpstream{Query: map[string][]string{}}
		o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
		o.Query.Set("limit", "1")
		if err := s.tienda.Get(ctx, "productos", o, &filas); err != nil {
			return nil, err
		}
		if len(filas) == 0 || !filas[0].Activo {
			return nil, models.Validacion(fmt.Sprintf("El producto %s ya no esta disponible.", it.Producto.ID))
		}
		prod := filas[0]
		if prod.Stock < it.Cantidad {
			return nil, models.Conflicto(fmt.Sprintf(
				"No hay stock suficiente de %q: quedan %d.", prod.Nombre, prod.Stock))
		}

		lineas = append(lineas, linea{prod: prod, cantidad: it.Cantidad})
		subtotal += prod.Precio * float64(it.Cantidad)
	}

	// El cupon se resuelve contra el subtotal ya calculado con precios de la
	// base, nunca contra el que mande el cliente.
	descuento := 0.0
	var idCupon int
	if strings.TrimSpace(p.Cupon) != "" {
		c, err := s.ValidarCupon(ctx, p.Cupon, subtotal)
		if err != nil {
			return nil, err
		}
		descuento = c.Descuento
		idCupon, _ = strconv.Atoi(c.ID)
	}

	idEstadoCarrito, err := s.catalogo.IDEstado(ctx, "carrito", "pagado")
	if err != nil {
		return nil, err
	}
	idEstadoPedido, err := s.catalogo.IDEstado(ctx, "pedido", "pendiente")
	if err != nil {
		return nil, err
	}

	// facturacion exige un carrito por clave foranea, asi que se crea uno ya
	// cerrado para dejar el pedido bien enlazado.
	var carrito struct {
		Id int `json:"Id"`
	}
	if err := s.tienda.Post(ctx, "carritos", map[string]any{
		"IdUsuarios": usuario,
		"IdEstado":   idEstadoCarrito,
		"Activo":     false,
	}, &carrito); err != nil {
		return nil, err
	}

	total := subtotal - descuento + envioFijo
	if total < 0 {
		total = 0
	}
	var pedido filaPedido
	if err := s.tienda.Post(ctx, "facturacion", map[string]any{
		"IdUsuarios":     usuario,
		"IdCarritos":     map[string]any{"Id": carrito.Id},
		"IdEstado":       idEstadoPedido,
		"Subtotal":       subtotal,
		"DescuentoTotal": descuento,
		"CuponDescuento": descuento,
		"Envio":          envioFijo,
		"Total":          total,
		"Activo":         true,
	}, &pedido); err != nil {
		return nil, err
	}
	if pedido.Id == 0 {
		return nil, models.Upstream("tienda", fmt.Errorf("el alta del pedido no devolvio id"))
	}

	items := make([]models.ItemPedido, 0, len(lineas))
	for _, l := range lineas {
		sub := l.prod.Precio * float64(l.cantidad)
		var creado map[string]any
		if err := s.tienda.Post(ctx, "pedido_items", map[string]any{
			"IdPedidos":      map[string]any{"Id": pedido.Id},
			"IdProductos":    map[string]any{"Id": l.prod.Id},
			"Cantidad":       l.cantidad,
			"PrecioUnitario": l.prod.Precio,
			"Subtotal":       sub,
			"Activo":         true,
		}, &creado); err != nil {
			return nil, err
		}

		// Descontar stock. Nadie mas lo hace en todo el sistema.
		if err := s.descontarStock(ctx, l.prod, l.cantidad); err != nil {
			return nil, err
		}

		items = append(items, models.ItemPedido{
			ProductoID: strconv.Itoa(l.prod.Id),
			Nombre:     l.prod.Nombre,
			Cantidad:   l.cantidad,
			Precio:     l.prod.Precio,
		})
	}

	// Ahora si se cobra. Hasta aqui el pedido existe y el stock esta
	// reservado; el cobro puede salir aprobado, pendiente (PSE sale al banco)
	// o rechazado, y el pedido queda reflejando ese estado.
	cobro, err := s.pagos.Cobrar(ctx, usuario, p.Correo, pedido.Id, nil, total, DatosPago{
		Metodo:     p.MetodoPago,
		Titular:    p.Titular,
		Ultimos4:   p.Ultimos4,
		Banco:      p.Banco,
		TipoCuenta: p.TipoCuenta,
	})
	if err != nil {
		return nil, err
	}

	if cobro.Estado == "rechazado" {
		// El cobro fallo: se devuelve el stock y se anula el pedido, porque
		// dejar unidades reservadas por un pago que no entro las haria
		// invendibles.
		for _, l := range lineas {
			_ = s.restaurarStock(ctx, l.prod)
		}
		_ = s.anularPedido(ctx, pedido.Id, usuario, carrito.Id)
		return nil, models.Conflicto(cobro.Mensaje)
	}

	if idCupon > 0 {
		_ = s.consumirCupon(ctx, idCupon)
	}

	estadoPedido := "pendiente"
	if cobro.Estado == "aprobado" {
		estadoPedido = "pagado"
		_ = s.marcarPedidoPagado(ctx, pedido.Id, usuario, carrito.Id, subtotal, descuento, total)
	}

	return &models.Pedido{
		ID:             strconv.Itoa(pedido.Id),
		Numero:         NumeroPedido(pedido.Id),
		Items:          items,
		Total:          total,
		Subtotal:       subtotal,
		Descuento:      descuento,
		Envio:          envioFijo,
		Estado:         estadoPedido,
		MetodoPago:     p.MetodoPago,
		Fecha:          time.Now().UTC().Format(time.RFC3339),
		PagoID:         cobro.PagoID,
		EstadoPago:     cobro.Estado,
		URLRedireccion: cobro.URLRedireccion,
	}, nil
}

// anularPedido marca el pedido como cancelado cuando el cobro no prospera.
func (s *Tienda) anularPedido(ctx context.Context, idPedido, idUsuario, idCarrito int) error {
	idEstado, err := s.catalogo.IDEstado(ctx, "pedido", "cancelado")
	if err != nil {
		return err
	}
	return s.tienda.Fijar(ctx, "facturacion/"+strconv.Itoa(idPedido), map[string]any{
		"IdEstado": idEstado,
		"Activo":   false,
	})
}

// marcarPedidoPagado pasa el pedido al estado "pagado".
func (s *Tienda) marcarPedidoPagado(ctx context.Context, idPedido, idUsuario, idCarrito int, subtotal, descuento, total float64) error {
	idEstado, err := s.catalogo.IDEstado(ctx, "pedido", "pagado")
	if err != nil {
		return err
	}
	return s.tienda.Fijar(ctx, "facturacion/"+strconv.Itoa(idPedido), map[string]any{"IdEstado": idEstado})
}

// restaurarStock devuelve el producto a las unidades que tenia antes del
// descuento.
//
// No vale con descontar una cantidad negativa: la fila que se conserva en
// memoria es la ANTERIOR al descuento, asi que restar un negativo dejaria el
// stock por encima del original. Se escribe el valor exacto.
func (s *Tienda) restaurarStock(ctx context.Context, p filaProducto) error {
	return s.fijarStock(ctx, p, p.Stock)
}

// descontarStock reenvia el producto entero porque el PUT del backend
// sobrescribe todas las columnas.
func (s *Tienda) descontarStock(ctx context.Context, p filaProducto, cantidad int) error {
	nuevo := p.Stock - cantidad
	if nuevo < 0 {
		nuevo = 0
	}
	return s.fijarStock(ctx, p, nuevo)
}

// fijarStock escribe un valor concreto de existencias.
func (s *Tienda) fijarStock(ctx context.Context, p filaProducto, nuevo int) error {
	_, err := s.actualizarProducto(ctx, p.Id, func(f map[string]any) error {
		f["Stock"] = nuevo
		return nil
	})
	return err
}

// actualizarProducto cambia columnas sueltas del producto sin perder el
// resto. Ver ClienteUpstream.Parchear.
func (s *Tienda) actualizarProducto(ctx context.Context, id int, cambiar func(map[string]any) error) (map[string]any, error) {
	return s.tienda.Parchear(ctx, "productos/"+strconv.Itoa(id), cambiar)
}

// MisPedidos devuelve los pedidos del usuario con sus lineas.
func (s *Tienda) MisPedidos(ctx context.Context, idUsuario string) ([]models.Pedido, error) {
	usuario, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	var pedidos []filaPedido
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(usuario),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "100")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.tienda.Get(ctx, "facturacion", o, &pedidos); err != nil {
		return nil, err
	}
	if len(pedidos) == 0 {
		return []models.Pedido{}, nil
	}

	// El historial de pagos da el metodo con que se pago cada pedido.
	metodos := map[string]string{}
	if pagos, err := s.pagos.Mios(ctx, idUsuario); err == nil {
		for _, pg := range pagos {
			if id, ok := pg["pedidoId"].(string); ok {
				if m, ok := pg["metodo"].(string); ok && metodos[id] == "" {
					metodos[id] = m
				}
			}
		}
	}
	out, err := s.PedidosADominio(ctx, pedidos)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].MetodoPago = metodos[out[i].ID]
	}
	return out, nil
}

// NumeroPedido es el numero que ve el cliente. Antes habia tres formatos para
// el mismo pedido segun la pantalla, y uno coincidia con el de los SKU.
func NumeroPedido(id int) string {
	return fmt.Sprintf("PED-%05d", id)
}

// PedidosADominio completa los pedidos con sus lineas, el nombre de cada
// producto y el estado.
//
// Las lineas se piden pedido a pedido y en paralelo. Leerlas todas de una vez
// y cruzarlas en memoria obligaba a un tope de filas que, superado, dejaba
// pedidos sin productos sin avisar.
func (s *Tienda) PedidosADominio(ctx context.Context, pedidos []filaPedido) ([]models.Pedido, error) {
	lineas := make([][]filaPedidoItem, len(pedidos))
	tareas := make([]func() error, 0, len(pedidos)+1)
	for i, p := range pedidos {
		i, p := i, p
		tareas = append(tareas, func() error {
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("query", QueryBeego(map[string]string{"IdPedidos": strconv.Itoa(p.Id)}))
			o.Query.Set("limit", "200")
			return s.tienda.Get(ctx, "pedido_items", o, &lineas[i])
		})
	}
	nombres := map[int]string{}
	tareas = append(tareas, func() error {
		var prods []filaProducto
		o := &OpcionesUpstream{Query: map[string][]string{}}
		o.Query.Set("limit", "1000")
		if err := s.tienda.Get(ctx, "productos", o, &prods); err != nil {
			return err
		}
		for _, pr := range prods {
			nombres[pr.Id] = pr.Nombre
		}
		return nil
	})
	if err := EnParalelo(tareas...); err != nil {
		return nil, err
	}

	out := make([]models.Pedido, 0, len(pedidos))
	for i, p := range pedidos {
		estado := "pendiente"
		if n, err := s.catalogo.EstadoDesdeID(ctx, p.IdEstado); err == nil {
			estado = n
		}
		items := make([]models.ItemPedido, 0, len(lineas[i]))
		for _, it := range lineas[i] {
			item := models.ItemPedido{Cantidad: it.Cantidad, Precio: it.PrecioUnitario}
			if it.IdProductos != nil {
				item.ProductoID = strconv.Itoa(it.IdProductos.Id)
				item.Nombre = nombres[it.IdProductos.Id]
			}
			items = append(items, item)
		}
		out = append(out, models.Pedido{
			ID:     strconv.Itoa(p.Id),
			Numero: NumeroPedido(p.Id),
			Items:  items,
			Total:  p.Total,
			Estado: estado,
			Fecha:  p.CreadoEn,
		})
	}
	return out, nil
}

// Gestion del catalogo de la tienda desde el panel de administracion.
//
// El panel editaba productos, categorias e inventario sobre datos de ejemplo
// guardados en localStorage: nada de lo que hacia el administrador llegaba a
// la base, ni se veia en la tienda.

// PeticionProducto es el cuerpo de alta y edicion. En la edicion todos los
// campos son opcionales: solo se cambia lo que viene.
//
// El SKU no se acepta: la base no tiene esa columna y el mid lo deriva del id.
type PeticionProducto struct {
	Nombre      *string  `json:"nombre"`
	Categoria   *string  `json:"categoria"`
	Descripcion *string  `json:"descripcion"`
	Precio      *float64 `json:"precio"`
	Stock       *int     `json:"stock"`
	ImagenURL   *string  `json:"imagenUrl"`
	Estado      *string  `json:"estado"`
}

// AdminProductos lista todo el catalogo, tambien los productos en borrador,
// que la tienda publica no muestra.
func (s *Tienda) AdminProductos(ctx context.Context) ([]models.Producto, error) {
	var filas []filaProducto
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "1000")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.tienda.Get(ctx, "productos", o, &filas); err != nil {
		return nil, err
	}
	out := make([]models.Producto, 0, len(filas))
	for _, f := range filas {
		out = append(out, productoADominio(f))
	}
	return out, nil
}

// aplicarEstado traduce el estado del panel a las columnas que lo guardan.
// "agotado" no es una columna: es stock cero.
func aplicarEstado(f map[string]any, estado string) error {
	switch estado {
	case "publicado":
		f["Activo"], f["Destacado"] = true, false
	case "destacado":
		f["Activo"], f["Destacado"] = true, true
	case "borrador":
		f["Activo"], f["Destacado"] = false, false
	case "agotado":
		f["Activo"], f["Stock"] = true, 0
	default:
		return models.Validacion("Estado no valido: "+estado).ConCampo("estado", "Usa publicado, destacado, borrador o agotado.")
	}
	return nil
}

// validarProducto revisa los datos de un producto. En el alta exige nombre,
// categoria y precio; en la edicion solo valida lo que venga.
func validarProducto(p PeticionProducto, alta bool) error {
	if alta && (p.Nombre == nil || p.Categoria == nil || p.Precio == nil) {
		return models.Validacion("Faltan datos del producto.").ConCampo("nombre", "Nombre, categoria y precio son obligatorios.")
	}
	if p.Nombre != nil && strings.TrimSpace(*p.Nombre) == "" {
		return models.Validacion("El nombre es obligatorio.").ConCampo("nombre", "Obligatorio.")
	}
	if p.Precio != nil && *p.Precio <= 0 {
		return models.Validacion("El precio debe ser mayor que cero.").ConCampo("precio", "Mayor que cero.")
	}
	if p.Stock != nil && *p.Stock < 0 {
		return models.Validacion("El stock no puede ser negativo.").ConCampo("stock", "Cero o mas.")
	}
	if p.Nombre != nil {
		if err := ValidarTexto("nombre", "El nombre", *p.Nombre, 2, 120); err != nil {
			return err
		}
	}
	if p.Descripcion != nil {
		if err := ValidarTexto("descripcion", "La descripcion", *p.Descripcion, 0, 1000); err != nil {
			return err
		}
	}
	if p.Precio != nil && *p.Precio > 10_000_000 {
		return models.Validacion("El precio no puede superar 10.000.000.").ConCampo("precio", "Demasiado alto.")
	}
	if p.Stock != nil && *p.Stock > 100_000 {
		return models.Validacion("El stock no puede superar 100.000 unidades.").ConCampo("stock", "Demasiado alto.")
	}
	return nil
}

// volcar copia a la fila los campos presentes en la peticion.
func (s *Tienda) volcar(ctx context.Context, f map[string]any, p PeticionProducto) error {
	if p.Nombre != nil {
		f["Nombre"] = strings.TrimSpace(*p.Nombre)
	}
	if p.Descripcion != nil {
		f["Descripcion"] = strings.TrimSpace(*p.Descripcion)
	}
	if p.Precio != nil {
		f["Precio"] = *p.Precio
	}
	if p.Stock != nil {
		f["Stock"] = *p.Stock
	}
	if p.ImagenURL != nil {
		f["ImagenUrl"] = strings.TrimSpace(*p.ImagenURL)
	}
	if p.Categoria != nil {
		id, err := s.idCategoria(ctx, *p.Categoria)
		if err != nil {
			return models.Validacion("La categoria no existe.").ConCampo("categoria", "Elige una categoria existente.")
		}
		f["IdCategoriasTienda"] = map[string]any{"Id": id}
	}
	if p.Estado != nil {
		return aplicarEstado(f, *p.Estado)
	}
	return nil
}

// AdminCrearProducto da de alta un producto. Sin estado explicito nace en
// borrador, para que no aparezca en la tienda a medio completar.
func (s *Tienda) AdminCrearProducto(ctx context.Context, p PeticionProducto) (*models.Producto, error) {
	if err := validarProducto(p, true); err != nil {
		return nil, err
	}
	// El tipo de producto es obligatorio en la base y el panel no lo pide.
	idTipo, err := s.catalogo.IDTipo(ctx, "producto_tienda", "insumos_aseo")
	if err != nil {
		return nil, err
	}
	f := map[string]any{
		"IdTipo":         idTipo,
		"Descripcion":    "",
		"PrecioOriginal": 0,
		"DescuentoPct":   0,
		"Stock":          0,
		"ImagenUrl":      "",
		"EsNuevo":        true,
		"Destacado":      false,
		"EnvioGratis":    false,
		"RatingPromedio": 0,
		"TotalResenas":   0,
		"Activo":         false,
	}
	if p.Estado == nil {
		borrador := "borrador"
		p.Estado = &borrador
	}
	if err := s.volcar(ctx, f, p); err != nil {
		return nil, err
	}
	var creado filaProducto
	if err := s.tienda.Post(ctx, "productos", f, &creado); err != nil {
		return nil, err
	}
	return s.productoAdmin(ctx, creado.Id)
}

// AdminActualizarProducto cambia solo los campos presentes.
func (s *Tienda) AdminActualizarProducto(ctx context.Context, idProducto string, p PeticionProducto) (*models.Producto, error) {
	id, err := IDNumerico(idProducto)
	if err != nil {
		return nil, err
	}
	if err := validarProducto(p, false); err != nil {
		return nil, err
	}
	if _, err := s.actualizarProducto(ctx, id, func(f map[string]any) error {
		return s.volcar(ctx, f, p)
	}); err != nil {
		return nil, err
	}
	return s.productoAdmin(ctx, id)
}

// AdminAjustarStock suma o resta unidades. Nunca deja el stock por debajo de
// cero.
func (s *Tienda) AdminAjustarStock(ctx context.Context, idProducto string, delta int) (*models.Producto, error) {
	id, err := IDNumerico(idProducto)
	if err != nil {
		return nil, err
	}
	if delta == 0 {
		return nil, models.Validacion("Indica cuantas unidades sumar o restar.").ConCampo("delta", "Distinto de cero.")
	}
	if _, err := s.actualizarProducto(ctx, id, func(f map[string]any) error {
		actual, _ := f["Stock"].(float64)
		nuevo := int(actual) + delta
		if nuevo < 0 {
			nuevo = 0
		}
		f["Stock"] = nuevo
		return nil
	}); err != nil {
		return nil, err
	}
	return s.productoAdmin(ctx, id)
}

// AdminBorrarProducto elimina un producto sin ventas.
//
// Uno con ventas no se puede borrar: sus lineas de pedido lo referencian y el
// historial de compras de los clientes quedaria roto. Para retirarlo de la
// tienda se pasa a borrador.
func (s *Tienda) AdminBorrarProducto(ctx context.Context, idProducto string) error {
	id, err := IDNumerico(idProducto)
	if err != nil {
		return err
	}
	if _, err := s.productoAdmin(ctx, id); err != nil {
		return err
	}
	var vendidos []filaPedidoItem
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdProductos": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.tienda.Get(ctx, "pedido_items", o, &vendidos); err != nil {
		return err
	}
	if len(vendidos) > 0 {
		return models.Conflicto("Este producto tiene ventas registradas y no se puede borrar. Pasalo a borrador para retirarlo de la tienda.")
	}

	// Las lineas de carrito son transitorias: se quitan para que la clave
	// foranea no bloquee el borrado.
	var lineas []filaCarritoItem
	oc := &OpcionesUpstream{Query: map[string][]string{}}
	oc.Query.Set("query", QueryBeego(map[string]string{"IdProductos": strconv.Itoa(id)}))
	oc.Query.Set("limit", "1000")
	if err := s.tienda.Get(ctx, "tr_carrito_items", oc, &lineas); err != nil {
		return err
	}
	for _, l := range lineas {
		if err := s.tienda.Delete(ctx, "tr_carrito_items/"+strconv.Itoa(l.Id)); err != nil {
			return err
		}
	}
	return s.tienda.Delete(ctx, "productos/"+strconv.Itoa(id))
}

// productoAdmin devuelve un producto por id aunque este retirado, para el panel
// de administracion.
func (s *Tienda) productoAdmin(ctx context.Context, id int) (*models.Producto, error) {
	var filas []filaProducto
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.tienda.Get(ctx, "productos", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 {
		return nil, models.NoEncontrado("ese producto")
	}
	p := productoADominio(filas[0])
	return &p, nil
}

// ---- Categorias ----

// categoriaPorID devuelve una categoria activa por id.
func (s *Tienda) categoriaPorID(ctx context.Context, id int) (*filaCategoria, error) {
	var cats []filaCategoria
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.tienda.Get(ctx, "categorias_tienda", o, &cats); err != nil {
		return nil, err
	}
	if len(cats) == 0 || !cats[0].Activa {
		return nil, models.NoEncontrado("esa categoria")
	}
	return &cats[0], nil
}

// exigirNombreLibre impide dos categorias activas con el mismo nombre: el
// frontend filtra productos por nombre de categoria.
func (s *Tienda) exigirNombreLibre(ctx context.Context, nombre string, excepto int) error {
	var cats []filaCategoria
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "200")
	if err := s.tienda.Get(ctx, "categorias_tienda", o, &cats); err != nil {
		return err
	}
	for _, c := range cats {
		if c.Activa && c.Id != excepto && strings.EqualFold(strings.TrimSpace(c.Nombre), nombre) {
			return models.Conflicto("Ya existe una categoria con ese nombre.")
		}
	}
	return nil
}

// AdminCrearCategoria da de alta una categoria vacia.
func (s *Tienda) AdminCrearCategoria(ctx context.Context, nombre string) (*models.Categoria, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return nil, models.Validacion("Escribe un nombre.").ConCampo("nombre", "Obligatorio.")
	}
	if err := ValidarTexto("nombre", "El nombre de la categoria", nombre, 2, 60); err != nil {
		return nil, err
	}
	if err := s.exigirNombreLibre(ctx, nombre, 0); err != nil {
		return nil, err
	}
	var creada filaCategoria
	if err := s.tienda.Post(ctx, "categorias_tienda", map[string]any{
		"Nombre": nombre,
		"Icono":  "",
		"Activa": true,
	}, &creada); err != nil {
		return nil, err
	}
	return &models.Categoria{ID: strconv.Itoa(creada.Id), Nombre: nombre, Slug: babosa(nombre)}, nil
}

// AdminRenombrarCategoria cambia el nombre. Los productos la referencian por
// id, asi que no hay que tocarlos.
func (s *Tienda) AdminRenombrarCategoria(ctx context.Context, idCategoria, nombre string) (*models.Categoria, error) {
	id, err := IDNumerico(idCategoria)
	if err != nil {
		return nil, err
	}
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return nil, models.Validacion("Escribe un nombre.").ConCampo("nombre", "Obligatorio.")
	}
	if err := ValidarTexto("nombre", "El nombre de la categoria", nombre, 2, 60); err != nil {
		return nil, err
	}
	if _, err := s.categoriaPorID(ctx, id); err != nil {
		return nil, err
	}
	if err := s.exigirNombreLibre(ctx, nombre, id); err != nil {
		return nil, err
	}
	if err := s.tienda.Fijar(ctx, "categorias_tienda/"+strconv.Itoa(id), map[string]any{"Nombre": nombre}); err != nil {
		return nil, err
	}
	return &models.Categoria{ID: strconv.Itoa(id), Nombre: nombre, Slug: babosa(nombre)}, nil
}

// AdminBorrarCategoria retira una categoria sin productos.
func (s *Tienda) AdminBorrarCategoria(ctx context.Context, idCategoria string) error {
	id, err := IDNumerico(idCategoria)
	if err != nil {
		return err
	}
	if _, err := s.categoriaPorID(ctx, id); err != nil {
		return err
	}
	var prods []filaProducto
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdCategoriasTienda": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.tienda.Get(ctx, "productos", o, &prods); err != nil {
		return err
	}
	if len(prods) > 0 {
		return models.Conflicto("La categoria tiene productos. Muevelos a otra antes de borrarla.")
	}
	return s.tienda.Fijar(ctx, "categorias_tienda/"+strconv.Itoa(id), map[string]any{"Activa": false})
}

type ResultadoReembolso struct {
	PedidoID string  `json:"pedidoId"`
	PagoID   string  `json:"pagoId,omitempty"`
	Monto    float64 `json:"monto"`
	Estado   string  `json:"estado"`
	Mensaje  string  `json:"mensaje"`
	// StockRepuesto avisa de si las unidades volvieron al catalogo.
	StockRepuesto bool `json:"stockRepuesto"`
}

// Reembolsar deshace un pedido ya cobrado.
//
// Cuatro pasos, en este orden y no en otro: primero se devuelve el dinero por
// donde entro, y solo despues se marca. Si se marcara antes y el proveedor
// rechazara la devolucion, el pedido figuraria como reembolsado sin que nadie
// haya recibido nada.
//
//  1. devolver el dinero (a la pasarela, o al monedero si se pago con saldo)
//  2. el pago pasa a 'reembolsado' y se anota la transaccion
//  3. el pedido pasa a 'reembolsado'
//  4. las unidades vuelven al catalogo
//
// Del 2 al 4 no hay transacciones: si alguno falla queda registrado, porque el
// dinero ya salio y eso hay que poder rastrearlo.
func (s *Tienda) Reembolsar(ctx context.Context, idPedido, motivo string) (*ResultadoReembolso, error) {
	id, err := IDNumerico(idPedido)
	if err != nil {
		return nil, err
	}

	pedido, err := s.pedidoPorID(ctx, id)
	if err != nil {
		return nil, err
	}

	estado, err := s.catalogo.EstadoDesdeID(ctx, pedido.IdEstado)
	if err != nil {
		return nil, err
	}
	if estado == "reembolsado" {
		return nil, models.Conflicto("Ese pedido ya esta reembolsado.")
	}
	// Solo se devuelve lo que se llego a cobrar. Un pedido pendiente (un PSE
	// sin confirmar) o cancelado no tiene dinero que devolver.
	if estado != "pagado" && estado != "enviado" && estado != "entregado" {
		return nil, models.Conflicto(
			"Solo se puede reembolsar un pedido cobrado. Este esta en estado «" + estado + "».")
	}

	pago, err := s.pagoDePedido(ctx, id)
	if err != nil {
		return nil, err
	}
	if pago == nil {
		return nil, models.Conflicto("Ese pedido no tiene un pago registrado, asi que no hay nada que devolver.")
	}

	// 1. Devolver el dinero.
	if err := s.pagos.DevolverDinero(ctx, *pago); err != nil {
		return nil, err
	}

	res := &ResultadoReembolso{
		PedidoID: strconv.Itoa(id),
		PagoID:   strconv.Itoa(pago.Id),
		Monto:    pago.Monto,
		Estado:   "reembolsado",
		Mensaje:  "Se devolvieron " + formatearMonto(pago.Monto) + ".",
	}

	// 2 y 3. Marcar. El dinero ya salio: un fallo aqui deja constancia.
	if err := s.pagos.MarcarReembolsado(ctx, *pago, motivo); err != nil {
		slog.Error("dinero devuelto pero el pago no quedo marcado",
			"pedido", id, "pago", pago.Id, "error", err)
	}
	if idEstado, err := s.catalogo.IDEstado(ctx, "pedido", "reembolsado"); err == nil {
		if err := s.tienda.Fijar(ctx, "facturacion/"+strconv.Itoa(id),
			map[string]any{"IdEstado": idEstado}); err != nil {
			slog.Error("dinero devuelto pero el pedido no quedo marcado",
				"pedido", id, "error", err)
		}
	}

	// 4. Reponer el stock.
	res.StockRepuesto = s.reponerStockDe(ctx, id) == nil
	return res, nil
}

// pedidoPorID lee un pedido activo.
func (s *Tienda) pedidoPorID(ctx context.Context, id int) (*filaPedido, error) {
	var filas []filaPedido
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.tienda.Get(ctx, "facturacion", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return nil, models.NoEncontrado("ese pedido")
	}
	return &filas[0], nil
}

// pagoDePedido busca el cobro aprobado de un pedido.
func (s *Tienda) pagoDePedido(ctx context.Context, idPedido int) (*filaPago, error) {
	var filas []filaPago
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdPedidos": strconv.Itoa(idPedido),
		"Activo":    "true",
	}))
	o.Query.Set("limit", "20")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.tienda.Get(ctx, "pagos", o, &filas); err != nil {
		return nil, err
	}
	for i := range filas {
		n, err := s.catalogo.EstadoDesdeID(ctx, filas[i].IdEstado)
		if err == nil && n == "aprobado" {
			return &filas[i], nil
		}
	}
	return nil, nil
}

// reponerStockDe devuelve al catalogo las unidades de un pedido.
func (s *Tienda) reponerStockDe(ctx context.Context, idPedido int) error {
	var lineas []filaPedidoItem
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdPedidos": strconv.Itoa(idPedido)}))
	o.Query.Set("limit", "200")
	if err := s.tienda.Get(ctx, "pedido_items", o, &lineas); err != nil {
		slog.Error("no se pudo leer las lineas para reponer stock", "pedido", idPedido, "error", err)
		return err
	}
	var fallo error
	for _, l := range lineas {
		if l.IdProductos == nil {
			continue
		}
		// Se suma sobre el valor actual, no sobre el que habia al comprar:
		// entre medias el stock pudo cambiar por otras ventas.
		if _, err := s.actualizarProducto(ctx, l.IdProductos.Id, func(f map[string]any) error {
			actual, _ := f["Stock"].(float64)
			f["Stock"] = int(actual) + l.Cantidad
			return nil
		}); err != nil {
			slog.Error("no se pudo reponer el stock de un producto",
				"pedido", idPedido, "producto", l.IdProductos.Id, "error", err)
			fallo = err
		}
	}
	return fallo
}

// DevolverDinero manda la devolucion por donde entro el cobro.
