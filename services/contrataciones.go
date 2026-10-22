package services

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"api_MID/models"
)

// Contrataciones implementa el paso de una postulacion aceptada a un servicio
// contratado, con su agenda.
//
// El frontend tiene las pantallas de contratacion y de agendar, pero su boton
// de confirmar solo cambiaba una bandera local: no existia servicio ni
// endpoint detras. Las tablas empleo.contrataciones y empleo.horarios_agendados
// llevaban vacias desde el principio.
type Contrataciones struct {
	empleo   *ClienteUpstream
	catalogo *Catalogo
	pagos    *Pagos
	avisos   *Notificaciones
	core     *ClienteUpstream
}

// ConAvisos activa la notificacion al empleado cuando lo contratan.
func (s *Contrataciones) ConAvisos(n *Notificaciones) *Contrataciones {
	s.avisos = n
	return s
}

// ConUsuarios permite completar las contrataciones con el nombre de la otra
// parte, que vive en Core_api.
func (s *Contrataciones) ConUsuarios(core *ClienteUpstream) *Contrataciones {
	s.core = core
	return s
}

// completar rellena el titulo de la oferta y el nombre de la contraparte. Lo
// que no se pueda leer se deja vacio: son datos de presentacion.
func (s *Contrataciones) completar(ctx context.Context, cs []models.Contratacion) {
	titulos := map[string]string{}
	nombres := map[string]string{}
	for i := range cs {
		cs[i].ContraparteID = cs[i].EmpleadoID
		if !cs[i].SoyEmpleador {
			cs[i].ContraparteID = cs[i].EmpleadorID
		}
		titulos[cs[i].OfertaID] = ""
		nombres[cs[i].ContraparteID] = ""
	}
	var mu sync.Mutex
	var tareas []func() error
	for id := range titulos {
		id := id
		tareas = append(tareas, func() error {
			var filas []filaOferta
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("query", QueryBeego(map[string]string{"Id": id}))
			o.Query.Set("limit", "1")
			if err := s.empleo.Get(ctx, "ofertas", o, &filas); err == nil && len(filas) > 0 {
				mu.Lock()
				titulos[id] = filas[0].Titulo
				mu.Unlock()
			}
			return nil
		})
	}
	if s.core != nil {
		for id := range nombres {
			id := id
			tareas = append(tareas, func() error {
				var filas []filaUsuario
				o := &OpcionesUpstream{Query: map[string][]string{}}
				o.Query.Set("query", QueryBeego(map[string]string{"Id": id}))
				o.Query.Set("limit", "1")
				if err := s.core.Get(ctx, "usuarios", o, &filas); err == nil && len(filas) > 0 {
					mu.Lock()
					nombres[id] = strings.TrimSpace(filas[0].Nombre + " " + filas[0].Apellido)
					mu.Unlock()
				}
				return nil
			})
		}
	}
	_ = EnParalelo(tareas...)
	for i := range cs {
		cs[i].Titulo = titulos[cs[i].OfertaID]
		cs[i].Contraparte = nombres[cs[i].ContraparteID]
	}
}

