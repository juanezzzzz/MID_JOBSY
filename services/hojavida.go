package services

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// HojasVida gestiona la hoja de vida del usuario.
//
// El tipo Resume del frontend lleva experiencias, educacion, habilidades y
// certificaciones. Las experiencias van a empleo.hv_experiencias y las
// certificaciones a empleo.certificaciones (del perfil de empleado); educacion
// y habilidades, que no tienen tabla, se guardan como JSON en la propia hoja.
type HojasVida struct {
	empleo *ClienteUpstream
	cat    *Catalogo
}

// NuevoHojasVida crea el servicio de hojas de vida sobre el upstream de empleo.
func NuevoHojasVida(empleo *ClienteUpstream, cat *Catalogo) *HojasVida {
	return &HojasVida{empleo: empleo, cat: cat}
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
	Educacion        string `json:"Educacion"`
	Habilidades      string `json:"Habilidades"`
	Activo           bool   `json:"Activo"`
}

type filaCertificacion struct {
	Id     int    `json:"Id"`
	Nombre string `json:"Nombre"`
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
	certs := []string{}
	if idPerfil, _ := s.perfilDe(ctx, id); idPerfil > 0 {
		if filas, err := s.certificacionesDe(ctx, idPerfil); err == nil {
			for _, c := range filas {
				certs = append(certs, c.Nombre)
			}
		}
	}

	educacion := []models.Educacion{}
	_ = json.Unmarshal([]byte(hv.Educacion), &educacion)
	habilidades := []string{}
	_ = json.Unmarshal([]byte(hv.Habilidades), &habilidades)

	return &models.HojaVida{
		ID:                 strconv.Itoa(hv.Id),
		UsuarioID:          strconv.Itoa(hv.IdUsuarios),
		ResumenProfesional: hv.Resumen,
		Experiencias:       exps,
		Educacion:          educacion,
		Habilidades:        habilidades,
		Certificaciones:    certs,
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
			Desde:       aMes(f.FechaInicio),
			Hasta:       aMes(f.FechaFin),
			Descripcion: f.Descripcion,
		})
	}
	return out, nil
}

// aMes deja una fecha del backend ("2022-01-10T00:00:00Z") como la maneja el
// <input type="month"> del front: "2022-01".
func aMes(fecha string) string {
	if len(fecha) >= 7 {
		return fecha[:7]
	}
	return ""
}

// desdeMes acepta "2022-01" o "2022-01-10" y devuelve la fecha para el backend.
func desdeMes(valor, campo string) (string, error) {
	valor = strings.TrimSpace(valor)
	for _, formato := range []string{"2006-01", "2006-01-02"} {
		if t, err := time.Parse(formato, valor); err == nil {
			return t.Format(time.RFC3339), nil
		}
	}
	return "", models.Validacion("Fecha de experiencia no valida: " + valor).ConCampo(campo, "Usa el formato AAAA-MM.")
}

// perfilDe devuelve el id del perfil de empleado del usuario, o 0 si no tiene.
func (s *HojasVida) perfilDe(ctx context.Context, idUsuario int) (int, error) {
	var filas []refAnidada
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "perfiles_empleado", o, &filas); err != nil || len(filas) == 0 {
		return 0, err
	}
	return filas[0].Id, nil
}

func (s *HojasVida) certificacionesDe(ctx context.Context, idPerfil int) ([]filaCertificacion, error) {
	var filas []filaCertificacion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdPerfilesEmpleado": strconv.Itoa(idPerfil)}))
	o.Query.Set("limit", "100")
	return filas, s.empleo.Get(ctx, "certificaciones", o, &filas)
}

