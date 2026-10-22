package services

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// Postulaciones implementa el flujo de postularse a una oferta.
//
// El backend solo sabe insertar una fila en empleo.postulaciones. Todo lo que
// hace que eso sea "postularse" vive aqui: comprobar que la oferta existe y
// sigue abierta, impedir la postulacion duplicada, resolver la hoja de vida
// del candidato y fijar el estado inicial.
type Postulaciones struct {
	empleo   *ClienteUpstream
	catalogo *Catalogo
	avisos   *Notificaciones
}

// ConAvisos activa la notificacion al empleador cuando alguien se postula.
func (s *Postulaciones) ConAvisos(n *Notificaciones) *Postulaciones {
	s.avisos = n
	return s
}

// NuevoPostulaciones crea el servicio de postulaciones sobre Empleo_API y el
// catalogo.
func NuevoPostulaciones(empleo *ClienteUpstream, cat *Catalogo) *Postulaciones {
	return &Postulaciones{empleo: empleo, catalogo: cat}
}

type filaPostulacion struct {
	Id          int         `json:"Id"`
	IdOfertas   *refAnidada `json:"IdOfertas"`
	IdEmpleado  int         `json:"IdEmpleado"`
	IdHojasVida *refAnidada `json:"IdHojasVida"`
	IdEstado    int         `json:"IdEstado"`

	DisponibleDesde  string  `json:"DisponibleDesde"`
	TarifaMin        float64 `json:"TarifaMin"`
	TarifaMax        float64 `json:"TarifaMax"`
	MensajeEmpleador string  `json:"MensajeEmpleador"`
	Activo           bool    `json:"Activo"`
	PostuladoEn      string  `json:"PostuladoEn"`
}

// estadoDominio traduce el estado de la base al vocabulario del frontend.
//
// La base usa 'aceptado'/'rechazado'/'cancelado'; el front tipa
// 'aceptada'/'rechazada' y no modela 'cancelado'.
func estadoPostulacionDominio(nombre string) models.EstadoPostulacion {
	switch strings.ToLower(strings.TrimSpace(nombre)) {
	case "revisando", "en_revision":
		return models.PostEnRevision
	case "aceptado", "aceptada":
		return models.PostAceptada
	case "rechazado", "rechazada", "cancelado":
		return models.PostRechazada
	default:
		return models.PostPendiente
	}
}

// aDominio convierte la fila de la base en la postulacion que ve el front,
// resolviendo el estado por el catalogo.
func (s *Postulaciones) aDominio(ctx context.Context, f filaPostulacion) models.Postulacion {
	p := models.Postulacion{
		ID:               strconv.Itoa(f.Id),
		EmpleadoID:       strconv.Itoa(f.IdEmpleado),
		Estado:           models.PostPendiente,
		FechaPostulacion: f.PostuladoEn,
		Mensaje:          f.MensajeEmpleador,
		DisponibleDesde:  f.DisponibleDesde,
		TarifaMin:        f.TarifaMin,
		TarifaMax:        f.TarifaMax,
	}
	if f.IdOfertas != nil {
		p.OfertaID = strconv.Itoa(f.IdOfertas.Id)
	}
	if nombre, err := s.catalogo.EstadoDesdeID(ctx, f.IdEstado); err == nil {
		p.Estado = estadoPostulacionDominio(nombre)
	}
	return p
}

// Mias devuelve las postulaciones del usuario autenticado.
func (s *Postulaciones) Mias(ctx context.Context, idEmpleado string) ([]models.Postulacion, error) {
	id, err := IDNumerico(idEmpleado)
	if err != nil {
		return nil, err
	}

	var filas []filaPostulacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdEmpleado": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.empleo.Get(ctx, "postulaciones", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.Postulacion, 0, len(filas))
	for _, f := range filas {
		out = append(out, s.aDominio(ctx, f))
	}
	return out, nil
}

// PeticionPostular es lo que manda el asistente de postulacion del frontend.
type PeticionPostular struct {
	OfertaID   string   `json:"jobId"`
	Mensaje    string   `json:"mensaje"`
	Dias       []string `json:"dias,omitempty"`
	HoraInicio string   `json:"horaInicio,omitempty"`
	HoraFin    string   `json:"horaFin,omitempty"`
	Tarifa     float64  `json:"tarifa,omitempty"`
}

