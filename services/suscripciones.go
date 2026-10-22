package services

import (
	"context"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// Suscripciones implementa los planes y el alta de suscripcion.
//
// El backend no activa, renueva ni vence nada: auto_renovar es un booleano que
// nadie lee. Aqui al menos se fija el estado, se calcula la fecha de
// renovacion y se impide tener dos suscripciones activas a la vez.
type Suscripciones struct {
	susc     *ClienteUpstream
	catalogo *Catalogo
}

// NuevoSuscripciones crea el servicio de suscripciones sobre su upstream y el
// catalogo.
func NuevoSuscripciones(s *ClienteUpstream, cat *Catalogo) *Suscripciones {
	return &Suscripciones{susc: s, catalogo: cat}
}

type filaPlan struct {
	Id            int     `json:"Id"`
	Nombre        string  `json:"Nombre"`
	PrecioMensual float64 `json:"PrecioMensual"`
	PrecioAnual   float64 `json:"PrecioAnual"`
	Descripcion   string  `json:"Descripcion"`
	Activo        bool    `json:"Activo"`
}

type filaSuscripcion struct {
	Id         int         `json:"Id"`
	IdUsuarios int         `json:"IdUsuarios"`
	IdPlanes   *refAnidada `json:"IdPlanes"`
	IdEstado   int         `json:"IdEstado"`
	InicioEn   string      `json:"InicioEn"`
	FinEn      string      `json:"FinEn"`
	Activo     bool        `json:"Activo"`
}

// tierDesdeNombre deduce el nivel del plan, que la base no guarda como tal
// pero el frontend tipa como 'gratis' | 'basico' | 'premium'.
func tierDesdeNombre(nombre string) string {
	n := strings.ToLower(nombre)
	switch {
	case strings.Contains(n, "premium"), strings.Contains(n, "pro"):
		return "premium"
	case strings.Contains(n, "basic"), strings.Contains(n, "basico"):
		return "basico"
	default:
		return "gratis"
	}
}

// Planes devuelve el catalogo de planes de suscripcion con sus beneficios.
func (s *Suscripciones) Planes(ctx context.Context) ([]models.Plan, error) {
	var filas []filaPlan
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
	o.Query.Set("limit", "100")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "asc")
	if err := s.susc.Get(ctx, "planes", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.Plan, 0, len(filas))
	for _, f := range filas {
		tier := tierDesdeNombre(f.Nombre)
		out = append(out, models.Plan{
			ID:            strconv.Itoa(f.Id),
			Nombre:        f.Nombre,
			Tier:          tier,
			PrecioMensual: f.PrecioMensual,
			// plan_features existe como tabla pero sin datos de semilla; la
			// descripcion es lo unico que hay hoy.
			Beneficios: beneficiosDe(f.Descripcion),
			Destacado:  tier == "premium",
		})
	}
	return out, nil
}

// beneficiosDe parte la descripcion del plan en vinetas.
func beneficiosDe(desc string) []string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return []string{}
	}
	var out []string
	for _, p := range strings.Split(desc, ".") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{desc}
	}
	return out
}

// Mia devuelve la suscripcion vigente del usuario, o nil si no tiene.
func (s *Suscripciones) Mia(ctx context.Context, idUsuario string) (*models.Suscripcion, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	var filas []filaSuscripcion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "1")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.susc.Get(ctx, "usuarios_suscripciones", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 {
		return nil, nil
	}

	f := filas[0]
	idPlan := ""
	if f.IdPlanes != nil {
		idPlan = strconv.Itoa(f.IdPlanes.Id)
	}
	return &models.Suscripcion{
		ID:              strconv.Itoa(f.Id),
		UsuarioID:       strconv.Itoa(f.IdUsuarios),
		PlanID:          idPlan,
		Activa:          f.Activo,
		FechaInicio:     f.InicioEn,
		FechaRenovacion: f.FinEn,
	}, nil
}

// Suscribir da de alta una suscripcion mensual.
//
// No hay cobro: el sistema no tiene pasarela de pago. Queda registrada la
// suscripcion y el cobro es deuda pendiente.
func (s *Suscripciones) Suscribir(ctx context.Context, idUsuario, idPlan string) (*models.Suscripcion, error) {
	usuario, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	plan, err := IDNumerico(idPlan)
	if err != nil {
		return nil, models.Validacion("Identificador de plan invalido.").ConCampo("planId", "Invalido.")
	}

	var planes []filaPlan
	op := &OpcionesUpstream{Query: map[string][]string{}}
	op.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(plan)}))
	op.Query.Set("limit", "1")
	if err := s.susc.Get(ctx, "planes", op, &planes); err != nil {
		return nil, err
	}
	if len(planes) == 0 || !planes[0].Activo {
		return nil, models.NoEncontrado("ese plan")
	}

	// Cerrar la suscripcion anterior antes de abrir otra: dos activas a la vez
	// harian ambiguo que plan rige.
	if previa, err := s.Mia(ctx, idUsuario); err == nil && previa != nil {
		_ = s.cerrar(ctx, previa.ID, usuario)
	}

	idEstado, err := s.catalogo.IDEstado(ctx, "suscripcion", "activa")
	if err != nil {
		return nil, err
	}

	ahora := time.Now().UTC()
	cuerpo := map[string]any{
		"IdUsuarios":             usuario,
		"IdPlanes":               map[string]any{"Id": plan},
		"IdModalidadSuscripcion": 1,
		"IdEstado":               idEstado,
		"InicioEn":               ahora.Format(time.RFC3339),
		"FinEn":                  ahora.AddDate(0, 1, 0).Format(time.RFC3339),
		"AutoRenovar":            true,
		"Activo":                 true,
	}
	var creada filaSuscripcion
	if err := s.susc.Post(ctx, "usuarios_suscripciones", cuerpo, &creada); err != nil {
		return nil, err
	}

	return &models.Suscripcion{
		ID:              strconv.Itoa(creada.Id),
		UsuarioID:       idUsuario,
		PlanID:          idPlan,
		Activa:          true,
		FechaInicio:     ahora.Format(time.RFC3339),
		FechaRenovacion: ahora.AddDate(0, 1, 0).Format(time.RFC3339),
	}, nil
}

// cerrar marca como terminada una suscripcion activa del usuario, para poder
// dar de alta otra sin dejar dos vigentes a la vez.
func (s *Suscripciones) cerrar(ctx context.Context, idSuscripcion string, usuario int) error {
	id, err := IDNumerico(idSuscripcion)
	if err != nil {
		return err
	}
	idEstado, err := s.catalogo.IDEstado(ctx, "suscripcion", "cancelada")
	if err != nil {
		return err
	}
	return s.susc.Fijar(ctx, "usuarios_suscripciones/"+strconv.Itoa(id), map[string]any{
		"IdEstado":    idEstado,
		"AutoRenovar": false,
		"Activo":      false,
	})
}

// ---- Soporte ----

// Soporte expone los reportes del usuario.
