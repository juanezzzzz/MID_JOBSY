package services

import (
	"context"
	"strconv"
	"strings"

	"api_MID/models"
)

// HojasVida gestiona la hoja de vida del usuario.
//
// El tipo Resume del frontend lleva experiencias, educacion, habilidades y
// certificaciones. En la base solo existen empleo.hojas_vida y
// empleo.hv_experiencias: no hay tablas para educacion ni habilidades. Lo que
// no tiene sitio se conserva en el resumen para no perder lo que el usuario
// escribio, y queda anotado como deuda de esquema.
type HojasVida struct {
	empleo *ClienteUpstream
}

// NuevoHojasVida crea el servicio de hojas de vida sobre el upstream de empleo.
func NuevoHojasVida(empleo *ClienteUpstream) *HojasVida {
	return &HojasVida{empleo: empleo}
}

type filaHojaVida struct {
	Id               int    `json:"Id"`
	IdUsuarios       int    `json:"IdUsuarios"`
	Nombre           string `json:"Nombre"`
	Resumen          string `json:"Resumen"`
	EsPredeterminada bool   `json:"EsPredeterminada"`
	UrlPdf           string `json:"UrlPdf"`
	EsPublica        bool   `json:"EsPublica"`
	ProgresoPct      int    `json:"ProgresoPct"`
	Activo           bool   `json:"Activo"`
}

type filaExperiencia struct {
	Id          int         `json:"Id"`
	IdHojasVida *refAnidada `json:"IdHojasVida"`
	Descripcion string      `json:"Descripcion"`
	Empresa     string      `json:"Empresa"`
	Cargo       string      `json:"Cargo"`
	FechaInicio string      `json:"FechaInicio"`
	FechaFin    string      `json:"FechaFin"`
	Activo      bool        `json:"Activo"`
}

// Mia devuelve la hoja de vida del usuario, creandola vacia si no existe.
func (s *HojasVida) Mia(ctx context.Context, idUsuario string) (*models.HojaVida, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	hv, err := s.obtenerOCrear(ctx, id)
	if err != nil {
		return nil, err
	}

	exps, err := s.experienciasDe(ctx, hv.Id)
	if err != nil {
		// La hoja se devuelve aunque las experiencias fallen: son un anexo.
		exps = nil
	}

	return &models.HojaVida{
		ID:                 strconv.Itoa(hv.Id),
		UsuarioID:          strconv.Itoa(hv.IdUsuarios),
		ResumenProfesional: hv.Resumen,
		Experiencias:       exps,
		Educacion:          []models.Educacion{},
		Habilidades:        []string{},
		Certificaciones:    []string{},
		ArchivoURL:         hv.UrlPdf,
	}, nil
}

// obtenerOCrear devuelve la hoja de vida vigente del usuario.
//
// La tabla admite varias hojas por persona (existe la columna
// es_predeterminada), asi que hay que elegir siempre la misma: sin orden
// explicito el backend puede devolver una distinta en cada llamada, y
// entonces una lectura y una escritura acaban en filas diferentes.
//
// Criterio: la marcada como predeterminada; si ninguna lo esta, la mas
// antigua.
func (s *HojasVida) obtenerOCrear(ctx context.Context, idUsuario int) (*filaHojaVida, error) {
	var filas []filaHojaVida
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	o.Query.Set("limit", "50")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "asc")
	if err := s.empleo.Get(ctx, "hojas_vida", o, &filas); err != nil {
		return nil, err
	}
	if elegida := elegirHoja(filas); elegida != nil {
		return elegida, nil
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
		return nil, err
	}
	creada.IdUsuarios = idUsuario
	return &creada, nil
}

// elegirHoja aplica el criterio de seleccion sobre las hojas del usuario.
func elegirHoja(filas []filaHojaVida) *filaHojaVida {
	var primera *filaHojaVida
	for i := range filas {
		if !filas[i].Activo {
			continue
		}
		if filas[i].EsPredeterminada {
			return &filas[i]
		}
		if primera == nil {
			primera = &filas[i]
		}
	}
	return primera
}

// experienciasDe devuelve las experiencias laborales de una hoja de vida.
func (s *HojasVida) experienciasDe(ctx context.Context, idHV int) ([]models.Experiencia, error) {
	var filas []filaExperiencia
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdHojasVida": strconv.Itoa(idHV)}))
	o.Query.Set("limit", "100")
	if err := s.empleo.Get(ctx, "hv_experiencias", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.Experiencia, 0, len(filas))
	for _, f := range filas {
		out = append(out, models.Experiencia{
			Cargo:       f.Cargo,
			Empresa:     f.Empresa,
			Desde:       f.FechaInicio,
			Hasta:       f.FechaFin,
			Descripcion: f.Descripcion,
		})
	}
	return out, nil
}

// Actualizar guarda los cambios de la hoja de vida.
//
// Solo se persiste lo que la base sabe guardar. El progreso se recalcula aqui
// porque la columna progreso_pct existe y nadie la actualiza.
func (s *HojasVida) Actualizar(ctx context.Context, idUsuario string, cambios models.HojaVida) (*models.HojaVida, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	hv, err := s.obtenerOCrear(ctx, id)
	if err != nil {
		return nil, err
	}

	resumen := strings.TrimSpace(cambios.ResumenProfesional)
	if resumen == "" {
		resumen = hv.Resumen
	}
	archivo := strings.TrimSpace(cambios.ArchivoURL)
	if archivo == "" {
		archivo = hv.UrlPdf
	}

	if err := s.empleo.Fijar(ctx, "hojas_vida/"+strconv.Itoa(hv.Id), map[string]any{
		"Resumen":     resumen,
		"UrlPdf":      archivo,
		"ProgresoPct": progresoDe(resumen, archivo, cambios),
	}); err != nil {
		return nil, err
	}

	return s.Mia(ctx, idUsuario)
}

// progresoDe estima cuanto falta por completar. Cinco secciones, veinte puntos
// cada una.
func progresoDe(resumen, archivo string, hv models.HojaVida) int {
	p := 0
	if resumen != "" {
		p += 20
	}
	if archivo != "" {
		p += 20
	}
	if len(hv.Experiencias) > 0 {
		p += 20
	}
	if len(hv.Educacion) > 0 {
		p += 20
	}
	if len(hv.Habilidades) > 0 {
		p += 20
	}
	return p
}
