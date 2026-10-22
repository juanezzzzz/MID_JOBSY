package services

import (
	"context"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// Soporte gestiona los reportes de los usuarios y la conversacion de cada
// reporte, tanto para el propio usuario como para los agentes (metodos admin).
type Soporte struct {
	soporte  *ClienteUpstream
	catalogo *Catalogo
}

// NuevoSoporte crea el servicio de soporte sobre su upstream y el catalogo
// (del que saca los tipos y estados de los reportes).
func NuevoSoporte(s *ClienteUpstream, cat *Catalogo) *Soporte {
	return &Soporte{soporte: s, catalogo: cat}
}

type filaReporte struct {
	Id          int    `json:"Id"`
	IdUsuarios  int    `json:"IdUsuarios"`
	IdTipo      int    `json:"IdTipo"`
	Descripcion string `json:"Descripcion"`
	ArchivoUrl  string `json:"ArchivoUrl"`
	IdEstado    int    `json:"IdEstado"`
	Activo      bool   `json:"Activo"`
	CreadoEn    string `json:"CreadoEn"`
}

// Mios devuelve los reportes del usuario autenticado.
//
// La tabla no tiene columna de asunto: el frontend lo trata como campo aparte
// y aqui se deriva de la primera linea de la descripcion.
func (s *Soporte) Mios(ctx context.Context, idUsuario string) ([]models.Reporte, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	var filas []filaReporte
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.soporte.Get(ctx, "reportes", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.Reporte, 0, len(filas))
	for _, f := range filas {
		out = append(out, s.aDominio(ctx, f))
	}
	return out, nil
}

// aDominio traduce una fila al Report del frontend.
//
// Antes el tipo salia siempre como "Incidencia", aunque se guardaba bien, y el
// estado llegaba con el nombre de la base, que el frontend no sabe pintar.
func (s *Soporte) aDominio(ctx context.Context, f filaReporte) models.Reporte {
	estado := "abierto"
	if n, err := s.catalogo.EstadoDesdeID(ctx, f.IdEstado); err == nil {
		estado = estadoReporteAFront(n)
	}
	tipo := "Incidencia"
	if n, err := s.catalogo.TipoDesdeID(ctx, "reporte", f.IdTipo); err == nil {
		tipo = tipoReporteAFront(n)
	}
	asunto, cuerpo := partirAsunto(f.Descripcion)
	return models.Reporte{
		ID:           strconv.Itoa(f.Id),
		Tipo:         tipo,
		Asunto:       asunto,
		Descripcion:  cuerpo,
		Estado:       estado,
		ReportanteID: strconv.Itoa(f.IdUsuarios),
		Fecha:        f.CreadoEn,
	}
}

// Los estados de reporte no coinciden entre capas:
//
//	base:  abierto, en_revision, respondido, cerrado
//	front: abierto, en_proceso,  resuelto,   cerrado
//
// La base describe 'cerrado' como "caso resuelto y cerrado", asi que los dos
// finales del frontend van a esa misma fila y al leer salen como 'resuelto'.
// 'respondido' (soporte ya contesto) es para el frontend un caso en proceso.
func estadoReporteAFront(base string) string {
	switch base {
	case "en_revision", "respondido":
		return "en_proceso"
	case "cerrado":
		return "resuelto"
	default:
		return "abierto"
	}
}

// estadoReporteABase traduce el estado que usa el front al nombre que espera
// la base; devuelve error si el estado no es valido.
func estadoReporteABase(front string) (string, error) {
	switch front {
	case "abierto":
		return "abierto", nil
	case "en_proceso":
		return "en_revision", nil
	case "resuelto", "cerrado":
		return "cerrado", nil
	default:
		return "", models.Validacion("Estado no valido: "+front).ConCampo("estado", "Usa abierto, en_proceso, resuelto o cerrado.")
	}
}

// tipoReporteAFront traduce el tipo de reporte guardado en la base al texto
// que muestra el front.
func tipoReporteAFront(base string) string {
	switch base {
	case "usuario":
		return "Usuario"
	case "pago":
		return "Pago"
	case "sugerencia":
		return "Sugerencia"
	default:
		return "Incidencia"
	}
}

type filaMensajeReporte struct {
	Id          int         `json:"Id"`
	IdReportes  *refAnidada `json:"IdReportes"`
	IdRemitente int         `json:"IdRemitente"`
	Mensaje     string      `json:"Mensaje"`
	ArchivoUrl  string      `json:"ArchivoUrl"`
	Activo      bool        `json:"Activo"`
	CreadoEn    string      `json:"CreadoEn"`
}

// Crear abre un reporte de soporte.
//
// El frontend guardaba los reportes en localStorage y simulaba la respuesta
// del equipo con un temporizador: soporte nunca veia nada.
func (s *Soporte) Crear(ctx context.Context, idUsuario, tipo, asunto, descripcion string) (*models.Reporte, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	asunto = strings.TrimSpace(asunto)
	descripcion = strings.TrimSpace(descripcion)
	if asunto == "" {
		return nil, models.Validacion("Escribe un asunto.").ConCampo("asunto", "Obligatorio.")
	}
	if descripcion == "" {
		return nil, models.Validacion("Describe lo que ocurre.").ConCampo("descripcion", "Obligatoria.")
	}
	if err := ValidarTexto("asunto", "El asunto", asunto, 3, 120); err != nil {
		return nil, err
	}
	if err := ValidarTexto("descripcion", "La descripcion", descripcion, 10, 2000); err != nil {
		return nil, err
	}

	idTipo, err := s.catalogo.IDTipo(ctx, "reporte", tipoReporteABase(tipo))
	if err != nil {
		return nil, err
	}
	idEstado, err := s.catalogo.IDEstado(ctx, "reporte", "abierto")
	if err != nil {
		return nil, err
	}

	// La tabla no tiene columna de asunto: se guarda como primera linea de la
	// descripcion, que es de donde se vuelve a extraer al leer.
	cuerpo := map[string]any{
		"IdUsuarios":  id,
		"IdTipo":      idTipo,
		"Descripcion": asunto + saltoLinea + descripcion,
		"ArchivoUrl":  "",
		"IdEstado":    idEstado,
		"Activo":      true,
	}
	var creado filaReporte
	if err := s.soporte.Post(ctx, "reportes", cuerpo, &creado); err != nil {
		return nil, err
	}

	return &models.Reporte{
		ID:           strconv.Itoa(creado.Id),
		Tipo:         tipo,
		Asunto:       asunto,
		Descripcion:  descripcion,
		Estado:       "abierto",
		ReportanteID: idUsuario,
	}, nil
}

// tipoReporteABase traduce el vocabulario del frontend al del catalogo.
// saltoLinea separa el asunto del cuerpo dentro de la columna descripcion,
// que es la unica que la tabla ofrece para ambos.
const saltoLinea = "\n"

// tipoReporteABase normaliza el tipo recibido del front (sin espacios ni
// mayusculas) al nombre que usa la base.
func tipoReporteABase(tipo string) string {
	switch strings.ToLower(strings.TrimSpace(tipo)) {
	case "usuario":
		return "usuario"
	case "pago":
		return "pago"
	case "sugerencia":
		return "sugerencia"
	default:
		return "tecnico"
	}
}

// Mensajes devuelve el hilo de un reporte propio.
func (s *Soporte) Mensajes(ctx context.Context, idUsuario, idReporte string) ([]models.MensajeReporte, error) {
	usuario, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	id, err := IDNumerico(idReporte)
	if err != nil {
		return nil, err
	}
	if _, err := s.exigirPropietario(ctx, usuario, id); err != nil {
		return nil, err
	}

	var filas []filaMensajeReporte
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdReportes": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "asc")
	if err := s.soporte.Get(ctx, "reporte_mensajes", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.MensajeReporte, 0, len(filas))
	for _, f := range filas {
		autor := "soporte"
		if f.IdRemitente == usuario {
			autor = "usuario"
		}
		out = append(out, models.MensajeReporte{
			ID:    strconv.Itoa(f.Id),
			Autor: autor,
			Texto: f.Mensaje,
			Fecha: f.CreadoEn,
			EsMio: f.IdRemitente == usuario,
		})
	}
	return out, nil
}

// Responder anade un mensaje al hilo.
func (s *Soporte) Responder(ctx context.Context, idUsuario, idReporte, texto string) (*models.MensajeReporte, error) {
	usuario, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	id, err := IDNumerico(idReporte)
	if err != nil {
		return nil, err
	}
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil, models.Validacion("Escribe un mensaje.").ConCampo("texto", "Obligatorio.")
	}
	if err := ValidarTexto("texto", "El mensaje", texto, 1, 2000); err != nil {
		return nil, err
	}
	rep, err := s.exigirPropietario(ctx, usuario, id)
	if err != nil {
		return nil, err
	}
	if s.estaCerrado(ctx, rep) {
		return nil, models.Conflicto("Este reporte esta cerrado. Abre uno nuevo si necesitas algo mas.")
	}

	cuerpo := map[string]any{
		"IdReportes":  map[string]any{"Id": id},
		"IdRemitente": usuario,
		"Mensaje":     texto,
		"ArchivoUrl":  "",
		"Activo":      true,
	}
	var creado filaMensajeReporte
	if err := s.soporte.Post(ctx, "reporte_mensajes", cuerpo, &creado); err != nil {
		return nil, err
	}

	// Si soporte ya habia contestado, la nueva pregunta lo devuelve a la cola.
	if n, err := s.catalogo.EstadoDesdeID(ctx, rep.IdEstado); err == nil && n == "respondido" {
		if err := s.fijarEstadoBase(ctx, rep, "en_revision"); err != nil {
			return nil, err
		}
	}

	return &models.MensajeReporte{
		ID:    strconv.Itoa(creado.Id),
		Autor: "usuario",
		Texto: texto,
		EsMio: true,
	}, nil
}

