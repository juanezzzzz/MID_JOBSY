package services

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"api_MID/models"
)

// Tipos y estados de reporte viven en el catalogo con contexto 'reporte'.
const contextoReporte = "reporte"

// Un reporte nuevo siempre arranca abierto; nadie lo cierra al crearlo.
const estadoInicialReporte = "abierto"

// Limites de texto, para que un cliente no meta un libro en un ticket.
const (
	maxDescripcionReporte = 2000
	maxMensajeReporte     = 1000
)

// filaReporte refleja el JSON de Soporte_API. Sus modelos no tienen tags json,
// asi que los campos llegan en PascalCase.
type filaReporte struct {
	Id          int
	IdUsuarios  int
	IdTipo      int
	Descripcion string
	ArchivoUrl  string
	IdEstado    int
	Activo      bool
	CreadoEn    string
}

// filaMensaje refleja reporte_mensajes. IdReportes es una relacion, y Beego
// la serializa como el objeto completo del reporte.
type filaMensaje struct {
	Id          int
	IdReportes  struct{ Id int }
	IdRemitente int
	Mensaje     string
	Activo      bool
	CreadoEn    string
}

// PeticionReporte es lo que manda el front para abrir un reporte.
type PeticionReporte struct {
	Tipo        string `json:"tipo"`
	Descripcion string `json:"descripcion"`
}

// Soporte maneja los reportes del propio usuario y sus mensajes.
type Soporte struct {
	api *ClienteAPI
}

// NuevoSoporte crea el servicio sobre el cliente de Soporte_API.
func NuevoSoporte(api *ClienteAPI) *Soporte {
	return &Soporte{api: api}
}

// MisReportes lista los reportes activos del usuario, del mas nuevo al mas viejo.
func (s *Soporte) MisReportes(idUsuario string) ([]models.Reporte, error) {
	uid, err := idNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	params := url.Values{
		"query":  {Filtro(map[string]string{"IdUsuarios": strconv.Itoa(uid), "Activo": "true"})},
		"sortby": {"CreadoEn"},
		"order":  {"desc"},
		// Beego trae 10 por defecto.
		"limit": {"100"},
	}
	var filas []filaReporte
	if err := s.api.Get("reportes", params, &filas); err != nil {
		return nil, err
	}

	reportes := make([]models.Reporte, 0, len(filas))
	for _, f := range filas {
		reportes = append(reportes, aReporte(f))
	}
	return reportes, nil
}

// CrearReporte abre un reporte a nombre del usuario, en estado abierto.
func (s *Soporte) CrearReporte(idUsuario string, p PeticionReporte) (*models.Reporte, error) {
	uid, err := idNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	p.Tipo = strings.TrimSpace(p.Tipo)
	p.Descripcion = strings.TrimSpace(p.Descripcion)
	if p.Tipo == "" {
		return nil, models.Validacion("Falta el tipo de reporte.").ConCampo("tipo", "Obligatorio.")
	}
	if p.Descripcion == "" {
		return nil, models.Validacion("Falta la descripcion.").ConCampo("descripcion", "Obligatoria.")
	}
	if utf8.RuneCountInString(p.Descripcion) > maxDescripcionReporte {
		return nil, models.Validacion("La descripcion es demasiado larga.").
			ConCampo("descripcion", "Maximo "+strconv.Itoa(maxDescripcionReporte)+" caracteres.")
	}

	idTipo, err := IDTipo(contextoReporte, p.Tipo)
	if err != nil {
		return nil, models.Validacion("Ese tipo de reporte no existe.").ConCampo("tipo", "Valor no valido.")
	}
	idEstado, err := IDEstado(contextoReporte, estadoInicialReporte)
	if err != nil {
		return nil, err
	}

	nueva := map[string]any{
		"IdUsuarios":  uid,
		"IdTipo":      idTipo,
		"Descripcion": p.Descripcion,
		"IdEstado":    idEstado,
		"Activo":      true,
	}
	var creada filaReporte
	if err := s.api.Post("reportes", nueva, &creada); err != nil {
		return nil, err
	}
	r := aReporte(creada)
	return &r, nil
}

// Mensajes devuelve el hilo de un reporte del usuario, del mas viejo al mas nuevo.
func (s *Soporte) Mensajes(idUsuario, idReporte string) ([]models.MensajeReporte, error) {
	uid, rep, err := s.reporteDe(idUsuario, idReporte)
	if err != nil {
		return nil, err
	}
	return s.hilo(rep.Id, func(f filaMensaje) bool { return f.IdRemitente == uid })
}

