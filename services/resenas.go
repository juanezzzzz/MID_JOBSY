package services

import (
	"context"
	"strconv"
	"strings"

	"api_MID/models"
)

// ---- Resenas ----

// Resenas gestiona las valoraciones entre empleador y empleado.
//
// El frontend guardaba las resenas en localStorage, asi que solo las veia
// quien las escribia. La tabla empleo.resenas existe y ademas hay columnas
// rating_promedio y total_resenas en perfiles_empleado que nadie actualizaba:
// aqui se recalculan al publicar.
type Resenas struct {
	empleo   *ClienteUpstream
	catalogo *Catalogo
}

// NuevoResenas crea el servicio de resenas sobre el upstream de empleo y el
// catalogo.
func NuevoResenas(empleo *ClienteUpstream, cat *Catalogo) *Resenas {
	return &Resenas{empleo: empleo, catalogo: cat}
}

type filaResena struct {
	Id              int         `json:"Id"`
	IdPostulaciones *refAnidada `json:"IdPostulaciones"`
	IdReviewer      int         `json:"IdReviewer"`
	IdReviewed      int         `json:"IdReviewed"`
	Calificacion    float64     `json:"Calificacion"`
	Comentario      string      `json:"Comentario"`
	IdTipo          int         `json:"IdTipo"`
	Activo          bool        `json:"Activo"`
	CreadoEn        string      `json:"CreadoEn"`
}

// PeticionResena es lo que manda la ficha del empleado.
type PeticionResena struct {
	// SobreUsuarioID es a quien se valora.
	SobreUsuarioID string  `json:"sobreUsuarioId"`
	PostulacionID  string  `json:"postulacionId,omitempty"`
	Calificacion   float64 `json:"calificacion"`
	Comentario     string  `json:"comentario"`
}

// De devuelve las resenas recibidas por un usuario.
func (s *Resenas) De(ctx context.Context, idUsuario string) ([]models.Resena, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	var filas []filaResena
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdReviewed": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.empleo.Get(ctx, "resenas", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.Resena, 0, len(filas))
	for _, f := range filas {
		out = append(out, models.Resena{
			ID:        strconv.Itoa(f.Id),
			Autor:     strconv.Itoa(f.IdReviewer),
			Estrellas: f.Calificacion,
			Texto:     f.Comentario,
			Fecha:     f.CreadoEn,
		})
	}
	return out, nil
}

// Publicar deja una resena y actualiza el promedio del valorado.
//
// Solo se puede valorar a alguien con quien se ha trabajado: sin esa
// comprobacion cualquiera podria hundir la reputacion de otro.
func (s *Resenas) Publicar(ctx context.Context, idAutor string, p PeticionResena) (*models.Resena, error) {
	autor, err := IDNumerico(idAutor)
	if err != nil {
		return nil, err
	}
	sobre, err := IDNumerico(p.SobreUsuarioID)
	if err != nil {
		return nil, models.Validacion("Identificador de usuario invalido.").
			ConCampo("sobreUsuarioId", "Invalido.")
	}
	if autor == sobre {
		return nil, models.Validacion("No puedes valorarte a ti mismo.")
	}
	if p.Calificacion < 1 || p.Calificacion > 5 {
		return nil, models.Validacion("La calificacion debe estar entre 1 y 5.").
			ConCampo("calificacion", "Entre 1 y 5.")
	}
	if err := ValidarTexto("comentario", "El comentario", p.Comentario, 0, 500); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Comentario) == "" {
		return nil, models.Validacion("Escribe un comentario.").ConCampo("comentario", "Obligatorio.")
	}

	idPost, err := s.postulacionCompartida(ctx, autor, sobre, p.PostulacionID)
	if err != nil {
		return nil, err
	}

	// El contexto 'resena' distingue quien valora a quien.
	nombreTipo := "empleador_a_empleado"
	if !s.esEmpleadorDe(ctx, autor, idPost) {
		nombreTipo = "empleado_a_empleador"
	}
	idTipo, err := s.catalogo.IDTipo(ctx, "resena", nombreTipo)
	if err != nil {
		return nil, err
	}

	cuerpo := map[string]any{
		"IdPostulaciones": map[string]any{"Id": idPost},
		"IdReviewer":      autor,
		"IdReviewed":      sobre,
		"Calificacion":    p.Calificacion,
		"Comentario":      strings.TrimSpace(p.Comentario),
		"IdTipo":          idTipo,
		"Activo":          true,
	}
	var creada filaResena
	if err := s.empleo.Post(ctx, "resenas", cuerpo, &creada); err != nil {
		return nil, err
	}

	// El promedio del perfil se recalcula: las columnas existen y nadie las
	// mantenia, asi que mostraban siempre cero.
	_ = s.recalcularPromedio(ctx, sobre)

	return &models.Resena{
		ID:        strconv.Itoa(creada.Id),
		Autor:     strconv.Itoa(autor),
		Estrellas: p.Calificacion,
		Texto:     strings.TrimSpace(p.Comentario),
	}, nil
}

