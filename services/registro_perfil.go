package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// PerfilRegistro guarda lo que el asistente de registro pide ademas de la
// cuenta: documento de identidad y, para empleados, el perfil de empleo con
// su modalidad, experiencia y especialidades.
//
// Sin esto un empleado recien registrado no tenia fila en
// empleo.perfiles_empleado y todo lo que la busca (disponibilidad, ficha en el
// buscador) respondia 404.
type PerfilRegistro struct {
	core, empleo, catalogo *ClienteUpstream
	cat                    *Catalogo
}

// NuevoPerfilRegistro crea el servicio sobre Core, Empleo y Catalogo.
func NuevoPerfilRegistro(core, empleo, catalogo *ClienteUpstream, cat *Catalogo) *PerfilRegistro {
	return &PerfilRegistro{core: core, empleo: empleo, catalogo: catalogo, cat: cat}
}

type filaNombre struct {
	Id     int    `json:"Id"`
	Nombre string `json:"Nombre"`
}

var sinTildes = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "ü", "u")

// normalizar compara nombres de catalogo sin mayusculas, tildes ni espacios
// sobrantes: el front escribe "Cuidado de ninos" y la base "Cuidado de niños".
func normalizar(s string) string {
	return strings.Join(strings.Fields(sinTildes.Replace(strings.ToLower(s))), " ")
}

// idPorNombre busca en un listado de catalogo el id cuyo nombre coincide.
func idPorNombre(ctx context.Context, cliente *ClienteUpstream, recurso, nombre string) (int, error) {
	var filas []filaNombre
	if err := cliente.Get(ctx, recurso, sinLimite(), &filas); err != nil {
		return 0, err
	}
	for _, f := range filas {
		if normalizar(f.Nombre) == normalizar(nombre) {
			return f.Id, nil
		}
	}
	return 0, models.Upstream(cliente.Servicio(), fmt.Errorf("%s no tiene la fila %q; hay que sembrarla", recurso, nombre))
}

// IDGenero traduce el genero del front al catalogo, que solo tiene Masculino,
// Femenino y Otro: "No binario" y "Prefiero no decirlo" van a Otro.
func (s *PerfilRegistro) IDGenero(ctx context.Context, genero string) (int, error) {
	nombre := "Otro"
	switch normalizar(genero) {
	case "femenino":
		nombre = "Femenino"
	case "masculino":
		nombre = "Masculino"
	}
	return idPorNombre(ctx, s.catalogo, "generos", nombre)
}

// ValidarFechaNacimiento acepta YYYY-MM-DD en el pasado y la devuelve en el
// formato que espera el backend.
func ValidarFechaNacimiento(fecha string) (string, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(fecha))
	if err != nil || !t.Before(time.Now()) {
		return "", models.Validacion("La fecha de nacimiento no es valida.").ConCampo("fechaNacimiento", "Usa el formato AAAA-MM-DD.")
	}
	return t.Format(time.RFC3339), nil
}

var primerNumero = regexp.MustCompile(`\d+`)

// nivelExperiencia deduce del texto libre del asistente ("6 anos en hogares
// de Yopal") los años y el nivel del catalogo.
func nivelExperiencia(texto string) (string, int) {
	anios := 0
	if m := primerNumero.FindString(texto); m != "" {
		anios, _ = strconv.Atoi(m)
	}
	switch {
	case anios >= 5:
		return "experto", anios
	case anios >= 1 || strings.TrimSpace(texto) != "":
		return "intermedio", anios
	default:
		return "sin_experiencia", anios
	}
}

// idModalidad toma la primera modalidad elegida que tenga equivalente en la
// base. "presencial" no existe alli (ver Catalogo.IDModalidad); si es la unica
// elegida se guarda como tiempo_completo, que es lo que el front describe
// ("Contrato estable en un hogar").
func (s *PerfilRegistro) idModalidad(ctx context.Context, elegidas []models.Modalidad) (int, error) {
	for _, m := range elegidas {
		if id, err := s.cat.IDModalidad(ctx, m); err == nil {
			return id, nil
		}
	}
	return s.cat.IDModalidad(ctx, models.ModalidadJornadaCompl)
}

// Completar guarda los datos opcionales del registro. Cada parte es
// independiente: un fallo no deshace la cuenta, que ya existe y es valida, y
// se devuelve para dejar constancia en el log.
func (s *PerfilRegistro) Completar(ctx context.Context, idUsuario int, p models.PeticionRegistro) error {
	var errs []error
	if strings.TrimSpace(p.Documento) != "" {
		errs = append(errs, s.documento(ctx, idUsuario, p))
	}
	if p.Rol == models.RolEmpleado {
		errs = append(errs, s.perfilEmpleo(ctx, idUsuario, p))
	}
	return errors.Join(errs...)
}

