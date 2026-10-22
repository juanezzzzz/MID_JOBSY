package services

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"api_MID/models"
)

// Empleador reune lo que el empleador hace sobre sus propias ofertas: verlas
// con sus postulaciones, pausarlas o cerrarlas, subirles fotos y responder a
// quien se postula. Toda operacion comprueba que la oferta sea suya: el backend
// obedeceria cualquier id.
type Empleador struct {
	empleo    *ClienteUpstream
	cat       *Catalogo
	ofertas   *Ofertas
	post      *Postulaciones
	empleados *Empleados
	avisos    *Notificaciones
}

// NuevoEmpleador crea el servicio sobre Empleo_API y los servicios que reutiliza.
func NuevoEmpleador(empleo *ClienteUpstream, cat *Catalogo, ofertas *Ofertas, post *Postulaciones, empleados *Empleados, avisos *Notificaciones) *Empleador {
	return &Empleador{empleo: empleo, cat: cat, ofertas: ofertas, post: post, empleados: empleados, avisos: avisos}
}

// propia devuelve la oferta si existe y es del empleador.
func (s *Empleador) propia(ctx context.Context, empleador int, idOferta string) (*filaOferta, error) {
	id, err := IDNumerico(idOferta)
	if err != nil {
		return nil, err
	}
	var filas []filaOferta
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "ofertas", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return nil, models.NoEncontrado("esa oferta")
	}
	if filas[0].IdEmpleador != empleador {
		return nil, models.Prohibido("Esa oferta no es tuya.")
	}
	return &filas[0], nil
}

// fotosDe devuelve las fotos activas de la oferta, la principal primero.
func (s *Empleador) fotosDe(ctx context.Context, idOferta int) ([]filaOfertaFoto, error) {
	var fotos []filaOfertaFoto
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdOfertas": strconv.Itoa(idOferta), "Activo": "true"}))
	o.Query.Set("limit", "50")
	if err := s.empleo.Get(ctx, "oferta_fotos", o, &fotos); err != nil {
		return nil, err
	}
	sort.SliceStable(fotos, func(i, j int) bool {
		if fotos[i].EsPrincipal != fotos[j].EsPrincipal {
			return fotos[i].EsPrincipal
		}
		return fotos[i].Orden < fotos[j].Orden
	})
	return fotos, nil
}

// postulacionesDe devuelve las postulaciones vigentes de una oferta.
func (s *Empleador) postulacionesDe(ctx context.Context, idOferta int) ([]filaPostulacion, error) {
	var filas []filaPostulacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdOfertas": strconv.Itoa(idOferta), "Activo": "true"}))
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	return filas, s.empleo.Get(ctx, "postulaciones", o, &filas)
}

// aPropia completa una oferta con su estado, fotos y conteo de postulaciones.
func (s *Empleador) aPropia(ctx context.Context, f filaOferta) (*models.OfertaPropia, error) {
	base, err := s.ofertas.aDominio(ctx, f)
	if err != nil {
		return nil, err
	}
	p := &models.OfertaPropia{Oferta: base, Fotos: []string{}}
	if nombre, err := s.cat.EstadoDesdeID(ctx, f.IdEstado); err == nil {
		p.Estado = nombre
	}
	var (
		fotos []filaOfertaFoto
		posts []filaPostulacion
		etiq  []string
	)
	if err := EnParalelo(
		func() (err error) { fotos, err = s.fotosDe(ctx, f.Id); return },
		func() (err error) { posts, err = s.postulacionesDe(ctx, f.Id); return },
		func() error {
			if e, err := s.ofertas.etiquetasDe(ctx, f.Id); err == nil {
				etiq = e
			}
			return nil
		},
	); err != nil {
		return nil, err
	}
	for _, foto := range fotos {
		p.Fotos = append(p.Fotos, foto.UrlFoto)
	}
	if len(p.Fotos) > 0 {
		p.ImagenURL = p.Fotos[0]
	}
	if etiq != nil {
		p.Etiquetas = etiq
	}
	p.Postulaciones = len(posts)
	for _, po := range posts {
		if est := s.post.aDominio(ctx, po).Estado; est == models.PostPendiente || est == models.PostEnRevision {
			p.Pendientes++
		}
	}
	return p, nil
}