// postulacionCompartida verifica que autor y valorado hayan coincidido en una
// contratacion, y devuelve la postulacion que lo acredita.
func (s *Resenas) postulacionCompartida(ctx context.Context, autor, sobre int, preferida string) (int, error) {
	var contrataciones []filaContratacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
	o.Query.Set("limit", "500")
	if err := s.empleo.Get(ctx, "contrataciones", o, &contrataciones); err != nil {
		return 0, err
	}

	deseada, _ := strconv.Atoi(strings.TrimSpace(preferida))
	var encontrada int
	for _, c := range contrataciones {
		juntos := (c.IdEmpleador == autor && c.IdEmpleado == sobre) ||
			(c.IdEmpleado == autor && c.IdEmpleador == sobre)
		if !juntos || c.IdPostulaciones == nil {
			continue
		}
		if deseada > 0 && c.IdPostulaciones.Id == deseada {
			return deseada, nil
		}
		if encontrada == 0 {
			encontrada = c.IdPostulaciones.Id
		}
	}
	if encontrada == 0 {
		return 0, models.Prohibido("Solo puedes valorar a alguien con quien hayas trabajado.")
	}
	return encontrada, nil
}

// esEmpleadorDe indica si el usuario es el empleador de la oferta a la que
// pertenece la postulacion, para decidir que tipo de resena puede escribir.
func (s *Resenas) esEmpleadorDe(ctx context.Context, usuario, idPostulacion int) bool {
	var filas []filaContratacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdPostulaciones": strconv.Itoa(idPostulacion),
	}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "contrataciones", o, &filas); err != nil || len(filas) == 0 {
		return false
	}
	return filas[0].IdEmpleador == usuario
}

// recalcularPromedio actualiza rating_promedio y total_resenas del perfil.
func (s *Resenas) recalcularPromedio(ctx context.Context, idUsuario int) error {
	var resenas []filaResena
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdReviewed": strconv.Itoa(idUsuario),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "1000")
	if err := s.empleo.Get(ctx, "resenas", o, &resenas); err != nil {
		return err
	}
	if len(resenas) == 0 {
		return nil
	}

	suma := 0.0
	for _, r := range resenas {
		suma += r.Calificacion
	}
	promedio := redondear(suma/float64(len(resenas)), 2)

	var perfiles []filaPerfil
	op := &OpcionesUpstream{Query: map[string][]string{}}
	op.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	op.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "perfiles_empleado", op, &perfiles); err != nil || len(perfiles) == 0 {
		return err
	}
	p := perfiles[0]

	return s.empleo.Fijar(ctx, "perfiles_empleado/"+strconv.Itoa(p.Id), map[string]any{
		"RatingPromedio": promedio,
		"TotalResenas":   len(resenas),
	})
}

// ---- Notificaciones ----

// Notificaciones es la bandeja de avisos del usuario.