// validarServicio aplica las reglas comunes a toda contratacion: direccion
// real, fecha desde hoy, hora HH:MM, entre 1 y 12 horas y una tarifa
// razonable. La fecha vacia toma hoy, y la hora de fin es opcional.
func validarServicio(direccion, fecha, hora, horaFin string, horas, tarifa float64, descripcion, nota string) error {
	errs := []error{
		ValidarDireccion("direccion", direccion),
		ValidarHora("horaInicio", hora),
		ValidarRango("duracionHoras", "La duracion", horas, HorasMin, HorasMax),
		ValidarTarifa("tarifa", tarifa),
		ValidarTexto("descripcion", "La descripcion", descripcion, 0, 500),
		ValidarTexto("nota", "La nota", nota, 0, 500),
	}
	if strings.TrimSpace(fecha) != "" {
		errs = append(errs, ValidarFechaServicio("fechaInicio", fecha))
	}
	if strings.TrimSpace(horaFin) != "" {
		errs = append(errs, ValidarHora("horaFin", horaFin))
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(horaFin) != "" && minutos(horaFin) <= minutos(hora) {
		return invalido("horaFin", "La hora de fin debe ser posterior a la de inicio.")
	}
	return nil
}

// cubrirOferta cierra la oferta al contratar a alguien desde ella y responde a
// las demas postulaciones que seguian abiertas: una oferta es un puesto, y
// dejarlas "pendientes" para siempre engana a quien se postulo. Si el
// empleador necesita a otra persona puede reactivar la oferta. Los fallos solo
// se registran: la contratacion ya quedo hecha.
func (s *Contrataciones) cubrirOferta(ctx context.Context, of filaOferta, aceptada int) {
	if idCerrada, err := s.catalogo.IDEstado(ctx, "oferta", "cerrada"); err == nil {
		if err := s.empleo.Fijar(ctx, "ofertas/"+strconv.Itoa(of.Id), map[string]any{"IdEstado": idCerrada}); err != nil {
			slog.Warn("no se pudo cerrar la oferta cubierta", "oferta", of.Id, "error", err)
		}
	}
	idRechazo, err := s.catalogo.IDEstado(ctx, "postulacion", "rechazado")
	if err != nil {
		return
	}
	var posts []filaPostulacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdOfertas": strconv.Itoa(of.Id), "Activo": "true"}))
	o.Query.Set("limit", "200")
	if err := s.empleo.Get(ctx, "postulaciones", o, &posts); err != nil {
		slog.Warn("no se pudieron leer las demas postulaciones", "oferta", of.Id, "error", err)
		return
	}
	for _, p := range posts {
		if p.Id == aceptada {
			continue
		}
		if nombre, err := s.catalogo.EstadoDesdeID(ctx, p.IdEstado); err != nil || (nombre != "pendiente" && nombre != "revisando") {
			continue
		}
		if err := s.empleo.Fijar(ctx, "postulaciones/"+strconv.Itoa(p.Id), map[string]any{"IdEstado": idRechazo}); err != nil {
			slog.Warn("no se pudo cerrar la postulacion", "postulacion", p.Id, "error", err)
			continue
		}
		if s.avisos != nil {
			if err := s.avisos.Crear(ctx, p.IdEmpleado, "postulacion", "La oferta ya fue cubierta",
				"La oferta «"+of.Titulo+"» ya fue cubierta. Gracias por postularte.", "/app/mis-postulaciones"); err != nil {
				slog.Warn("no se pudo avisar a la candidata", "postulacion", p.Id, "error", err)
			}
		}
	}
}

// avisarContratado deja el aviso al empleado; si falla solo se registra,
// porque la contratacion ya quedo hecha.
func (s *Contrataciones) avisarContratado(ctx context.Context, idEmpleado int, titulo string) {
	if s.avisos == nil {
		return
	}
	if err := s.avisos.Crear(ctx, idEmpleado, "postulacion", "Te contrataron",
		"Te contrataron para «"+titulo+"». Revisa los detalles en tus contrataciones.", "/app/contrataciones"); err != nil {
		slog.Warn("no se pudo avisar al empleado", "empleado", idEmpleado, "error", err)
	}
}

// NuevoContrataciones crea el servicio de contrataciones sobre Empleo_API, el
// catalogo y el servicio de pagos.
func NuevoContrataciones(empleo *ClienteUpstream, cat *Catalogo, pagos *Pagos) *Contrataciones {
	return &Contrataciones{empleo: empleo, catalogo: cat, pagos: pagos}
}

type filaContratacion struct {
	Id                 int         `json:"Id"`
	IdEmpleador        int         `json:"IdEmpleador"`
	IdEmpleado         int         `json:"IdEmpleado"`
	IdOfertas          *refAnidada `json:"IdOfertas"`
	IdPostulaciones    *refAnidada `json:"IdPostulaciones"`
	IdEstado           int         `json:"IdEstado"`
	DireccionServicio  string      `json:"DireccionServicio"`
	FechaInicio        string      `json:"FechaInicio"`
	HoraInicio         string      `json:"HoraInicio"`
	DuracionHoras      float64     `json:"DuracionHoras"`
	IdModalidad        int         `json:"IdModalidad"`
	TarifaAcordada     float64     `json:"TarifaAcordada"`
	TotalEstimado      float64     `json:"TotalEstimado"`
	DescripcionTrabajo string      `json:"DescripcionTrabajo"`
	NotaAdicional      string      `json:"NotaAdicional"`
	Activo             bool        `json:"Activo"`
	CreadoEn           string      `json:"CreadoEn"`
}

type filaHorario struct {
	Id               int         `json:"Id"`
	IdContrataciones *refAnidada `json:"IdContrataciones"`
	IdDiaSemana      int         `json:"IdDiaSemana"`
	IdTurno          int         `json:"IdTurno"`
	HoraInicio       string      `json:"HoraInicio"`
	HoraFin          string      `json:"HoraFin"`
	Nota             string      `json:"Nota"`
	Activo           bool        `json:"Activo"`
}

// PeticionContratar es lo que manda la pantalla de contratacion.
type PeticionContratar struct {
	PostulacionID string  `json:"postulacionId"`
	Direccion     string  `json:"direccion"`
	FechaInicio   string  `json:"fechaInicio"`
	HoraInicio    string  `json:"horaInicio"`
	DuracionHoras float64 `json:"duracionHoras"`
	Tarifa        float64 `json:"tarifa"`
	Descripcion   string  `json:"descripcion,omitempty"`
	Nota          string  `json:"nota,omitempty"`
	// Dias son los codigos de dia de la semana ("lun", "mar"...) del horario
	// recurrente. Opcional: un servicio puntual no lo necesita.
	Dias    []string `json:"dias,omitempty"`
	HoraFin string   `json:"horaFin,omitempty"`
}

// aDominio convierte la fila de la base en la contratacion que ve el front,
// resolviendo el estado por el catalogo.
func (s *Contrataciones) aDominio(ctx context.Context, f filaContratacion) models.Contratacion {
	c := models.Contratacion{
		ID:            strconv.Itoa(f.Id),
		EmpleadorID:   strconv.Itoa(f.IdEmpleador),
		EmpleadoID:    strconv.Itoa(f.IdEmpleado),
		Direccion:     f.DireccionServicio,
		FechaInicio:   f.FechaInicio,
		HoraInicio:    f.HoraInicio,
		DuracionHoras: f.DuracionHoras,
		Tarifa:        f.TarifaAcordada,
		TotalEstimado: f.TotalEstimado,
		Descripcion:   f.DescripcionTrabajo,
		Nota:          f.NotaAdicional,
		Estado:        "activa",
		Fecha:         f.CreadoEn,
	}
	if f.IdOfertas != nil {
		c.OfertaID = strconv.Itoa(f.IdOfertas.Id)
	}
	if f.IdPostulaciones != nil {
		c.PostulacionID = strconv.Itoa(f.IdPostulaciones.Id)
	}
	if n, err := s.catalogo.EstadoDesdeID(ctx, f.IdEstado); err == nil {
		c.Estado = n
	}
	return c
}

// Crear convierte una postulacion en contratacion.
//
// Solo el empleador dueno de la oferta puede contratar, y solo sobre una
// postulacion viva de esa oferta. El backend aceptaria cualquier combinacion
// de ids.
func (s *Contrataciones) Crear(ctx context.Context, idEmpleador string, p PeticionContratar) (*models.Contratacion, error) {
	empleador, err := IDNumerico(idEmpleador)
	if err != nil {
		return nil, err
	}
	idPost, err := IDNumerico(p.PostulacionID)
	if err != nil {
		return nil, models.Validacion("Identificador de postulacion invalido.").
			ConCampo("postulacionId", "Invalido.")
	}
	if err := validarServicio(p.Direccion, p.FechaInicio, p.HoraInicio, p.HoraFin, p.DuracionHoras, p.Tarifa, p.Descripcion, p.Nota); err != nil {
		return nil, err
	}

	var posts []filaPostulacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(idPost)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "postulaciones", o, &posts); err != nil {
		return nil, err
	}
	if len(posts) == 0 || !posts[0].Activo {
		return nil, models.NoEncontrado("esa postulacion")
	}
	post := posts[0]
	if post.IdOfertas == nil {
		return nil, models.Upstream("empleo", errPostSinOferta)
	}

	var ofertas []filaOferta
	oo := &OpcionesUpstream{Query: map[string][]string{}}
	oo.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(post.IdOfertas.Id)}))
	oo.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "ofertas", oo, &ofertas); err != nil {
		return nil, err
	}
	if len(ofertas) == 0 {
		return nil, models.NoEncontrado("esa oferta")
	}
	// Autorizacion por fila: sin esto cualquiera contrataria sobre ofertas
	// ajenas.
	if ofertas[0].IdEmpleador != empleador {
		return nil, models.Prohibido("Solo puedes contratar sobre tus propias ofertas.")
	}

	// Una postulacion se contrata una sola vez.
	var previas []filaContratacion
	op := &OpcionesUpstream{Query: map[string][]string{}}
	op.Query.Set("query", QueryBeego(map[string]string{
		"IdPostulaciones": strconv.Itoa(idPost),
		"Activo":          "true",
	}))
	op.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "contrataciones", op, &previas); err == nil && len(previas) > 0 {
		return nil, models.Conflicto("Esa postulacion ya tiene una contratacion.")
	}

	idEstado, err := s.catalogo.IDEstado(ctx, "contratacion", "activa")
	if err != nil {
		return nil, err
	}

	fecha := strings.TrimSpace(p.FechaInicio)
	if fecha == "" {
		fecha = time.Now().UTC().Format("2006-01-02")
	}
	total := p.Tarifa * p.DuracionHoras

	cuerpo := map[string]any{
		"IdEmpleador":        empleador,
		"IdEmpleado":         post.IdEmpleado,
		"IdOfertas":          map[string]any{"Id": post.IdOfertas.Id},
		"IdPostulaciones":    map[string]any{"Id": idPost},
		"IdEstado":           idEstado,
		"DireccionServicio":  strings.TrimSpace(p.Direccion),
		"FechaInicio":        fecha + "T00:00:00Z",
		"HoraInicio":         strings.TrimSpace(p.HoraInicio),
		"DuracionHoras":      p.DuracionHoras,
		"IdModalidad":        ofertas[0].IdModalidadTrabajo,
		"TarifaAcordada":     p.Tarifa,
		"TotalEstimado":      total,
		"DescripcionTrabajo": strings.TrimSpace(p.Descripcion),
		"NotaAdicional":      strings.TrimSpace(p.Nota),
		"Activo":             true,
	}

	var creada filaContratacion
	if err := s.empleo.Post(ctx, "contrataciones", cuerpo, &creada); err != nil {
		return nil, err
	}
	if creada.Id == 0 {
		return nil, models.Upstream("empleo", errContratacionSinID)
	}

	// La agenda recurrente es opcional; su fallo no invalida la contratacion.
	if len(p.Dias) > 0 {
		_ = s.agendar(ctx, creada.Id, p)
	}

	// La postulacion pasa a aceptada: dejarla pendiente despues de contratar
	// seria mentir en la pantalla del candidato.
	_ = s.marcarPostulacionAceptada(ctx, post)
	s.avisarContratado(ctx, post.IdEmpleado, ofertas[0].Titulo)
	s.cubrirOferta(ctx, ofertas[0], post.Id)

	res := s.aDominio(ctx, creada)
	if res.EmpleadoID == "0" {
		res.EmpleadoID = strconv.Itoa(post.IdEmpleado)
	}
	res.TotalEstimado = total
	return &res, nil
}

