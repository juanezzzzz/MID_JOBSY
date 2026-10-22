package services

import (
	"context"
	"strconv"

	"api_MID/models"
)

// ---- Resenas ----

// Notificaciones lista y marca como leidas las notificaciones del usuario, y
// las crea desde otros flujos (p. ej. al aceptar una postulacion).
type Notificaciones struct {
	usuarios *ClienteUpstream
	catalogo *Catalogo
}

// NuevoNotificaciones crea el servicio de notificaciones sobre el upstream de
// usuarios y el catalogo.
func NuevoNotificaciones(u *ClienteUpstream, cat *Catalogo) *Notificaciones {
	return &Notificaciones{usuarios: u, catalogo: cat}
}

type filaNotificacion struct {
	Id         int    `json:"Id"`
	IdUsuarios int    `json:"IdUsuarios"`
	IdTipo     int    `json:"IdTipo"`
	Titulo     string `json:"Titulo"`
	Cuerpo     string `json:"Cuerpo"`
	Leida      bool   `json:"Leida"`
	UrlDestino string `json:"UrlDestino"`
	Activo     bool   `json:"Activo"`
	CreadoEn   string `json:"CreadoEn"`
}

// Mias devuelve las notificaciones activas del usuario, de la mas reciente a la
// mas antigua.
func (s *Notificaciones) Mias(ctx context.Context, idUsuario string) ([]models.Notificacion, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	var filas []filaNotificacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "100")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.usuarios.Get(ctx, "notificaciones", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.Notificacion, 0, len(filas))
	for _, f := range filas {
		out = append(out, models.Notificacion{
			ID:      strconv.Itoa(f.Id),
			Titulo:  f.Titulo,
			Cuerpo:  f.Cuerpo,
			Leida:   f.Leida,
			Destino: f.UrlDestino,
			Fecha:   f.CreadoEn,
		})
	}
	return out, nil
}

// Crear deja un aviso. Lo usa el propio middleware tras una postulacion, una
// contratacion o un pago.
func (s *Notificaciones) Crear(ctx context.Context, idUsuario int, contexto, titulo, cuerpo, destino string) error {
	idTipo, err := s.catalogo.IDTipo(ctx, "notificacion", contexto)
	if err != nil {
		idTipo, err = s.catalogo.IDTipo(ctx, "notificacion", "sistema")
		if err != nil {
			return err
		}
	}
	peticion := map[string]any{
		"IdUsuarios": idUsuario,
		"IdTipo":     idTipo,
		"Titulo":     titulo,
		"Cuerpo":     cuerpo,
		"Leida":      false,
		"UrlDestino": destino,
		"Activo":     true,
	}
	var creada map[string]any
	return s.usuarios.Post(ctx, "notificaciones", peticion, &creada)
}

// MarcarLeida marca un aviso propio como leido.
func (s *Notificaciones) MarcarLeida(ctx context.Context, idUsuario, idNotificacion string) error {
	usuario, err := IDNumerico(idUsuario)
	if err != nil {
		return err
	}
	id, err := IDNumerico(idNotificacion)
	if err != nil {
		return err
	}

	var filas []filaNotificacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.usuarios.Get(ctx, "notificaciones", o, &filas); err != nil {
		return err
	}
	if len(filas) == 0 {
		return models.NoEncontrado("esa notificacion")
	}
	f := filas[0]
	if f.IdUsuarios != usuario {
		return models.Prohibido("")
	}

	return s.usuarios.Fijar(ctx, "notificaciones/"+strconv.Itoa(id), map[string]any{"Leida": true})
}

// ---- Configuraciones ----

// Configuraciones son las preferencias de privacidad y avisos.
//
// La pantalla de configuracion del frontend guardaba todo en memoria: el
// boton de guardar solo encendia un indicador verde.
