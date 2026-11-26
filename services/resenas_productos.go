package services

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// ResenasProductos guarda las opiniones de la tienda en Tienda_APi
// (tienda.resenas_productos). Antes vivian en el localStorage del navegador:
// solo las veia quien las escribia.
type ResenasProductos struct {
	tienda  *ClienteUpstream
	shop    *Tienda
	cuentas *Cuentas
}

// NuevoResenasProductos crea el servicio sobre Tienda_APi, la tienda (para
// validar el producto y las compras) y las cuentas (para el nombre del autor).
func NuevoResenasProductos(t *ClienteUpstream, shop *Tienda, cuentas *Cuentas) *ResenasProductos {
	return &ResenasProductos{tienda: t, shop: shop, cuentas: cuentas}
}

type filaResenaProducto struct {
	Id           int       `json:"Id"`
	IdProductos  int       `json:"IdProductos"`
	IdUsuarios   int       `json:"IdUsuarios"`
	Autor        string    `json:"Autor"`
	Calificacion int       `json:"Calificacion"`
	Titulo       string    `json:"Titulo"`
	Comentario   string    `json:"Comentario"`
	Fotos        string    `json:"Fotos"`
	Verificado   bool      `json:"Verificado"`
	Activo       bool      `json:"Activo"`
	CreadoEn     time.Time `json:"CreadoEn"`
}

func (f filaResenaProducto) aDominio() models.ResenaProducto {
	fotos := []string{}
	_ = json.Unmarshal([]byte(f.Fotos), &fotos)
	return models.ResenaProducto{
		ID:           strconv.Itoa(f.Id),
		ProductoID:   strconv.Itoa(f.IdProductos),
		Autor:        f.Autor,
		Calificacion: f.Calificacion,
		Titulo:       f.Titulo,
		Comentario:   f.Comentario,
		Fotos:        fotos,
		Fecha:        f.CreadoEn.Format(time.RFC3339),
		Verificado:   f.Verificado,
	}
}

func (s *ResenasProductos) filas(ctx context.Context, filtro map[string]string) ([]filaResenaProducto, error) {
	var filas []filaResenaProducto
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(filtro))
	// shortcut: se muestran las 200 mas recientes por producto; paginar si alguno pasa de ahi.
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.tienda.Get(ctx, "resenas_productos", o, &filas); err != nil {
		return nil, err
	}
	return filas, nil
}

// DeProducto lista las resenas visibles de un producto, la mas nueva primero.
func (s *ResenasProductos) DeProducto(ctx context.Context, idProducto string) ([]models.ResenaProducto, error) {
	id, err := IDNumerico(idProducto)
	if err != nil {
		return nil, err
	}
	filas, err := s.filas(ctx, map[string]string{"IdProductos": strconv.Itoa(id), "Activo": "true"})
	if err != nil {
		return nil, err
	}
	out := make([]models.ResenaProducto, 0, len(filas))
	for _, f := range filas {
		out = append(out, f.aDominio())
	}
	return out, nil
}

// PeticionResenaProducto es lo que envia el formulario (las fotos ya subidas).
type PeticionResenaProducto struct {
	Calificacion int
	Titulo       string
	Comentario   string
	Fotos        []string
}

// Publicar guarda la resena del usuario. Una por persona y producto; quien
// compro el producto (pedido pagado o posterior) queda como compra verificada.
func (s *ResenasProductos) Publicar(ctx context.Context, idUsuario, idProducto string, p PeticionResenaProducto) (*models.ResenaProducto, error) {
	uid, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	p.Titulo, p.Comentario = strings.TrimSpace(p.Titulo), strings.TrimSpace(p.Comentario)
	if p.Calificacion < 1 || p.Calificacion > 5 {
		return nil, models.Validacion("Elige una calificacion de 1 a 5 estrellas.").ConCampo("calificacion", "Entre 1 y 5.")
	}
	if err := ValidarTexto("titulo", "El titulo", p.Titulo, 3, 120); err != nil {
		return nil, err
	}
	if err := ValidarTexto("comentario", "El comentario", p.Comentario, 10, 1000); err != nil {
		return nil, err
	}
	prod, err := s.shop.ProductoPorID(ctx, idProducto)
	if err != nil {
		return nil, err
	}
	pid, _ := strconv.Atoi(prod.ID)

	previas, err := s.filas(ctx, map[string]string{"IdProductos": strconv.Itoa(pid), "IdUsuarios": strconv.Itoa(uid)})
	if err != nil {
		return nil, err
	}
	if len(previas) > 0 {
		return nil, models.Conflicto("Ya publicaste una resena de este producto.")
	}

	autor := "Usuario de Jobsy"
	if u, err := s.cuentas.Perfil(ctx, idUsuario); err == nil {
		// Nombre y la inicial del apellido: suficiente para la tienda sin exponer el nombre completo.
		autor = strings.TrimSpace(u.Nombre)
		if a := strings.TrimSpace(u.Apellido); a != "" {
			autor += " " + string([]rune(a)[0]) + "."
		}
	}

	verificado := false
	if pedidos, err := s.shop.MisPedidos(ctx, idUsuario); err == nil {
		for _, ped := range pedidos {
			if ped.Estado != "pagado" && ped.Estado != "enviado" && ped.Estado != "entregado" {
				continue
			}
			for _, it := range ped.Items {
				if it.ProductoID == prod.ID {
					verificado = true
				}
			}
		}
	}

	fotos, _ := json.Marshal(append([]string{}, p.Fotos...))
	var creada filaResenaProducto
	if err := s.tienda.Post(ctx, "resenas_productos", map[string]any{
		"IdProductos": pid, "IdUsuarios": uid, "Autor": autor,
		"Calificacion": p.Calificacion, "Titulo": p.Titulo, "Comentario": p.Comentario,
		"Fotos": string(fotos), "Verificado": verificado, "Activo": true,
	}, &creada); err != nil {
		return nil, err
	}
	if creada.CreadoEn.IsZero() {
		creada.CreadoEn = time.Now()
	}
	r := creada.aDominio()
	return &r, nil
}