// exigirPropietario impide leer o escribir en reportes ajenos. El backend
// obedeceria cualquier id.
func (s *Soporte) exigirPropietario(ctx context.Context, usuario, idReporte int) (*filaReporte, error) {
	rep, err := s.reporte(ctx, idReporte)
	if err != nil {
		return nil, err
	}
	if rep.IdUsuarios != usuario {
		return nil, models.Prohibido("")
	}
	return rep, nil
}

// reporte lee un reporte activo.
func (s *Soporte) reporte(ctx context.Context, idReporte int) (*filaReporte, error) {
	var filas []filaReporte
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(idReporte)}))
	o.Query.Set("limit", "1")
	if err := s.soporte.Get(ctx, "reportes", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return nil, models.NoEncontrado("ese reporte")
	}
	return &filas[0], nil
}

// estaCerrado indica si el reporte esta en estado "cerrado" (para no permitir
// mas mensajes sobre el).
func (s *Soporte) estaCerrado(ctx context.Context, r *filaReporte) bool {
	n, err := s.catalogo.EstadoDesdeID(ctx, r.IdEstado)
	return err == nil && n == "cerrado"
}

// partirAsunto separa la primera linea como asunto.
func partirAsunto(desc string) (string, string) {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return "(sin asunto)", ""
	}
	if i := strings.IndexAny(desc, "\n."); i > 0 && i < 120 {
		return strings.TrimSpace(desc[:i]), strings.TrimSpace(desc[i+1:])
	}
	if len(desc) > 80 {
		return desc[:80], desc
	}
	return desc, desc
}