// guardarExperiencias reemplaza las experiencias de la hoja por las recibidas.
func (s *HojasVida) guardarExperiencias(ctx context.Context, idHV int, exps []models.Experiencia) error {
	nuevas := make([]map[string]any, 0, len(exps))
	idTipo, err := s.cat.IDTipo(ctx, "experiencia_hv", "hogar_residencial")
	if err != nil {
		return err
	}
	for i, e := range exps {
		desde, err := desdeMes(e.Desde, "experiencias["+strconv.Itoa(i)+"].desde")
		if err != nil {
			return err
		}
		fila := map[string]any{
			"IdHojasVida": map[string]any{"Id": idHV},
			"IdTipo":      idTipo,
			"Cargo":       strings.TrimSpace(e.Cargo),
			"Empresa":     strings.TrimSpace(e.Empresa),
			"Descripcion": strings.TrimSpace(e.Descripcion),
			"FechaInicio": desde,
			"Activo":      true,
		}
		if strings.TrimSpace(e.Hasta) != "" {
			hasta, err := desdeMes(e.Hasta, "experiencias["+strconv.Itoa(i)+"].hasta")
			if err != nil {
				return err
			}
			fila["FechaFin"] = hasta
		}
		nuevas = append(nuevas, fila)
	}

	var actuales []refAnidada
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdHojasVida": strconv.Itoa(idHV)}))
	o.Query.Set("limit", "100")
	if err := s.empleo.Get(ctx, "hv_experiencias", o, &actuales); err != nil {
		return err
	}
	for _, a := range actuales {
		if err := s.empleo.Delete(ctx, "hv_experiencias/"+strconv.Itoa(a.Id)); err != nil {
			return err
		}
	}
	for _, f := range nuevas {
		var creada map[string]any
		if err := s.empleo.Post(ctx, "hv_experiencias", f, &creada); err != nil {
			return err
		}
	}
	return nil
}

// guardarCertificaciones sincroniza por nombre: conserva las que siguen (con
// su estado de validacion), crea las nuevas como pendientes y retira las que
// el usuario quito. Solo los empleados tienen perfil donde colgarlas.
func (s *HojasVida) guardarCertificaciones(ctx context.Context, idUsuario int, nombres []string) error {
	idPerfil, err := s.perfilDe(ctx, idUsuario)
	if err != nil {
		return err
	}
	if idPerfil == 0 {
		if len(nombres) > 0 {
			return models.Validacion("Solo los empleados pueden registrar certificaciones.").ConCampo("certificaciones", "Sin perfil de empleado.")
		}
		return nil
	}
	actuales, err := s.certificacionesDe(ctx, idPerfil)
	if err != nil {
		return err
	}
	pedidas := map[string]string{}
	for _, n := range nombres {
		if t := strings.TrimSpace(n); t != "" {
			pedidas[normalizar(t)] = t
		}
	}
	var errs []error
	for _, a := range actuales {
		if _, sigue := pedidas[normalizar(a.Nombre)]; sigue {
			delete(pedidas, normalizar(a.Nombre))
			continue
		}
		errs = append(errs, s.empleo.Delete(ctx, "certificaciones/"+strconv.Itoa(a.Id)))
	}
	if len(pedidas) > 0 {
		idEstado, err := s.cat.IDEstado(ctx, "certificacion", "pendiente")
		if err != nil {
			return err
		}
		for _, nombre := range pedidas {
			var creada map[string]any
			errs = append(errs, s.empleo.Post(ctx, "certificaciones", map[string]any{
				"IdPerfilesEmpleado": map[string]any{"Id": idPerfil},
				"Nombre":             nombre,
				"IdEstado":           idEstado,
				"Activo":             true,
			}, &creada))
		}
	}
	return errors.Join(errs...)
}

// Actualizar guarda los cambios de la hoja de vida.
//
// Las listas que no vienen en la peticion (nil) se conservan: la subida del CV
// solo manda archivoUrl. Una lista vacia si significa "borrar todo". El
// progreso se recalcula aqui porque la columna progreso_pct nadie la actualiza.
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

	// Para el progreso cuenta lo que quedara guardado, no solo lo que llego.
	anterior, err := s.Mia(ctx, idUsuario)
	if err != nil {
		return nil, err
	}
	final := *anterior
	fijar := map[string]any{"Resumen": resumen, "UrlPdf": archivo}
	if cambios.Educacion != nil {
		b, _ := json.Marshal(cambios.Educacion)
		fijar["Educacion"] = string(b)
		final.Educacion = cambios.Educacion
	}
	if cambios.Habilidades != nil {
		b, _ := json.Marshal(cambios.Habilidades)
		fijar["Habilidades"] = string(b)
		final.Habilidades = cambios.Habilidades
	}
	if cambios.Experiencias != nil {
		if err := s.guardarExperiencias(ctx, hv.Id, cambios.Experiencias); err != nil {
			return nil, err
		}
		final.Experiencias = cambios.Experiencias
	}
	if cambios.Certificaciones != nil {
		if err := s.guardarCertificaciones(ctx, id, cambios.Certificaciones); err != nil {
			return nil, err
		}
	}
	fijar["ProgresoPct"] = progresoDe(resumen, archivo, final)

	if err := s.empleo.Fijar(ctx, "hojas_vida/"+strconv.Itoa(hv.Id), fijar); err != nil {
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