// Crear registra una postulacion.
func (s *Postulaciones) Crear(ctx context.Context, idEmpleado string, p PeticionPostular) (*models.Postulacion, error) {
	empleado, err := IDNumerico(idEmpleado)
	if err != nil {
		return nil, err
	}
	oferta, err := IDNumerico(p.OfertaID)
	if err != nil {
		return nil, models.Validacion("Identificador de oferta invalido.").ConCampo("jobId", "Invalido.")
	}

	// La oferta debe existir y seguir activa: el backend insertaria la fila
	// igual contra una oferta cerrada o inexistente.
	var ofertas []filaOferta
	oo := &OpcionesUpstream{Query: map[string][]string{}}
	oo.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(oferta)}))
	oo.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "ofertas", oo, &ofertas); err != nil {
		return nil, err
	}
	if len(ofertas) == 0 || !ofertas[0].Activo {
		return nil, models.NoEncontrado("esa oferta")
	}
	if ofertas[0].IdEmpleador == empleado {
		return nil, models.Validacion("No puedes postularte a tu propia oferta.")
	}
	// Activo solo dice que no esta borrada: una oferta pausada o cerrada por
	// su empleador tampoco admite postulaciones.
	if idActiva, err := s.catalogo.IDEstado(ctx, "oferta", "activa"); err == nil && ofertas[0].IdEstado != idActiva {
		return nil, models.Conflicto("Esa oferta ya no recibe postulaciones.")
	}

	// Una sola postulacion por persona y oferta.
	var previas []filaPostulacion
	op := &OpcionesUpstream{Query: map[string][]string{}}
	op.Query.Set("query", QueryBeego(map[string]string{
		"IdOfertas":  strconv.Itoa(oferta),
		"IdEmpleado": strconv.Itoa(empleado),
		"Activo":     "true",
	}))
	op.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "postulaciones", op, &previas); err == nil && len(previas) > 0 {
		return nil, models.Conflicto("Ya te postulaste a esta oferta.")
	}

	idHV, err := s.hojaVidaDe(ctx, empleado)
	if err != nil {
		return nil, err
	}

	idEstado, err := s.catalogo.IDEstado(ctx, "postulacion", "pendiente")
	if err != nil {
		return nil, err
	}

	cuerpo := map[string]any{
		"IdOfertas":          map[string]any{"Id": oferta},
		"IdEmpleado":         empleado,
		"IdHojasVida":        map[string]any{"Id": idHV},
		"IdEstado":           idEstado,
		"DisponibleDesde":    time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"IdModalidadDisp":    ofertas[0].IdModalidadTrabajo,
		"IdTurnoPostulacion": 1,
		"NotaDisponibilidad": notaDisponibilidad(p),
		"IdFormaCobro":       1,
		"TarifaMin":          p.Tarifa,
		"TarifaMax":          p.Tarifa,
		"MensajeEmpleador":   strings.TrimSpace(p.Mensaje),
		"AceptaTerminos":     true,
		"Activo":             true,
		"OrigenPostulacion":  false,
	}

	var creada filaPostulacion
	if err := s.empleo.Post(ctx, "postulaciones", cuerpo, &creada); err != nil {
		return nil, err
	}

	res := s.aDominio(ctx, creada)
	// El alta no siempre devuelve la fila completa; se completa lo que se sabe.
	if res.OfertaID == "" {
		res.OfertaID = strconv.Itoa(oferta)
	}
	if res.EmpleadoID == "0" || res.EmpleadoID == "" {
		res.EmpleadoID = strconv.Itoa(empleado)
	}
	res.Mensaje = strings.TrimSpace(p.Mensaje)

	// El empleador se entera por su bandeja de avisos. Si el aviso falla la
	// postulacion ya quedo hecha: solo se registra.
	if s.avisos != nil {
		if err := s.avisos.Crear(ctx, ofertas[0].IdEmpleador, "postulacion", "Nueva postulacion",
			"Alguien se postulo a tu oferta «"+ofertas[0].Titulo+"».", "/app/mis-ofertas"); err != nil {
			slog.Warn("no se pudo avisar al empleador", "oferta", oferta, "error", err)
		}
	}
	return &res, nil
}

// notaDisponibilidad conserva los datos del asistente que la tabla no tiene en
// columnas propias. El formulario los pide y hoy el frontend los descarta.
func notaDisponibilidad(p PeticionPostular) string {
	var partes []string
	if len(p.Dias) > 0 {
		partes = append(partes, "Dias: "+strings.Join(p.Dias, ", "))
	}
	if p.HoraInicio != "" || p.HoraFin != "" {
		partes = append(partes, "Horario: "+p.HoraInicio+" a "+p.HoraFin)
	}
	return strings.Join(partes, ". ")
}

// hojaVidaDe devuelve la hoja de vida del candidato, creando una vacia si aun
// no tiene: la postulacion la exige como clave foranea obligatoria.
func (s *Postulaciones) hojaVidaDe(ctx context.Context, idUsuario int) (int, error) {
	// Mismo criterio que HojasVida.obtenerOCrear: sin orden explicito el
	// backend podria devolver una hoja distinta en cada llamada y se acabarian
	// creando duplicados.
	var hvs []filaHojaVida
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	o.Query.Set("limit", "50")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "asc")
	if err := s.empleo.Get(ctx, "hojas_vida", o, &hvs); err != nil {
		return 0, err
	}
	if elegida := elegirHoja(hvs); elegida != nil {
		return elegida.Id, nil
	}

	cuerpo := map[string]any{
		"IdUsuarios":       idUsuario,
		"Nombre":           "Hoja de vida",
		"Resumen":          "",
		"EsPredeterminada": true,
		"UrlPdf":           "",
		"EsPublica":        false,
		"ProgresoPct":      0,
		"Activo":           true,
	}
	var creada filaHojaVida
	if err := s.empleo.Post(ctx, "hojas_vida", cuerpo, &creada); err != nil {
		return 0, err
	}
	if creada.Id == 0 {
		return 0, models.Upstream("empleo", errPendienteHV)
	}
	return creada.Id, nil
}

// Retirar da de baja una postulacion propia.
//
// Se marca como inactiva en vez de borrarla: el Delete del backend hace un
// DELETE real y las reseñas cuelgan de la postulacion por clave foranea.
func (s *Postulaciones) Retirar(ctx context.Context, idEmpleado, idPostulacion string) error {
	empleado, err := IDNumerico(idEmpleado)
	if err != nil {
		return err
	}
	id, err := IDNumerico(idPostulacion)
	if err != nil {
		return err
	}

	var filas []filaPostulacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "postulaciones", o, &filas); err != nil {
		return err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return models.NoEncontrado("esa postulacion")
	}
	// Autorizacion por fila: el backend obedeceria cualquier id.
	if filas[0].IdEmpleado != empleado {
		return models.Prohibido("")
	}

	return s.empleo.Fijar(ctx, "postulaciones/"+strconv.Itoa(id), map[string]any{"Activo": false})
}

type erroresHV string

// Error implementa la interfaz error.
func (e erroresHV) Error() string { return string(e) }

const errPendienteHV = erroresHV("el alta de la hoja de vida no devolvio id")
