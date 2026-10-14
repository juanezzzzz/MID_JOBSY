package services

import (
	"context"
	"strconv"

	"MID_JOBSY/models"
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

func (s *Carrito) abiertoDe(ctx context.Context, idUsuario int)(*filaCarrito, error){
	idActivo, err := s.catalogo.IDEstado(ctx, "carrito", "activo")
	if err != nil{
		return nil, err
	}

	var filas []filaCarrito
	o := &OpcionesUpstram{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(idUsuario),
		"IdEstado": strconv.Itoa(idActivo),
		"Activo": "true",
	}))

	o.Query.Set("limit", "10")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.tienda.Get(ctx, "carritos", o, &filas); 
}