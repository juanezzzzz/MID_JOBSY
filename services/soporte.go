package services

import (
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
	params := url.Values{
		"query":  {Filtro(map[string]string{"IdReportes": strconv.Itoa(rep.Id), "Activo": "true"})},
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
			EsMio:   f.IdRemitente == uid,
		})
	}
	return mensajes, nil
}

// Responder agrega un mensaje del usuario a su propio reporte. Un reporte
// cerrado ya no acepta mensajes.
func (s *Soporte) Responder(idUsuario, idReporte, texto string) (*models.MensajeReporte, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil, models.Validacion("El mensaje esta vacio.").ConCampo("texto", "Obligatorio.")
	}
	if utf8.RuneCountInString(texto) > maxMensajeReporte {
		return nil, models.Validacion("El mensaje es demasiado largo.").
			ConCampo("texto", "Maximo "+strconv.Itoa(maxMensajeReporte)+" caracteres.")
	}

	uid, rep, err := s.reporteDe(idUsuario, idReporte)
	if err != nil {
		return nil, err
	}
	if estado, _ := EstadoDesdeID(rep.IdEstado); estado == "cerrado" {
		return nil, models.Conflicto("El reporte esta cerrado y ya no acepta mensajes.")
	}

	nuevo := map[string]any{
		"IdReportes":  map[string]any{"Id": rep.Id},
		"IdRemitente": uid,
		"Mensaje":     texto,
		"Activo":      true,
	}
	var creado filaMensaje
	if err := s.api.Post("reporte_mensajes", nuevo, &creado); err != nil {
		return nil, err
	}
	return &models.MensajeReporte{
		ID:      strconv.Itoa(creado.Id),
		AutorID: strconv.Itoa(uid),
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
	rid, err := idNumerico(idReporte)
	if err != nil {
		return 0, nil, err
	}

	var f filaReporte
	if err := s.api.Get("reportes/"+strconv.Itoa(rid), nil, &f); err != nil {
		return 0, nil, err
	}
	if f.Id == 0 || !f.Activo || f.IdUsuarios != uid {
		return 0, nil, models.NoEncontrado("ese reporte")
	}
	return uid, &f, nil
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
