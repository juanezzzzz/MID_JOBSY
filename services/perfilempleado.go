package services

import (
	"context"
	"strconv"
	"strings"

	"api_MID/models"
)

// PerfilEmpleado gestiona lo que un empleado ofrece: su disponibilidad
// semanal, sus especialidades y sus documentos de verificacion.
//
// La pantalla de agendar del frontend mostraba franjas horarias fijas en el
// codigo y no consultaba la disponibilidad real de nadie. Las tablas
// empleo.disponibilidad_empleado y empleo.documentos_usuario llevaban vacias.
type PerfilEmpleado struct {
	empleo   *ClienteUpstream
	catalogo *Catalogo
}

// NuevoPerfilEmpleado crea el servicio de perfil de empleado sobre el upstream
// de empleo y el catalogo.
func NuevoPerfilEmpleado(empleo *ClienteUpstream, cat *Catalogo) *PerfilEmpleado {
	return &PerfilEmpleado{empleo: empleo, catalogo: cat}
}

type filaDisponibilidad struct {
	Id                 int         `json:"Id"`
	IdPerfilesEmpleado *refAnidada `json:"IdPerfilesEmpleado"`
	IdDiaSemana        int         `json:"IdDiaSemana"`
	IdTurno            int         `json:"IdTurno"`
	HoraInicio         string      `json:"HoraInicio"`
	HoraFin            string      `json:"HoraFin"`
	Activo             bool        `json:"Activo"`
}

type filaDocumento struct {
	Id         int    `json:"Id"`
	IdUsuarios int    `json:"IdUsuarios"`
	IdTipo     int    `json:"IdTipo"`
	UrlArchivo string `json:"UrlArchivo"`
	IdEstado   int    `json:"IdEstado"`
	Notas      string `json:"Notas"`
	Activo     bool   `json:"Activo"`
	CreadoEn   string `json:"CreadoEn"`
}

// perfilDe localiza el perfil de empleado de un usuario.
func (s *PerfilEmpleado) perfilDe(ctx context.Context, idUsuario int) (*filaPerfil, error) {
	var filas []filaPerfil
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "perfiles_empleado", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 {
		return nil, models.NoEncontrado("el perfil de ese empleado")
	}
	return &filas[0], nil
}

// Disponibilidad devuelve las franjas en que el empleado puede trabajar.
func (s *PerfilEmpleado) Disponibilidad(ctx context.Context, idUsuario string) ([]models.FranjaAgenda, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	perfil, err := s.perfilDe(ctx, id)
	if err != nil {
		// Un usuario sin perfil de empleado no tiene disponibilidad, que no
		// es lo mismo que un error.
		return []models.FranjaAgenda{}, nil
	}

	var filas []filaDisponibilidad
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdPerfilesEmpleado": strconv.Itoa(perfil.Id),
		"Activo":             "true",
	}))
	o.Query.Set("limit", "100")
	if err := s.empleo.Get(ctx, "disponibilidad_empleado", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.FranjaAgenda, 0, len(filas))
	for _, f := range filas {
		dia, _ := s.catalogo.DiaSemanaDesdeID(ctx, f.IdDiaSemana)
		out = append(out, models.FranjaAgenda{
			Dia:        dia,
			HoraInicio: f.HoraInicio,
			HoraFin:    f.HoraFin,
		})
	}
	return out, nil
}

// FijarDisponibilidad reemplaza la agenda semanal del empleado.
//
// Se reemplaza en bloque en vez de ir franja a franja: el frontend envia la
// semana entera y asi no quedan restos de una configuracion anterior.
func (s *PerfilEmpleado) FijarDisponibilidad(ctx context.Context, idUsuario string, franjas []models.FranjaAgenda) ([]models.FranjaAgenda, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	perfil, err := s.perfilDe(ctx, id)
	if err != nil {
		return nil, err
	}

	for _, f := range franjas {
		if f.HoraInicio == "" || f.HoraFin == "" {
			return nil, models.Validacion("Cada franja necesita hora de inicio y de fin.")
		}
		if err := ValidarHora("horaInicio", f.HoraInicio); err != nil {
			return nil, err
		}
		if err := ValidarHora("horaFin", f.HoraFin); err != nil {
			return nil, err
		}
		if err := ValidarTexto("nota", "La nota", f.Nota, 0, 200); err != nil {
			return nil, err
		}
		if f.HoraFin <= f.HoraInicio {
			return nil, models.Validacion("La hora de fin debe ser posterior a la de inicio.")
		}
	}

	// La tabla admite una fila por (perfil, dia, turno), y las filas viejas no se
	// borran (puede haber historico colgando): se reutiliza la del mismo dia y
	// turno y se desactivan las que ya no estan. Antes se creaban filas nuevas
	// ignorando el error, y el indice unico las rechazaba en silencio.
	type clave struct{ dia, turno int }
	nuevas := map[clave]models.FranjaAgenda{}
	for _, f := range franjas {
		idDia, err := s.catalogo.IDDiaSemana(ctx, f.Dia)
		if err != nil {
			return nil, models.Validacion("Dia de la semana desconocido: " + f.Dia)
		}
		k := clave{idDia, turnoDe(f.HoraInicio)}
		if _, dup := nuevas[k]; dup {
			return nil, models.Validacion("Solo puedes tener una franja de manana, una de tarde y una de noche por dia.")
		}
		nuevas[k] = f
	}

	var todas []filaDisponibilidad
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdPerfilesEmpleado": strconv.Itoa(perfil.Id)}))
	o.Query.Set("limit", "200")
	if err := s.empleo.Get(ctx, "disponibilidad_empleado", o, &todas); err != nil {
		return nil, err
	}
	existentes := map[clave]filaDisponibilidad{}
	for _, f := range todas {
		existentes[clave{f.IdDiaSemana, f.IdTurno}] = f
	}

	for k, f := range nuevas {
		if fila, ok := existentes[k]; ok {
			if err := s.empleo.Fijar(ctx, "disponibilidad_empleado/"+strconv.Itoa(fila.Id), map[string]any{
				"HoraInicio": f.HoraInicio, "HoraFin": f.HoraFin, "Activo": true,
			}); err != nil {
				return nil, err
			}
			continue
		}
		var creada map[string]any
		if err := s.empleo.Post(ctx, "disponibilidad_empleado", map[string]any{
			"IdPerfilesEmpleado": map[string]any{"Id": perfil.Id},
			"IdDiaSemana":        k.dia,
			"IdTurno":            k.turno,
			"HoraInicio":         f.HoraInicio,
			"HoraFin":            f.HoraFin,
			"Activo":             true,
		}, &creada); err != nil {
			return nil, err
		}
	}
	for k, fila := range existentes {
		if _, sigue := nuevas[k]; !sigue && fila.Activo {
			if err := s.empleo.Fijar(ctx, "disponibilidad_empleado/"+strconv.Itoa(fila.Id), map[string]any{"Activo": false}); err != nil {
				return nil, err
			}
		}
	}

	return s.Disponibilidad(ctx, idUsuario)
}