// agendar crea las franjas recurrentes de la contratacion.
func (s *Contrataciones) agendar(ctx context.Context, idContratacion int, p PeticionContratar) error {
	for _, dia := range p.Dias {
		idDia, err := s.catalogo.IDDiaSemana(ctx, dia)
		if err != nil {
			continue
		}
		cuerpo := map[string]any{
			"IdContrataciones": map[string]any{"Id": idContratacion},
			"IdDiaSemana":      idDia,
			"IdTurno":          1,
			"HoraInicio":       p.HoraInicio,
			"HoraFin":          p.HoraFin,
			"Nota":             "",
			"Activo":           true,
		}
		var creado map[string]any
		_ = s.empleo.Post(ctx, "horarios_agendados", cuerpo, &creado)
	}
	return nil
}

// marcarPostulacionAceptada pasa la postulacion al estado "aceptado".
func (s *Contrataciones) marcarPostulacionAceptada(ctx context.Context, f filaPostulacion) error {
	idEstado, err := s.catalogo.IDEstado(ctx, "postulacion", "aceptado")
	if err != nil {
		return err
	}
	return s.empleo.Fijar(ctx, "postulaciones/"+strconv.Itoa(f.Id), map[string]any{"IdEstado": idEstado})
}

// Mias devuelve las contrataciones del usuario, sea como empleador o empleado.
func (s *Contrataciones) Mias(ctx context.Context, idUsuario string) ([]models.Contratacion, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	// El CRUD solo admite igualdad, no OR: hacen falta dos lecturas. Van en
	// paralelo porque son independientes.
	var comoEmpleador, comoEmpleado []filaContratacion
	err = EnParalelo(
		func() error {
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("query", QueryBeego(map[string]string{
				"IdEmpleador": strconv.Itoa(id), "Activo": "true",
			}))
			o.Query.Set("limit", "200")
			return s.empleo.Get(ctx, "contrataciones", o, &comoEmpleador)
		},
		func() error {
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("query", QueryBeego(map[string]string{
				"IdEmpleado": strconv.Itoa(id), "Activo": "true",
			}))
			o.Query.Set("limit", "200")
			return s.empleo.Get(ctx, "contrataciones", o, &comoEmpleado)
		},
	)
	if err != nil {
		return nil, err
	}

	vistas := map[int]bool{}
	out := make([]models.Contratacion, 0, len(comoEmpleador)+len(comoEmpleado))
	for _, grupo := range [][]filaContratacion{comoEmpleador, comoEmpleado} {
		for _, f := range grupo {
			if vistas[f.Id] {
				continue
			}
			vistas[f.Id] = true
			c := s.aDominio(ctx, f)
			c.SoyEmpleador = f.IdEmpleador == id
			out = append(out, c)
		}
	}
	s.completar(ctx, out)
	// La mas reciente primero.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Fecha > out[j].Fecha })
	return out, nil
}

