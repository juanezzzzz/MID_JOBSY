// Package services contiene la logica de negocio que Jobsy_API no tiene.
//
// Los upstream son CRUD tabla por tabla: saben insertar una fila en
// postulaciones, pero no saben "postularse". Todo lo que sea validar, calcular,
// agregar o secuenciar vive aqui.
package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"api_MID/models"
)

// Empleados resuelve la vista de empleado que pinta el frontend.
//
// El front tiene un tipo Employee plano con 11 campos. En PostgreSQL eso son
// tres tablas de dos schemas distintos:
//
//	core.usuarios              nombre, foto, ciudad, verificado
//	empleo.perfiles_empleado   bio, rating, total_trabajos, disponible
//	empleo.TR_perfil_especialidades + empleo.especialidades   las etiquetas
//
// Este es el caso canonico de agregacion: una peticion del front, varias
// lecturas en paralelo, una sola respuesta.
type Empleados struct {
	core     *ClienteUpstream
	empleo   *ClienteUpstream
	catalogo *Catalogo
}

// NuevoEmpleados crea el servicio de empleados sobre Core_api (usuarios),
// Empleo_API (perfiles) y el catalogo.
func NuevoEmpleados(core, empleo *ClienteUpstream, cat *Catalogo) *Empleados {
	return &Empleados{core: core, empleo: empleo, catalogo: cat}
}

// filaUsuario refleja core.usuarios tal como lo serializa Beego: PascalCase,
// porque los modelos de Jobsy_API no llevan tags json.
//
// El identificador llega como "Id", no como "IdUsuarios": Beego serializa por
// el nombre del campo Go, y en ese modelo la clave primaria se llama Id
// aunque su columna sea id_usuarios. Lo mismo vale para filtrar y ordenar:
// query=Id:7 funciona, query=IdUsuarios:7 hace reventar el servicio con un
// 500. Ojo, no es universal: perfiles_empleado si tiene un campo IdUsuarios,
// porque ahi es una clave foranea y no la primaria.
// filaRolAnidado es el rol tal como lo devuelve Core: un objeto anidado,
// porque el campo esta declarado con rel(fk).
type filaRolAnidado struct {
	Id     int    `json:"Id"`
	Nombre string `json:"Nombre"`
}

type filaUsuario struct {
	Id              int             `json:"Id"`
	IdRolesUsuarios *filaRolAnidado `json:"IdRolesUsuarios"`
	Nombre          string          `json:"Nombre"`
	Apellido        string          `json:"Apellido"`
	Correo          string          `json:"Correo"`
	Telefono        string          `json:"Telefono"`
	Ciudad          string          `json:"Ciudad"`
	FotoUrl         string          `json:"FotoUrl"`
	Disponible      bool            `json:"Disponible"`
	Activo          bool            `json:"Activo"`
	CreadoEn        string          `json:"CreadoEn"`
	Nacionalidad    string          `json:"Nacionalidad"`
}

// filaPerfil refleja empleo.perfiles_empleado.
//
// La clave primaria llega como "Id" (el campo Go se llama asi). IdUsuarios si
// existe con ese nombre porque ahi es una clave foranea.
type filaPerfil struct {
	Id                int     `json:"Id"`
	IdUsuarios        int     `json:"IdUsuarios"`
	Bio               string  `json:"Bio"`
	AniosExperiencia  int     `json:"AniosExperiencia"`
	TarifaHora        float64 `json:"TarifaHora"`
	RatingPromedio    float64 `json:"RatingPromedio"`
	TotalResenas      int     `json:"TotalResenas"`
	TotalTrabajos     int     `json:"TotalTrabajos"`
	Verificado        bool    `json:"Verificado"`
	AparecerBusquedas bool    `json:"AparecerBusquedas"`
	Activo            bool    `json:"Activo"`
	BarrioZona        string  `json:"BarrioZona"`
}

type filaEspecialidad struct {
	Id     int    `json:"Id"`
	Nombre string `json:"Nombre"`
}

// refAnidada es una clave foranea tal como la serializa Beego cuando el campo
// esta declarado con rel(fk): no llega el entero sino el objeto entero, con
// solo la clave primaria poblada y el resto en valores cero.
type refAnidada struct {
	Id int `json:"Id"`
}

type filaPerfilEspecialidad struct {
	IdPerfilesEmpleado refAnidada `json:"IdPerfilesEmpleado"`
	IdEspecialidades   refAnidada `json:"IdEspecialidades"`
}

