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