// PorID devuelve una contratacion con su agenda, solo a quien participa en ella.
func (s *Contrataciones) PorID(ctx context.Context, idUsuario, idContratacion string) (*models.Contratacion, error) {
	usuario, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	id, err := IDNumerico(idContratacion)
	if err != nil {
		return nil, err
	}

	var filas []filaContratacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "contrataciones", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return nil, models.NoEncontrado("esa contratacion")
	}
	f := filas[0]
	if f.IdEmpleador != usuario && f.IdEmpleado != usuario {
		return nil, models.Prohibido("")
	}

	c := s.aDominio(ctx, f)
	c.SoyEmpleador = f.IdEmpleador == usuario
	c.Agenda = s.agendaDe(ctx, id)
	una := []models.Contratacion{c}
	s.completar(ctx, una)
	return &una[0], nil
}

// agendaDe devuelve las franjas activas agendadas para una contratacion. Si la
// lectura falla devuelve una agenda vacia en vez de error.
func (s *Contrataciones) agendaDe(ctx context.Context, idContratacion int) []models.FranjaAgenda {
	var filas []filaHorario
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdContrataciones": strconv.Itoa(idContratacion),
		"Activo":           "true",
	}))
	o.Query.Set("limit", "50")
	if err := s.empleo.Get(ctx, "horarios_agendados", o, &filas); err != nil {
		return []models.FranjaAgenda{}
	}

	out := make([]models.FranjaAgenda, 0, len(filas))
	for _, f := range filas {
		dia, _ := s.catalogo.DiaSemanaDesdeID(ctx, f.IdDiaSemana)
		out = append(out, models.FranjaAgenda{
			Dia:        dia,
			HoraInicio: f.HoraInicio,
			HoraFin:    f.HoraFin,
			Nota:       f.Nota,
		})
	}
	return out
}