// hilo lee los mensajes de un reporte, del mas viejo al mas nuevo. esMio dice
// de que lado va cada burbuja segun quien este mirando.
func (s *Soporte) hilo(idReporte int, esMio func(filaMensaje) bool) ([]models.MensajeReporte, error) {
	params := url.Values{
		"query":  {Filtro(map[string]string{"IdReportes": strconv.Itoa(idReporte), "Activo": "true"})},
		"sortby": {"CreadoEn"},
		"order":  {"asc"},
		"limit":  {"500"},
	}
	var filas []filaMensaje
	if err := s.api.Get("reporte_mensajes", params, &filas); err != nil {
		return nil, err
	}

	mensajes := make([]models.MensajeReporte, 0, len(filas))
	for _, f := range filas {
		mensajes = append(mensajes, models.MensajeReporte{
			ID:      strconv.Itoa(f.Id),
			AutorID: strconv.Itoa(f.IdRemitente),
			Texto:   f.Mensaje,
			Fecha:   f.CreadoEn,
			EsMio:   esMio(f),
		})
	}
	return mensajes, nil
}

// Responder agrega un mensaje del usuario a su propio reporte. Un reporte
// cerrado ya no acepta mensajes.
func (s *Soporte) Responder(idUsuario, idReporte, texto string) (*models.MensajeReporte, error) {
	texto, err := textoMensaje(texto)
	if err != nil {
		return nil, err
	}
	uid, rep, err := s.reporteDe(idUsuario, idReporte)
	if err != nil {
		return nil, err
	}
	return s.publicar(rep, uid, texto)
}

// textoMensaje limpia y valida el texto de un mensaje.
func textoMensaje(texto string) (string, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return "", models.Validacion("El mensaje esta vacio.").ConCampo("texto", "Obligatorio.")
	}
	if utf8.RuneCountInString(texto) > maxMensajeReporte {
		return "", models.Validacion("El mensaje es demasiado largo.").
			ConCampo("texto", "Maximo "+strconv.Itoa(maxMensajeReporte)+" caracteres.")
	}
	return texto, nil
}

// publicar guarda un mensaje en el hilo. Un reporte cerrado ya no acepta
// mensajes, ni del usuario ni de soporte.
func (s *Soporte) publicar(rep *filaReporte, idAutor int, texto string) (*models.MensajeReporte, error) {
	if estado, _ := EstadoDesdeID(rep.IdEstado); estado == "cerrado" {
		return nil, models.Conflicto("El reporte esta cerrado y ya no acepta mensajes.")
	}

	nuevo := map[string]any{
		"IdReportes":  map[string]any{"Id": rep.Id},
		"IdRemitente": idAutor,
		"Mensaje":     texto,
		"Activo":      true,
	}
	var creado filaMensaje
	if err := s.api.Post("reporte_mensajes", nuevo, &creado); err != nil {
		return nil, err
	}
	return &models.MensajeReporte{
		ID:      strconv.Itoa(creado.Id),
		AutorID: strconv.Itoa(idAutor),
		Texto:   creado.Mensaje,
		Fecha:   creado.CreadoEn,
		EsMio:   true,
	}, nil
}

// reporteDe lee un reporte y comprueba que sea del usuario. Si es de otro se
// responde 404, no 403, para no confirmar que ese id existe.
func (s *Soporte) reporteDe(idUsuario, idReporte string) (int, *filaReporte, error) {
	uid, err := idNumerico(idUsuario)
	if err != nil {
		return 0, nil, err
	}
	f, err := s.reporteActivo(idReporte)
	if err != nil {
		return 0, nil, err
	}
	if f.IdUsuarios != uid {
		return 0, nil, models.NoEncontrado("ese reporte")
	}
	return uid, f, nil
}

// reporteActivo lee un reporte sin mirar de quien es. Lo usa la vista de
// soporte y, con la comprobacion de dueno, la del usuario.
func (s *Soporte) reporteActivo(idReporte string) (*filaReporte, error) {
	rid, err := idNumerico(idReporte)
	if err != nil {
		return nil, err
	}
	var f filaReporte
	if err := s.api.Get("reportes/"+strconv.Itoa(rid), nil, &f); err != nil {
		return nil, err
	}
	if f.Id == 0 || !f.Activo {
		return nil, models.NoEncontrado("ese reporte")
	}
	return &f, nil
}