// PorID devuelve un empleado completo a partir del id de usuario.
//
// Hace tres lecturas: usuario y perfil pueden ir en paralelo porque son
// independientes; las especialidades necesitan el id del perfil, asi que van
// despues. No hay forma de evitar ese segundo salto con un backend CRUD.
func (s *Empleados) PorID(ctx context.Context, idUsuario string) (*models.Empleado, error) {
	id, err := strconv.Atoi(strings.TrimSpace(idUsuario))
	if err != nil || id <= 0 {
		return nil, models.Validacion("Identificador de empleado invalido.")
	}

	var (
		usuario  filaUsuario
		perfiles []filaPerfil
	)

	// Leemos por filtro en vez de por /:id porque la ruta de GetAll no llama
	// a LoadRelated, que es donde estaban los panicos del backend.
	err = EnParalelo(
		func() error {
			var filas []filaUsuario
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
			o.Query.Set("limit", "1")
			if err := s.core.Get(ctx, "usuarios", o, &filas); err != nil {
				return err
			}
			if len(filas) == 0 {
				return models.NoEncontrado("ese empleado")
			}
			usuario = filas[0]
			return nil
		},
		func() error {
			o := &OpcionesUpstream{Query: map[string][]string{}}
			o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(id)}))
			o.Query.Set("limit", "1")
			return s.empleo.Get(ctx, "perfiles_empleado", o, &perfiles)
		},
	)
	if err != nil {
		return nil, err
	}

	if !usuario.Activo {
		// Un usuario dado de baja no debe distinguirse de uno inexistente.
		return nil, models.NoEncontrado("ese empleado")
	}

	emp := &models.Empleado{
		ID:             strconv.Itoa(usuario.Id),
		UsuarioID:      strconv.Itoa(usuario.Id),
		Nombre:         strings.TrimSpace(usuario.Nombre + " " + usuario.Apellido),
		FotoURL:        usuario.FotoUrl,
		Ciudad:         usuario.Ciudad,
		Disponible:     usuario.Disponible,
		Especialidades: []string{},
	}

	// Un usuario con rol empleado puede no tener perfil todavia. Devolvemos lo
	// que hay en vez de fallar: el front pinta la ficha igual.
	if len(perfiles) > 0 {
		p := perfiles[0]
		emp.Descripcion = p.Bio
		emp.Calificacion = p.RatingPromedio
		emp.TotalServicios = p.TotalTrabajos
		emp.Verificado = p.Verificado
		emp.Experiencia = textoExperiencia(p.AniosExperiencia)
		emp.TarifaHora = p.TarifaHora

		esp, err := s.especialidadesDe(ctx, p.Id)
		if err != nil {
			// Las especialidades son decorativas: si fallan, preferimos
			// devolver la ficha sin ellas antes que un 502.
			esp = []string{}
		}
		emp.Especialidades = esp
	}

	return emp, nil
}

// especialidadesDe resuelve la relacion N:M en dos saltos, que es lo unico que
// permite un backend sin joins: primero los ids, luego los nombres.
func (s *Empleados) especialidadesDe(ctx context.Context, idPerfil int) ([]string, error) {
	var enlaces []filaPerfilEspecialidad
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdPerfilesEmpleado": strconv.Itoa(idPerfil)}))
	o.Query.Set("limit", "100")
	if err := s.empleo.Get(ctx, "tr_perfil_especialidades", o, &enlaces); err != nil {
		return nil, err
	}
	if len(enlaces) == 0 {
		return []string{}, nil
	}

	// El catalogo de especialidades es pequeno: una sola lectura y se cruza en
	// memoria. Pedir una por id seria N+1.
	var todas []filaEspecialidad
	oc := &OpcionesUpstream{Query: map[string][]string{}}
	oc.Query.Set("limit", "500")
	if err := s.empleo.Get(ctx, "especialidades", oc, &todas); err != nil {
		return nil, err
	}

	porID := make(map[int]string, len(todas))
	for _, e := range todas {
		porID[e.Id] = e.Nombre
	}

	nombres := make([]string, 0, len(enlaces))
	for _, e := range enlaces {
		if n, ok := porID[e.IdEspecialidades.Id]; ok {
			nombres = append(nombres, n)
		}
	}
	return nombres, nil
}