// Finalizar cierra una contratacion. Solo el empleador puede darla por hecha.
func (s *Contrataciones) Finalizar(ctx context.Context, idUsuario, idContratacion string) (*models.Contratacion, error) {
	usuario, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	id, err := IDNumerico(idContratacion)
	if err != nil {
		return nil, err
	}

	var filas []filaContratacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "contrataciones", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return nil, models.NoEncontrado("esa contratacion")
	}
	f := filas[0]
	if f.IdEmpleador != usuario {
		return nil, models.Prohibido("Solo el empleador puede dar por finalizado el servicio.")
	}

	idEstado, err := s.catalogo.IDEstado(ctx, "contratacion", "completada")
	if err != nil {
		return nil, err
	}

	if err := s.empleo.Fijar(ctx, "contrataciones/"+strconv.Itoa(id), map[string]any{
		"IdEstado":     idEstado,
		"FinalizadoEn": time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return nil, err
	}

	// El servicio completado cuenta para el historial del empleado. La
	// columna total_trabajos existe en perfiles_empleado y nadie la
	// mantenia, asi que el perfil mostraba siempre cero servicios por muchos
	// trabajos que hubiera hecho.
	_ = s.sumarTrabajoCompletado(ctx, f.IdEmpleado)

	f.IdEstado = idEstado
	c := s.aDominio(ctx, f)
	c.SoyEmpleador = true
	return &c, nil
}

// sumarTrabajoCompletado incrementa el contador de servicios del empleado.
func (s *Contrataciones) sumarTrabajoCompletado(ctx context.Context, idEmpleado int) error {
	var perfiles []filaPerfil
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idEmpleado)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "perfiles_empleado", o, &perfiles); err != nil || len(perfiles) == 0 {
		return err
	}
	p := perfiles[0]

	_, err := s.empleo.Parchear(ctx, "perfiles_empleado/"+strconv.Itoa(p.Id), func(f map[string]any) error {
		total, _ := f["TotalTrabajos"].(float64)
		f["TotalTrabajos"] = int(total) + 1
		return nil
	})
	return err
}