// ---- Panel de administracion ----

// Admin calcula los indicadores del panel.
//
// Ninguno existe en el backend: son agregados que aqui se computan leyendo las
// tablas. Con volumenes grandes habria que moverlos a vistas materializadas.

// Atencion de reportes desde el panel de administracion.
//
// El panel respondia y cambiaba estados sobre la copia en localStorage del
// navegador del administrador: el usuario nunca veia la respuesta, y la
// "respuesta de soporte" que si veia la inventaba un temporizador del
// frontend.

// Todos devuelve los reportes activos de todos los usuarios.
func (s *Soporte) Todos(ctx context.Context) ([]models.Reporte, error) {
	var filas []filaReporte
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
	o.Query.Set("limit", "500")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.soporte.Get(ctx, "reportes", o, &filas); err != nil {
		return nil, err
	}
	out := make([]models.Reporte, 0, len(filas))
	for _, f := range filas {
		out = append(out, s.aDominio(ctx, f))
	}
	return out, nil
}

// MensajesAdmin devuelve el hilo de cualquier reporte. El autor se decide
// contra el dueno del reporte: todo lo que no escribio el es de soporte.
func (s *Soporte) MensajesAdmin(ctx context.Context, idReporte string) ([]models.MensajeReporte, error) {
	id, err := IDNumerico(idReporte)
	if err != nil {
		return nil, err
	}
	rep, err := s.reporte(ctx, id)
	if err != nil {
		return nil, err
	}
	var filas []filaMensajeReporte
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdReportes": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "asc")
	if err := s.soporte.Get(ctx, "reporte_mensajes", o, &filas); err != nil {
		return nil, err
	}
	out := make([]models.MensajeReporte, 0, len(filas))
	for _, f := range filas {
		autor := "soporte"
		if f.IdRemitente == rep.IdUsuarios {
			autor = "usuario"
		}
		out = append(out, models.MensajeReporte{
			ID:    strconv.Itoa(f.Id),
			Autor: autor,
			Texto: f.Mensaje,
			Fecha: f.CreadoEn,
			EsMio: autor == "soporte",
		})
	}
	return out, nil
}