// Listar devuelve empleados paginados.
//
// Nota sobre el total: los upstream devuelven un array pelado, sin cuenta
// total, asi que no se puede construir "pagina 3 de 47" de forma honesta.
// Pedimos un elemento de mas para saber con certeza si hay pagina siguiente, y
// reportamos Total como una cota inferior. Es menos de lo que el front espera,
// pero es lo unico verdadero que se puede decir.
func (s *Empleados) Listar(ctx context.Context, busqueda string, pagina, tamano int) (models.Pagina[models.Empleado], error) {
	var vacia models.Pagina[models.Empleado]

	if pagina < 1 {
		pagina = 1
	}
	if tamano < 1 || tamano > 100 {
		tamano = 20
	}

	// Solo usuarios con rol de empleado: el buscador de empleados devolvia
	// tambien a los empleadores, que no ofrecen servicios y no tienen perfil.
	idRolEmpleado, err := s.idRolEmpleado(ctx)
	if err != nil {
		return vacia, err
	}

	filtros := map[string]string{
		"Activo":          "true",
		"IdRolesUsuarios": strconv.Itoa(idRolEmpleado),
	}
	if b := strings.TrimSpace(busqueda); b != "" {
		// Beego traduce query=Campo:valor a un LIKE por defecto en strings.
		filtros["Nombre"] = b
	}

	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(filtros))
	// Se leen todos los usuarios que cumplen el filtro (una sola lectura,
	// barata) para conocer el total real; el perfil, que es lo costoso, solo
	// se cruza para la pagina pedida. Antes el total era el de la pagina.
	o.Query.Set("limit", "5000")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")

	var todas []filaUsuario
	if err := s.core.Get(ctx, "usuarios", o, &todas); err != nil {
		return vacia, err
	}
	total := len(todas)
	desde := min((pagina-1)*tamano, total)
	hasta := min(desde+tamano, total)
	filas := todas[desde:hasta]

	// Enriquecer cada usuario con su perfil seria N+1 contra el upstream. Se
	// hace una sola lectura de perfiles y se cruza en memoria.
	perfilPorUsuario, err := s.perfilesDe(ctx, filas)
	if err != nil {
		return vacia, err
	}

	items := make([]models.Empleado, 0, len(filas))
	for _, u := range filas {
		e := models.Empleado{
			ID:             strconv.Itoa(u.Id),
			UsuarioID:      strconv.Itoa(u.Id),
			Nombre:         strings.TrimSpace(u.Nombre + " " + u.Apellido),
			FotoURL:        u.FotoUrl,
			Ciudad:         u.Ciudad,
			Disponible:     u.Disponible,
			Especialidades: []string{},
		}
		if p, ok := perfilPorUsuario[u.Id]; ok {
			e.Descripcion = p.Bio
			e.Calificacion = p.RatingPromedio
			e.TotalServicios = p.TotalTrabajos
			e.Verificado = p.Verificado
			e.Experiencia = textoExperiencia(p.AniosExperiencia)
			e.TarifaHora = p.TarifaHora
		}
		items = append(items, e)
	}

	return models.NuevaPagina(items, pagina, tamano, total, hasta < total), nil
}

// idRolEmpleado resuelve el id del rol "empleado" en core.roles_usuarios.
func (s *Empleados) idRolEmpleado(ctx context.Context) (int, error) {
	var roles []struct {
		Id     int    `json:"Id"`
		Nombre string `json:"Nombre"`
	}
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "50")
	if err := s.core.Get(ctx, "roles_usuarios", o, &roles); err != nil {
		return 0, err
	}
	for _, r := range roles {
		if strings.EqualFold(strings.TrimSpace(r.Nombre), "empleado") {
			return r.Id, nil
		}
	}
	return 0, models.Upstream("core", errSinRolEmpleado)
}

type erroresEmpleados string

// Error implementa la interfaz error.
func (e erroresEmpleados) Error() string { return string(e) }

const errSinRolEmpleado = erroresEmpleados("core.roles_usuarios no tiene el rol empleado")

// perfilesDe devuelve los perfiles activos de los usuarios dados, indexados por
// id de usuario.
func (s *Empleados) perfilesDe(ctx context.Context, usuarios []filaUsuario) (map[int]filaPerfil, error) {
	if len(usuarios) == 0 {
		return map[int]filaPerfil{}, nil
	}
	// El CRUD de Beego no admite "IN (...)", solo igualdad por campo. Traemos
	// la pagina de perfiles y cruzamos en memoria; con paginas de 20 es mejor
	// que 20 peticiones.
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true"}))
	o.Query.Set("limit", strconv.Itoa(len(usuarios)*4))

	var perfiles []filaPerfil
	if err := s.empleo.Get(ctx, "perfiles_empleado", o, &perfiles); err != nil {
		return nil, err
	}

	quiero := make(map[int]bool, len(usuarios))
	for _, u := range usuarios {
		quiero[u.Id] = true
	}
	out := make(map[int]filaPerfil, len(usuarios))
	for _, p := range perfiles {
		if quiero[p.IdUsuarios] {
			out[p.IdUsuarios] = p
		}
	}
	return out, nil
}

// textoExperiencia redacta los anos de experiencia para mostrarlos en el front.
func textoExperiencia(anios int) string {
	switch {
	case anios <= 0:
		return "Sin experiencia registrada"
	case anios == 1:
		return "1 ano de experiencia"
	default:
		return fmt.Sprintf("%d anos de experiencia", anios)
	}
}