type erroresContratacion string

// Error implementa la interfaz error.
func (e erroresContratacion) Error() string { return string(e) }

const (
	errPostSinOferta     = erroresContratacion("la postulacion no apunta a ninguna oferta")
	errContratacionSinID = erroresContratacion("el alta de la contratacion no devolvio id")
)

// Contratacion directa: el empleador elige a alguien del buscador y lo
// contrata sin que haya publicado una oferta ni el otro se haya postulado.
//
// El frontend tiene esa pantalla desde el principio (buscar empleada ->
// contratar), pero su boton de confirmar solo encendia una bandera local.
//
// empleo.contrataciones exige oferta y postulacion (ambas NOT NULL), asi que
// una contratacion directa no puede saltarselas. Lo que hace este flujo es
// crearlas: una oferta dirigida a esa persona y su postulacion ya aceptada,
// marcada con origen_postulacion = true, que es justo lo que esa columna
// distingue segun la DDL ("true=empleador | false=empleado").

// PeticionContratarDirecto es el cuerpo de POST /empleo/contrataciones/directa.
type PeticionContratarDirecto struct {
	EmpleadoID    string           `json:"empleadoId"`
	Titulo        string           `json:"titulo,omitempty"`
	Direccion     string           `json:"direccion"`
	Ciudad        string           `json:"ciudad,omitempty"`
	FechaInicio   string           `json:"fechaInicio"`
	HoraInicio    string           `json:"horaInicio"`
	DuracionHoras float64          `json:"duracionHoras"`
	Tarifa        float64          `json:"tarifa"`
	Modalidad     models.Modalidad `json:"modalidad,omitempty"`
	Descripcion   string           `json:"descripcion,omitempty"`
	Nota          string           `json:"nota,omitempty"`
	Dias          []string         `json:"dias,omitempty"`
	HoraFin       string           `json:"horaFin,omitempty"`
}

// CrearDirecta contrata a un empleado sin oferta previa.
//
// Son tres altas encadenadas en un backend sin transacciones. Si una falla se
// deshacen las anteriores; si la compensacion tambien falla se registra, que
// es cuanto se puede hacer sin transacciones.
func (s *Contrataciones) CrearDirecta(ctx context.Context, idEmpleador string, p PeticionContratarDirecto) (*models.Contratacion, error) {
	empleador, err := IDNumerico(idEmpleador)
	if err != nil {
		return nil, err
	}
	empleado, err := IDNumerico(p.EmpleadoID)
	if err != nil {
		return nil, models.Validacion("Identificador de empleado invalido.").
			ConCampo("empleadoId", "Invalido.")
	}
	if empleado == empleador {
		return nil, models.Validacion("No puedes contratarte a ti mismo.").
			ConCampo("empleadoId", "Invalido.")
	}
	if err := validarServicio(p.Direccion, p.FechaInicio, p.HoraInicio, p.HoraFin, p.DuracionHoras, p.Tarifa, p.Descripcion, p.Nota); err != nil {
		return nil, err
	}
	if err := ValidarTexto("titulo", "El titulo", p.Titulo, 0, 100); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Ciudad) != "" {
		ciudad, err := CiudadServicio("ciudad", p.Ciudad)
		if err != nil {
			return nil, err
		}
		p.Ciudad = ciudad
	}

	// Que exista y sea un empleado: sin esto se podria "contratar" a un
	// empleador, o a un id que no existe.
	if err := s.exigirEmpleado(ctx, empleado); err != nil {
		return nil, err
	}

	modalidad := p.Modalidad
	if modalidad == "" {
		modalidad = models.ModalidadPorHoras
	}
	idModalidad, err := s.catalogo.IDModalidad(ctx, modalidad)
	if err != nil {
		return nil, err
	}

	idOferta, err := s.ofertaDirigida(ctx, empleador, idModalidad, p)
	if err != nil {
		return nil, err
	}

	idPost, err := s.postulacionInvitada(ctx, idOferta, empleado, idModalidad, p)
	if err != nil {
		s.deshacer(ctx, "ofertas", idOferta)
		return nil, err
	}

	idEstado, err := s.catalogo.IDEstado(ctx, "contratacion", "activa")
	if err != nil {
		s.deshacer(ctx, "postulaciones", idPost)
		s.deshacer(ctx, "ofertas", idOferta)
		return nil, err
	}

	fecha := strings.TrimSpace(p.FechaInicio)
	if fecha == "" {
		fecha = time.Now().UTC().Format("2006-01-02")
	}
	total := p.Tarifa * p.DuracionHoras

	var creada filaContratacion
	if err := s.empleo.Post(ctx, "contrataciones", map[string]any{
		"IdEmpleador":        empleador,
		"IdEmpleado":         empleado,
		"IdOfertas":          map[string]any{"Id": idOferta},
		"IdPostulaciones":    map[string]any{"Id": idPost},
		"IdEstado":           idEstado,
		"DireccionServicio":  strings.TrimSpace(p.Direccion),
		"FechaInicio":        fecha + "T00:00:00Z",
		"HoraInicio":         strings.TrimSpace(p.HoraInicio),
		"DuracionHoras":      p.DuracionHoras,
		"IdModalidad":        idModalidad,
		"TarifaAcordada":     p.Tarifa,
		"TotalEstimado":      total,
		"DescripcionTrabajo": strings.TrimSpace(p.Descripcion),
		"NotaAdicional":      strings.TrimSpace(p.Nota),
		"Activo":             true,
	}, &creada); err != nil {
		s.deshacer(ctx, "postulaciones", idPost)
		s.deshacer(ctx, "ofertas", idOferta)
		return nil, err
	}
	if creada.Id == 0 {
		s.deshacer(ctx, "postulaciones", idPost)
		s.deshacer(ctx, "ofertas", idOferta)
		return nil, models.Upstream("empleo", errContratacionSinID)
	}

	// La agenda recurrente es opcional; su fallo no invalida la contratacion.
	if len(p.Dias) > 0 {
		_ = s.agendar(ctx, creada.Id, PeticionContratar{
			Dias:       p.Dias,
			HoraInicio: p.HoraInicio,
			HoraFin:    p.HoraFin,
		})
	}

	titulo := strings.TrimSpace(p.Titulo)
	if titulo == "" {
		titulo = "un trabajo"
	}
	s.avisarContratado(ctx, empleado, titulo)

	res := s.aDominio(ctx, creada)
	if res.EmpleadoID == "0" {
		res.EmpleadoID = strconv.Itoa(empleado)
	}
	res.TotalEstimado = total
	res.SoyEmpleador = true
	return &res, nil
}