// aReporte pasa una fila de Soporte_API a la forma del front. Si el catalogo
// no conoce el tipo o el estado se deja vacio: es mejor listar el reporte que
// esconderlo por un catalogo desactualizado.
func aReporte(f filaReporte) models.Reporte {
	tipo, _ := TipoDesdeID(f.IdTipo)
	estado, _ := EstadoDesdeID(f.IdEstado)
	return models.Reporte{
		ID:           strconv.Itoa(f.Id),
		Tipo:         tipo,
		Descripcion:  f.Descripcion,
		Estado:       estado,
		ReportanteID: strconv.Itoa(f.IdUsuarios),
		ArchivoURL:   f.ArchivoUrl,
		Fecha:        f.CreadoEn,
	}
}

// idNumerico convierte un id que llega como texto (asi lo tipa el front) al
// entero que usa la base.
func idNumerico(id string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(id))
	if err != nil || n <= 0 {
		return 0, models.Validacion("El id no es valido.")
	}
	return n, nil
}

// ---- Vista de soporte (AdminSoporteController) ----
//
// Reciben el ctx de la peticion como el resto de servicios del admin. Si el
// cliente ya se fue no se llama a Soporte_API.

// Todos lista los reportes activos de todos los usuarios, del mas nuevo al
// mas viejo.
func (s *Soporte) Todos(ctx context.Context) ([]models.Reporte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	params := url.Values{
		"query":  {Filtro(map[string]string{"Activo": "true"})},
		"sortby": {"CreadoEn"},
		"order":  {"desc"},
		"limit":  {"500"},
	}
	var filas []filaReporte
	if err := s.api.Get("reportes", params, &filas); err != nil {
		return nil, err
	}

	reportes := make([]models.Reporte, 0, len(filas))
	for _, f := range filas {
		reportes = append(reportes, aReporte(f))
	}
	return reportes, nil
}

// MensajesAdmin devuelve el hilo de cualquier reporte. Desde soporte, lo "mio"
// es todo lo que no escribio el usuario que abrio el reporte.
func (s *Soporte) MensajesAdmin(ctx context.Context, idReporte string) ([]models.MensajeReporte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rep, err := s.reporteActivo(idReporte)
	if err != nil {
		return nil, err
	}
	return s.hilo(rep.Id, func(f filaMensaje) bool { return f.IdRemitente != rep.IdUsuarios })
}

// ResponderComoSoporte agrega un mensaje al reporte a nombre del agente.
func (s *Soporte) ResponderComoSoporte(ctx context.Context, idAgente, idReporte, texto string) (*models.MensajeReporte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	texto, err := textoMensaje(texto)
	if err != nil {
		return nil, err
	}
	aid, err := idNumerico(idAgente)
	if err != nil {
		return nil, err
	}
	rep, err := s.reporteActivo(idReporte)
	if err != nil {
		return nil, err
	}
	return s.publicar(rep, aid, texto)
}

// CambiarEstado mueve el reporte a otro estado del contexto reporte
// (abierto, en_revision, cerrado...).
func (s *Soporte) CambiarEstado(ctx context.Context, idReporte, estado string) (*models.Reporte, error) {
	estado = strings.TrimSpace(estado)
	if estado == "" {
		return nil, models.Validacion("Falta el estado.").ConCampo("estado", "Obligatorio.")
	}
	idEstado, err := buscarID(ctx, "estados", contextoReporte, estado)
	if err != nil {
		return nil, models.Validacion("Ese estado de reporte no existe.").ConCampo("estado", "Valor no valido.")
	}
	rep, err := s.reporteActivo(idReporte)
	if err != nil {
		return nil, err
	}
	if err := s.api.Fijar("reportes", rep.Id, map[string]any{"IdEstado": idEstado}); err != nil {
		return nil, err
	}
	rep.IdEstado = idEstado
	r := aReporte(*rep)
	return &r, nil
}

// Retirar desactiva el reporte. La fila no se borra para no perder el hilo.
func (s *Soporte) Retirar(ctx context.Context, idReporte string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rep, err := s.reporteActivo(idReporte)
	if err != nil {
		return err
	}
	return s.api.Fijar("reportes", rep.Id, map[string]any{"Activo": false})
}