// ResponderComoSoporte anade la respuesta de un agente y deja el reporte como
// respondido si aun estaba abierto o en revision.
func (s *Soporte) ResponderComoSoporte(ctx context.Context, idAgente, idReporte, texto string) (*models.MensajeReporte, error) {
	agente, err := IDNumerico(idAgente)
	if err != nil {
		return nil, err
	}
	id, err := IDNumerico(idReporte)
	if err != nil {
		return nil, err
	}
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil, models.Validacion("Escribe una respuesta.").ConCampo("texto", "Obligatoria.")
	}
	if err := ValidarTexto("texto", "La respuesta", texto, 1, 2000); err != nil {
		return nil, err
	}
	rep, err := s.reporte(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.estaCerrado(ctx, rep) {
		return nil, models.Conflicto("El reporte esta cerrado. Reabrelo antes de responder.")
	}

	var creado filaMensajeReporte
	if err := s.soporte.Post(ctx, "reporte_mensajes", map[string]any{
		"IdReportes":  map[string]any{"Id": id},
		"IdRemitente": agente,
		"Mensaje":     texto,
		"ArchivoUrl":  "",
		"Activo":      true,
	}, &creado); err != nil {
		return nil, err
	}

	if err := s.fijarEstadoBase(ctx, rep, "respondido"); err != nil {
		return nil, err
	}
	return &models.MensajeReporte{
		ID:    strconv.Itoa(creado.Id),
		Autor: "soporte",
		Texto: texto,
		Fecha: time.Now().UTC().Format(time.RFC3339),
		EsMio: true,
	}, nil
}

// CambiarEstado aplica un estado del vocabulario del frontend.
func (s *Soporte) CambiarEstado(ctx context.Context, idReporte, estado string) (*models.Reporte, error) {
	id, err := IDNumerico(idReporte)
	if err != nil {
		return nil, err
	}
	base, err := estadoReporteABase(estado)
	if err != nil {
		return nil, err
	}
	rep, err := s.reporte(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.fijarEstadoBase(ctx, rep, base); err != nil {
		return nil, err
	}
	r := s.aDominio(ctx, *rep)
	return &r, nil
}

// Retirar oculta un reporte. No se borra: el hilo es el rastro de la
// atencion prestada.
func (s *Soporte) Retirar(ctx context.Context, idReporte string) error {
	id, err := IDNumerico(idReporte)
	if err != nil {
		return err
	}
	rep, err := s.reporte(ctx, id)
	if err != nil {
		return err
	}
	rep.Activo = false
	return s.guardar(ctx, rep)
}

// fijarEstadoBase cambia el estado del reporte al identificado por nombre
// (resolviendo su id en el catalogo) y lo guarda.
func (s *Soporte) fijarEstadoBase(ctx context.Context, rep *filaReporte, nombre string) error {
	idEstado, err := s.catalogo.IDEstado(ctx, "reporte", nombre)
	if err != nil {
		return err
	}
	if rep.IdEstado == idEstado {
		return nil
	}
	rep.IdEstado = idEstado
	return s.guardar(ctx, rep)
}

// guardar escribe el estado y la vigencia del reporte.
func (s *Soporte) guardar(ctx context.Context, rep *filaReporte) error {
	return s.soporte.Fijar(ctx, "reportes/"+strconv.Itoa(rep.Id), map[string]any{
		"IdEstado": rep.IdEstado,
		"Activo":   rep.Activo,
	})
}