// exigirEmpleado comprueba que el destinatario existe, esta activo y tiene rol
// de empleado.
func (s *Contrataciones) exigirEmpleado(ctx context.Context, id int) error {
	var perfiles []filaPerfil
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "perfiles_empleado", o, &perfiles); err != nil {
		return err
	}
	if len(perfiles) == 0 {
		return models.NoEncontrado("ese empleado")
	}
	return nil
}

// ofertaDirigida crea la oferta que respalda la contratacion directa.
//
// No se publica: nace cerrada, para que no aparezca en el buscador de empleos
// como si cualquiera pudiera postularse.
func (s *Contrataciones) ofertaDirigida(ctx context.Context, empleador, idModalidad int, p PeticionContratarDirecto) (int, error) {
	idEstado, err := s.catalogo.IDEstado(ctx, "oferta", "cerrada")
	if err != nil {
		return 0, err
	}
	idTamano, err := s.catalogo.IDTamano(ctx, models.TamanoMediano, false)
	if err != nil {
		return 0, err
	}
	idTipoInmueble, err := s.primerTipoInmueble(ctx)
	if err != nil {
		return 0, err
	}
	idMascota, err := s.catalogo.IDTipo(ctx, "mascota", "ninguna")
	if err != nil {
		return 0, err
	}

	titulo := strings.TrimSpace(p.Titulo)
	if titulo == "" {
		titulo = "Servicio contratado directamente"
	}
	ciudad := strings.TrimSpace(p.Ciudad)
	if ciudad == "" {
		ciudad = "Sin especificar"
	}

	var creada filaOferta
	if err := s.empleo.Post(ctx, "ofertas", map[string]any{
		"IdEmpleador":        empleador,
		"Titulo":             titulo,
		"Descripcion":        strings.TrimSpace(p.Descripcion),
		"IdTipoInmueble":     map[string]any{"Id": idTipoInmueble},
		"IdTamanoInmueble":   map[string]any{"Id": idTamano},
		"NumHabitaciones":    0,
		"NumBanos":           0,
		"Barrio":             "",
		"Direccion":          strings.TrimSpace(p.Direccion),
		"Ciudad":             ciudad,
		"TarifaHora":         p.Tarifa,
		"TarifaJornada":      0,
		"HorarioDescripcion": strings.TrimSpace(p.HoraInicio),
		"IdModalidadTrabajo": idModalidad,
		"InsumosCliente":     true,
		"Vigilancia":         false,
		"IdTipoMascota":      idMascota,
		"CondicionesExtra":   "",
		"IdEstado":           idEstado,
		"Destacada":          false,
		"Nueva":              false,
		"Vistas":             0,
		"Activo":             true,
		"ExpiraEn":           time.Now().AddDate(0, 1, 0).UTC().Format(time.RFC3339),
	}, &creada); err != nil {
		return 0, err
	}
	if creada.Id == 0 {
		return 0, models.Upstream("empleo", errAltaSinID)
	}
	return creada.Id, nil
}