// DocumentoRegistrado indica si el numero ya pertenece a otra cuenta. El numero
// es unico en la base: sin esta comprobacion la cuenta se creaba igual y el
// documento se descartaba, dejando dos cuentas para la misma persona.
func (s *PerfilRegistro) DocumentoRegistrado(ctx context.Context, numero string) (bool, error) {
	var filas []refAnidada
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"NumeroDocumento": strings.TrimSpace(numero)}))
	o.Query.Set("limit", "1")
	if err := s.core.Get(ctx, "documentos_identidad", o, &filas); err != nil {
		return false, err
	}
	return len(filas) > 0, nil
}

func (s *PerfilRegistro) documento(ctx context.Context, idUsuario int, p models.PeticionRegistro) error {
	tipo := "pasaporte"
	if n := normalizar(p.Nacionalidad); n == "" || n == "colombiana" {
		tipo = "cedula"
	}
	idTipo, err := idPorNombre(ctx, s.core, "tipo_documento", tipo)
	if err != nil {
		return err
	}
	var creado map[string]any
	return s.core.Post(ctx, "documentos_identidad", map[string]any{
		"IdUsuarios":      map[string]any{"Id": idUsuario},
		"IdTipoDocumento": map[string]any{"Id": idTipo},
		"NumeroDocumento": strings.TrimSpace(p.Documento),
		"Activo":          true,
	}, &creado)
}

func (s *PerfilRegistro) perfilEmpleo(ctx context.Context, idUsuario int, p models.PeticionRegistro) error {
	nivel, anios := nivelExperiencia(p.Experiencia)
	idNivel, err := idPorNombre(ctx, s.catalogo, "niveles_experiencia", nivel)
	if err != nil {
		return err
	}
	idModalidad, err := s.idModalidad(ctx, p.Modalidades)
	if err != nil {
		return err
	}
	idTurno, err := idPorNombre(ctx, s.catalogo, "turnos", "completo")
	if err != nil {
		return err
	}

	fila := map[string]any{
		"IdUsuarios":         idUsuario,
		"IdNivelExperiencia": idNivel,
		"AniosExperiencia":   anios,
		"TarifaNegociable":   true,
		"IdModalidadTrabajo": idModalidad,
		"IdTurnoPreferido":   idTurno,
		"HvVisible":          true,
		"AparecerBusquedas":  true,
		"Activo":             true,
	}
	if bio := strings.TrimSpace(p.Experiencia); bio != "" {
		fila["Bio"] = bio
	}
	if p.DistanciaKm > 0 {
		fila["DistanciaMaxKm"] = int(p.DistanciaKm)
	}
	if b := strings.TrimSpace(p.Barrio); b != "" {
		fila["BarrioZona"] = b
	}
	var perfil struct {
		Id int `json:"Id"`
	}
	if err := s.empleo.Post(ctx, "perfiles_empleado", fila, &perfil); err != nil {
		return err
	}
	if perfil.Id == 0 {
		return models.Upstream("empleo", fmt.Errorf("el alta del perfil no devolvio su id"))
	}
	return s.especialidades(ctx, perfil.Id, p.Especialidades)
}

// especialidades enlaza las elegidas con el catalogo de empleo. Se acepta que
// el nombre del front sea el comienzo del de la base ("Cocina" -> "Cocina y
// alimentacion"); las que no existen se informan.
func (s *PerfilRegistro) especialidades(ctx context.Context, idPerfil int, elegidas []string) error {
	if len(elegidas) == 0 {
		return nil
	}
	var catalogo []filaNombre
	if err := s.empleo.Get(ctx, "especialidades", sinLimite(), &catalogo); err != nil {
		return err
	}
	var errs []error
	for _, e := range elegidas {
		id := 0
		for _, c := range catalogo {
			if normalizar(c.Nombre) == normalizar(e) {
				id = c.Id
				break
			}
			if id == 0 && strings.HasPrefix(normalizar(c.Nombre), normalizar(e)+" ") {
				id = c.Id
			}
		}
		if id == 0 {
			errs = append(errs, fmt.Errorf("especialidad %q no existe en empleo.especialidades", e))
			continue
		}
		var enlace map[string]any
		errs = append(errs, s.empleo.Post(ctx, "tr_perfil_especialidades", map[string]any{
			"IdPerfilesEmpleado": map[string]any{"Id": idPerfil},
			"IdEspecialidades":   map[string]any{"Id": id},
			"Activo":             true,
		}, &enlace))
	}
	return errors.Join(errs...)
}