// MisOfertas lista las ofertas del empleador que no estan borradas, la mas
// reciente primero.
func (s *Empleador) MisOfertas(ctx context.Context, idEmpleador string) ([]models.OfertaPropia, error) {
	empleador, err := IDNumerico(idEmpleador)
	if err != nil {
		return nil, err
	}
	var filas []filaOferta
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdEmpleador": strconv.Itoa(empleador), "Activo": "true"}))
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.empleo.Get(ctx, "ofertas", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.OfertaPropia, 0, len(filas))
	for _, f := range filas {
		p, err := s.aPropia(ctx, f)
		if err != nil {
			slog.Warn("oferta propia omitida", "oferta", f.Id, "motivo", err)
			continue
		}
		if p.Estado == "borrada" {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}

// Recibidas devuelve las postulaciones a una oferta propia con la ficha de
// cada candidato.
func (s *Empleador) Recibidas(ctx context.Context, idEmpleador, idOferta string) ([]models.PostulacionRecibida, error) {
	empleador, err := IDNumerico(idEmpleador)
	if err != nil {
		return nil, err
	}
	of, err := s.propia(ctx, empleador, idOferta)
	if err != nil {
		return nil, err
	}
	posts, err := s.postulacionesDe(ctx, of.Id)
	if err != nil {
		return nil, err
	}

	// Que postulaciones ya se contrataron: una lectura por oferta.
	var contrs []filaContratacion
	oc := &OpcionesUpstream{Query: map[string][]string{}}
	oc.Query.Set("query", QueryBeego(map[string]string{"IdOfertas": strconv.Itoa(of.Id), "Activo": "true"}))
	oc.Query.Set("limit", "200")
	contratada := map[int]int{}
	if err := s.empleo.Get(ctx, "contrataciones", oc, &contrs); err == nil {
		for _, c := range contrs {
			if c.IdPostulaciones != nil {
				contratada[c.IdPostulaciones.Id] = c.Id
			}
		}
	}

	out := make([]models.PostulacionRecibida, len(posts))
	tareas := make([]func() error, len(posts))
	for i, p := range posts {
		i, p := i, p
		out[i] = models.PostulacionRecibida{Postulacion: s.post.aDominio(ctx, p)}
		if id, ok := contratada[p.Id]; ok {
			out[i].ContratacionID = strconv.Itoa(id)
		}
		tareas[i] = func() error {
			// Sin ficha se muestra la postulacion igual: el candidato puede no
			// tener perfil de empleo completo.
			if e, err := s.empleados.PorID(ctx, strconv.Itoa(p.IdEmpleado)); err == nil {
				out[i].Empleado = e
			}
			return nil
		}
	}
	_ = EnParalelo(tareas...)
	return out, nil
}

// CambiarEstadoPostulacion pasa una postulacion a revision o la rechaza, y
// avisa al candidato. Aceptar es contratar (Contrataciones.Crear).
func (s *Empleador) CambiarEstadoPostulacion(ctx context.Context, idEmpleador, idPostulacion, estado string) (*models.Postulacion, error) {
	empleador, err := IDNumerico(idEmpleador)
	if err != nil {
		return nil, err
	}
	id, err := IDNumerico(idPostulacion)
	if err != nil {
		return nil, err
	}
	nombreBase, texto := "", ""
	switch models.EstadoPostulacion(strings.TrimSpace(estado)) {
	case models.PostEnRevision:
		nombreBase, texto = "revisando", "esta en revision"
	case models.PostRechazada:
		nombreBase, texto = "rechazado", "no fue seleccionada"
	default:
		return nil, models.Validacion("Estado no valido: usa en_revision o rechazada. Para aceptar, contrata.").ConCampo("estado", "No valido.")
	}

	var filas []filaPostulacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "postulaciones", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 || !filas[0].Activo || filas[0].IdOfertas == nil {
		return nil, models.NoEncontrado("esa postulacion")
	}
	of, err := s.propia(ctx, empleador, strconv.Itoa(filas[0].IdOfertas.Id))
	if err != nil {
		return nil, err
	}
	if actual := s.post.aDominio(ctx, filas[0]).Estado; actual == models.PostAceptada {
		return nil, models.Conflicto("Esa postulacion ya fue aceptada con una contratacion.")
	}

	idEstado, err := s.cat.IDEstado(ctx, "postulacion", nombreBase)
	if err != nil {
		return nil, err
	}
	if err := s.empleo.Fijar(ctx, "postulaciones/"+strconv.Itoa(id), map[string]any{"IdEstado": idEstado}); err != nil {
		return nil, err
	}
	s.avisar(ctx, filas[0].IdEmpleado, "Tu postulacion "+texto,
		fmt.Sprintf("Tu postulacion a «%s» %s.", of.Titulo, texto), "/app/mis-postulaciones")

	filas[0].IdEstado = idEstado
	res := s.post.aDominio(ctx, filas[0])
	return &res, nil
}

// CambiarEstadoOferta activa, pausa o cierra una oferta propia. Solo las
// activas aparecen en el buscador y admiten postulaciones.
func (s *Empleador) CambiarEstadoOferta(ctx context.Context, idEmpleador, idOferta, estado string) (*models.OfertaPropia, error) {
	empleador, err := IDNumerico(idEmpleador)
	if err != nil {
		return nil, err
	}
	estado = strings.TrimSpace(estado)
	if estado != "activa" && estado != "pausada" && estado != "cerrada" {
		return nil, models.Validacion("Estado no valido: usa activa, pausada o cerrada.").ConCampo("estado", "No valido.")
	}
	of, err := s.propia(ctx, empleador, idOferta)
	if err != nil {
		return nil, err
	}
	idEstado, err := s.cat.IDEstado(ctx, "oferta", estado)
	if err != nil {
		return nil, err
	}
	if err := s.empleo.Fijar(ctx, "ofertas/"+strconv.Itoa(of.Id), map[string]any{"IdEstado": idEstado}); err != nil {
		return nil, err
	}
	of.IdEstado = idEstado
	return s.aPropia(ctx, *of)
}

// AgregarFoto asocia una foto ya guardada a una oferta propia. La primera
// queda como principal, que es la que muestran el buscador y la ficha.
func (s *Empleador) AgregarFoto(ctx context.Context, idEmpleador, idOferta, url string) (*models.OfertaPropia, error) {
	empleador, err := IDNumerico(idEmpleador)
	if err != nil {
		return nil, err
	}
	of, err := s.propia(ctx, empleador, idOferta)
	if err != nil {
		return nil, err
	}
	fotos, err := s.fotosDe(ctx, of.Id)
	if err != nil {
		return nil, err
	}
	if len(fotos) >= 10 {
		return nil, models.Validacion("Una oferta admite como maximo 10 fotos.")
	}
	var creada map[string]any
	if err := s.empleo.Post(ctx, "oferta_fotos", map[string]any{
		"IdOfertas":   map[string]any{"Id": of.Id},
		"UrlFoto":     url,
		"Orden":       len(fotos) + 1,
		"EsPrincipal": len(fotos) == 0,
		"Activo":      true,
	}, &creada); err != nil {
		return nil, err
	}
	return s.aPropia(ctx, *of)
}

// avisar deja una notificacion; si falla solo se registra, porque la accion
// principal ya se hizo.
func (s *Empleador) avisar(ctx context.Context, idUsuario int, titulo, cuerpo, destino string) {
	if s.avisos == nil {
		return
	}
	if err := s.avisos.Crear(ctx, idUsuario, "postulacion", titulo, cuerpo, destino); err != nil {
		slog.Warn("no se pudo crear la notificacion", "usuario", idUsuario, "error", err)
	}
}