// postulacionInvitada crea la postulacion que el empleador origina, ya
// aceptada: la persona no se postulo, la eligieron.
func (s *Contrataciones) postulacionInvitada(ctx context.Context, idOferta, empleado, idModalidad int, p PeticionContratarDirecto) (int, error) {
	idHV, err := s.hojaVidaDe(ctx, empleado)
	if err != nil {
		return 0, err
	}
	idEstado, err := s.catalogo.IDEstado(ctx, "postulacion", "aceptado")
	if err != nil {
		return 0, err
	}

	var creada filaPostulacion
	if err := s.empleo.Post(ctx, "postulaciones", map[string]any{
		"IdOfertas":          map[string]any{"Id": idOferta},
		"IdEmpleado":         empleado,
		"IdHojasVida":        map[string]any{"Id": idHV},
		"IdEstado":           idEstado,
		"DisponibleDesde":    time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"IdModalidadDisp":    idModalidad,
		"IdTurnoPostulacion": 1,
		"NotaDisponibilidad": "",
		"IdFormaCobro":       1,
		"TarifaMin":          p.Tarifa,
		"TarifaMax":          p.Tarifa,
		"MensajeEmpleador":   strings.TrimSpace(p.Nota),
		"AceptaTerminos":     true,
		"Activo":             true,
		// true = la inicio el empleador. Ver la DDL de la columna.
		"OrigenPostulacion": true,
	}, &creada); err != nil {
		return 0, err
	}
	if creada.Id == 0 {
		return 0, models.Upstream("empleo", errAltaSinID)
	}
	return creada.Id, nil
}

// deshacer borra una fila recien creada al fallar un paso posterior.
func (s *Contrataciones) deshacer(ctx context.Context, recurso string, id int) {
	if id == 0 {
		return
	}
	if err := s.empleo.Delete(ctx, recurso+"/"+strconv.Itoa(id)); err != nil {
		// Queda una fila huerfana. Sin transacciones no hay mas que hacer,
		// pero tiene que constar para poder limpiarla.
		slog.Error("no se pudo deshacer tras una contratacion directa fallida",
			"recurso", recurso, "id", id, "error", err)
	}
}

// primerTipoInmueble toma el primer tipo activo: la tabla lo exige y la
// contratacion directa no habla de inmuebles.
func (s *Contrataciones) primerTipoInmueble(ctx context.Context) (int, error) {
	var filas []struct {
		Id     int  `json:"Id"`
		Activo bool `json:"Activo"`
	}
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "50")
	if err := s.empleo.Get(ctx, "tipo_inmueble", o, &filas); err != nil {
		return 0, err
	}
	for _, f := range filas {
		if f.Activo {
			return f.Id, nil
		}
	}
	return 0, models.Upstream("empleo", errSinTipoInmueble)
}

// hojaVidaDe devuelve la hoja de vida del empleado. La postulacion la exige.
// Mismo criterio de eleccion que en Postulaciones: sin orden explicito el
// backend podria devolver una distinta en cada llamada.
func (s *Contrataciones) hojaVidaDe(ctx context.Context, idUsuario int) (int, error) {
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

	// Sin hoja de vida se crea una vacia, igual que cuando es la propia
	// persona quien se postula. Negarse a contratar a quien no la ha
	// rellenado dejaria al empleador ante un error que no puede arreglar.
	var creada filaHojaVida
	if err := s.empleo.Post(ctx, "hojas_vida", map[string]any{
		"IdUsuarios":       idUsuario,
		"Nombre":           "Hoja de vida",
		"Resumen":          "",
		"EsPredeterminada": true,
		"UrlPdf":           "",
		"EsPublica":        false,
		"ProgresoPct":      0,
		"Activo":           true,
	}, &creada); err != nil {
		return 0, err
	}
	if creada.Id == 0 {
		return 0, models.Upstream("empleo", errAltaSinID)
	}
	return creada.Id, nil
}