// ---- Documentos de verificacion ----

// Documentos devuelve los documentos que el usuario ha subido.
func (s *PerfilEmpleado) Documentos(ctx context.Context, idUsuario string) ([]models.DocumentoUsuario, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	var filas []filaDocumento
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "50")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.empleo.Get(ctx, "documentos_usuario", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.DocumentoUsuario, 0, len(filas))
	for _, f := range filas {
		estado := "pendiente"
		if n, err := s.catalogo.EstadoDesdeID(ctx, f.IdEstado); err == nil {
			estado = n
		}
		out = append(out, models.DocumentoUsuario{
			ID:         strconv.Itoa(f.Id),
			Tipo:       s.nombreTipoDocumento(ctx, f.IdTipo),
			ArchivoURL: f.UrlArchivo,
			Estado:     estado,
			Notas:      f.Notas,
			Fecha:      f.CreadoEn,
		})
	}
	return out, nil
}

// nombreTipoDocumento resuelve el nombre legible del tipo de documento a
// partir de su id, usando el catalogo.
func (s *PerfilEmpleado) nombreTipoDocumento(ctx context.Context, idTipo int) string {
	// El catalogo de tipos es polimorfico y no expone la busqueda inversa por
	// contexto, asi que se prueban los del contexto de documentos.
	for _, n := range []string{"foto", "antecedentes", "hoja_vida"} {
		if id, err := s.catalogo.IDTipo(ctx, "documento_usuario", n); err == nil && id == idTipo {
			return n
		}
	}
	return "documento"
}

// SubirDocumento registra un documento de verificacion ya almacenado.
//
// El archivo se sube por el endpoint de archivos; aqui solo se registra su
// url y queda pendiente de revision. La verificacion la hace una persona:
// marcar el documento como valido automaticamente vaciaria de sentido el
// distintivo de perfil verificado.
func (s *PerfilEmpleado) SubirDocumento(ctx context.Context, idUsuario, tipo, url string) (*models.DocumentoUsuario, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	tipo = strings.ToLower(strings.TrimSpace(tipo))
	if tipo == "" {
		tipo = "foto"
	}
	if strings.TrimSpace(url) == "" {
		return nil, models.Validacion("Falta la url del documento.").ConCampo("url", "Obligatoria.")
	}

	idTipo, err := s.catalogo.IDTipo(ctx, "documento_usuario", tipo)
	if err != nil {
		return nil, models.Validacion("Tipo de documento no reconocido: "+tipo).
			ConCampo("tipo", "No valido.")
	}
	idEstado, err := s.catalogo.IDEstado(ctx, "documento", "pendiente")
	if err != nil {
		return nil, err
	}

	var creado filaDocumento
	if err := s.empleo.Post(ctx, "documentos_usuario", map[string]any{
		"IdUsuarios": id,
		"IdTipo":     idTipo,
		"UrlArchivo": strings.TrimSpace(url),
		"IdEstado":   idEstado,
		"Notas":      "",
		"Activo":     true,
	}, &creado); err != nil {
		return nil, err
	}

	return &models.DocumentoUsuario{
		ID:         strconv.Itoa(creado.Id),
		Tipo:       tipo,
		ArchivoURL: url,
		Estado:     "pendiente",
	}, nil
}

// turnoDe ubica una franja en el turno del catalogo por su hora de inicio:
// 1 manana (antes de las 12), 2 tarde (antes de las 18), 3 noche.
func turnoDe(horaInicio string) int {
	switch {
	case horaInicio < "12:00":
		return 1
	case horaInicio < "18:00":
		return 2
	default:
		return 3
	}
}
