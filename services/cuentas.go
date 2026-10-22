package services

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// Cuentas implementa el alta, el acceso y el perfil de usuario.
//
// Todo esto vive aqui porque Jobsy_API no tiene nada de ello: no emite ni
// valida tokens, no hashea contrasenas y no consulta roles. Su tabla
// core.credenciales existe pero ningun codigo suyo la escribe.
type Cuentas struct {
	core   *ClienteUpstream
	auth   *ServicioAuth
	perfil *PerfilRegistro
}

// ConPerfil activa el guardado de los datos opcionales del registro
// (documento, perfil de empleo). Sin el, el alta solo crea la cuenta.
func (c *Cuentas) ConPerfil(p *PerfilRegistro) *Cuentas {
	c.perfil = p
	return c
}

// NuevoCuentas crea el servicio de cuentas sobre Core_api y el servicio de
// autenticacion.
func NuevoCuentas(core *ClienteUpstream, a *ServicioAuth) *Cuentas {
	return &Cuentas{core: core, auth: a}
}

type filaRol struct {
	Id     int    `json:"Id"`
	Nombre string `json:"Nombre"`
}

// filaUsuarioCompleto incluye el rol, que llega anidado por ser un rel(fk).
type filaUsuarioCompleto struct {
	Id              int      `json:"Id"`
	Nombre          string   `json:"Nombre"`
	Apellido        string   `json:"Apellido"`
	Correo          string   `json:"Correo"`
	Telefono        string   `json:"Telefono"`
	Ciudad          string   `json:"Ciudad"`
	Nacionalidad    string   `json:"Nacionalidad"`
	FotoUrl         string   `json:"FotoUrl"`
	IdRolesUsuarios *filaRol `json:"IdRolesUsuarios"`
	Disponible      bool     `json:"Disponible"`
	PerfilPublico   bool     `json:"PerfilPublico"`
	Activo          bool     `json:"Activo"`
	CreadoEn        string   `json:"CreadoEn"`
}

type filaCredencial struct {
	Id         int `json:"Id"`
	IdUsuarios *struct {
		Id int `json:"Id"`
	} `json:"IdUsuarios"`
	PasswordHash     string `json:"PasswordHash"`
	IntentosFallidos int    `json:"IntentosFallidos"`
	Bloqueado        bool   `json:"Bloqueado"`
	Activo           bool   `json:"Activo"`
}

// rolDesdeBase traduce el nombre del rol de la base al del frontend.
//
// La base usa 'administrador'; el front tipa 'admin'. Los otros dos coinciden.
func rolDesdeBase(nombre string) models.Rol {
	switch strings.ToLower(strings.TrimSpace(nombre)) {
	case "administrador", "admin":
		return models.RolAdmin
	case "empleador":
		return models.RolEmpleador
	default:
		return models.RolEmpleado
	}
}

// rolABase traduce el rol del dominio al nombre que usa la base (admin se
// guarda como "administrador").
func rolABase(r models.Rol) string {
	if r == models.RolAdmin {
		return "administrador"
	}
	return string(r)
}

// aDominio convierte la fila del usuario en el usuario que ve el front. Sin rol
// asignado se toma como empleado.
func (c *Cuentas) aDominio(f filaUsuarioCompleto) models.Usuario {
	rol := models.RolEmpleado
	if f.IdRolesUsuarios != nil {
		rol = rolDesdeBase(f.IdRolesUsuarios.Nombre)
	}
	return models.Usuario{
		ID:            strconv.Itoa(f.Id),
		Nombre:        f.Nombre,
		Apellido:      f.Apellido,
		Email:         f.Correo,
		Telefono:      f.Telefono,
		Rol:           rol,
		FotoURL:       f.FotoUrl,
		Verificado:    f.PerfilPublico,
		Ciudad:        f.Ciudad,
		FechaRegistro: f.CreadoEn,
	}
}

// porCorreo busca un usuario por su correo. Devuelve nil sin error cuando no
// existe, para que quien llama decida como tratarlo.
func (c *Cuentas) porCorreo(ctx context.Context, correo string) (*filaUsuarioCompleto, error) {
	var filas []filaUsuarioCompleto
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Correo": strings.TrimSpace(correo)}))
	o.Query.Set("limit", "1")
	if err := c.core.Get(ctx, "usuarios", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 {
		return nil, nil
	}
	return &filas[0], nil
}

// porID busca un usuario por id; devuelve NoEncontrado si no existe.
func (c *Cuentas) porID(ctx context.Context, id int) (*filaUsuarioCompleto, error) {
	var filas []filaUsuarioCompleto
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := c.core.Get(ctx, "usuarios", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 {
		return nil, models.NoEncontrado("ese usuario")
	}
	return &filas[0], nil
}

// credencialDe devuelve la credencial del usuario, o nil si todavia no tiene.
func (c *Cuentas) credencialDe(ctx context.Context, idUsuario int) (*filaCredencial, error) {
	var filas []filaCredencial
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	o.Query.Set("limit", "1")
	if err := c.core.Get(ctx, "credenciales", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 {
		return nil, nil
	}
	return &filas[0], nil
}

// Login valida las credenciales y emite el token.
func (c *Cuentas) Login(ctx context.Context, p models.PeticionLogin) (*models.RespuestaLogin, error) {
	if err := ValidarEmail(p.Usuario); err != nil {
		return nil, err
	}
	if p.Password == "" {
		return nil, models.Validacion("La contrasena es obligatoria.").
			ConCampo("password", "Obligatoria.")
	}

	usuario, err := c.porCorreo(ctx, p.Usuario)
	if err != nil {
		return nil, err
	}
	if usuario == nil || !usuario.Activo {
		// Se gasta el mismo tiempo que en una comparacion real para que la
		// latencia no revele si la cuenta existe.
		c.auth.HashFalso(p.Password)
		return nil, models.NoAutorizado("Correo o contrasena incorrectos.")
	}

	cred, err := c.credencialDe(ctx, usuario.Id)
	if err != nil {
		return nil, err
	}
	if cred == nil || !cred.Activo {
		c.auth.HashFalso(p.Password)
		return nil, models.NoAutorizado("Correo o contrasena incorrectos.")
	}
	if cred.Bloqueado {
		return nil, models.Prohibido("Tu cuenta esta bloqueada. Contacta a soporte.")
	}
	if err := c.auth.Verificar(cred.PasswordHash, p.Password); err != nil {
		return nil, err
	}

	u := c.aDominio(*usuario)
	token, _, err := c.auth.Emitir(u)
	if err != nil {
		return nil, err
	}
	return &models.RespuestaLogin{Token: token, Usuario: u}, nil
}

// Registrar da de alta un usuario y su credencial.
//
// Son dos escrituras en dos tablas y el backend no ofrece transacciones, asi
// que si la segunda falla se compensa borrando la primera. No es atomico: si
// la compensacion tambien falla queda un usuario sin credencial, y eso se
// registra para poder limpiarlo a mano.
func (c *Cuentas) Registrar(ctx context.Context, p models.PeticionRegistro) (*models.Usuario, error) {
	if err := ValidarEmail(p.Email); err != nil {
		return nil, err
	}
	if err := ValidarPassword(p.Password); err != nil {
		return nil, err
	}
	if err := validarRegistro(p); err != nil {
		return nil, err
	}
	if !p.Rol.Valido() {
		return nil, models.Validacion("Rol invalido.").ConCampo("rol", "Debe ser empleador o empleado.")
	}
	if p.Rol == models.RolAdmin {
		return nil, models.Prohibido("No puedes registrarte con ese rol.")
	}
	fechaNacimiento := "1990-01-01T00:00:00Z"
	if strings.TrimSpace(p.FechaNacimiento) != "" {
		f, err := ValidarFechaNacimiento(p.FechaNacimiento)
		if err != nil {
			return nil, err
		}
		fechaNacimiento = f
	}

	// El correo es unico en la base, pero el upstream devolveria un error
	// generico; comprobarlo antes permite dar un mensaje util.
	if existente, err := c.porCorreo(ctx, p.Email); err != nil {
		return nil, err
	} else if existente != nil {
		return nil, models.Conflicto("Ya existe una cuenta con ese correo.").
			ConCampo("email", "Ya registrado.")
	}

	if c.perfil != nil && strings.TrimSpace(p.Documento) != "" {
		if usado, err := c.perfil.DocumentoRegistrado(ctx, p.Documento); err != nil {
			return nil, err
		} else if usado {
			return nil, models.Conflicto("Ya existe una cuenta con ese numero de documento.").
				ConCampo("documento", "Ya registrado.")
		}
	}

	idRol, err := c.idRol(ctx, rolABase(p.Rol))
	if err != nil {
		return nil, err
	}

	hash, err := c.auth.Hash(p.Password)
	if err != nil {
		return nil, err
	}

	idGenero := 1
	if c.perfil != nil {
		if id, err := c.perfil.IDGenero(ctx, p.Genero); err == nil {
			idGenero = id
		}
	}

	// core.usuarios declara NOT NULL casi todo. Lo que el asistente no envio
	// se rellena con valores neutros; el usuario lo completa desde su perfil.
	nuevo := map[string]any{
		"Nombre":          strings.TrimSpace(p.Nombre),
		"Apellido":        strings.TrimSpace(p.Apellido),
		"Correo":          strings.TrimSpace(p.Email),
		"Telefono":        "",
		"Ciudad":          strings.TrimSpace(p.Ciudad),
		"FechaNacimiento": fechaNacimiento,
		"IdGenero":        idGenero,
		"Nacionalidad":    strings.TrimSpace(p.Nacionalidad),
		"FotoUrl":         "",
		"IdRolesUsuarios": map[string]any{"Id": idRol},
		"Disponible":      true,
		"PerfilPublico":   false,
		"Activo":          true,
	}

	var creado filaUsuarioCompleto
	if err := c.core.Post(ctx, "usuarios", nuevo, &creado); err != nil {
		return nil, err
	}
	if creado.Id == 0 {
		return nil, models.Upstream("core", fmt.Errorf("el alta no devolvio el id del usuario"))
	}

	cred := map[string]any{
		"IdUsuarios":       map[string]any{"Id": creado.Id},
		"PasswordHash":     hash,
		"DobleFactor":      false,
		"IntentosFallidos": 0,
		"Bloqueado":        false,
		"Activo":           true,
	}
	var credCreada map[string]any
	if err := c.core.Post(ctx, "credenciales", cred, &credCreada); err != nil {
		// Compensacion: sin credencial el usuario no podria entrar nunca y
		// ademas bloquearia el correo para un reintento.
		if errBorrado := c.core.Delete(ctx, "usuarios/"+strconv.Itoa(creado.Id)); errBorrado != nil {
			return nil, models.Interno(fmt.Errorf(
				"alta a medias: usuario %d creado sin credencial y no se pudo deshacer (%v); causa original: %w",
				creado.Id, errBorrado, err))
		}
		return nil, err
	}

	// La cuenta ya es valida: si falla el resto (documento, perfil de empleo)
	// no se deshace, se deja constancia y el usuario lo completa despues.
	if c.perfil != nil {
		if err := c.perfil.Completar(ctx, creado.Id, p); err != nil {
			slog.Warn("registro: datos del perfil sin guardar", "usuario", creado.Id, "error", err)
		}
	}

	// El alta devuelve el usuario tal como quedo, con su rol resuelto.
	completo, err := c.porID(ctx, creado.Id)
	if err != nil {
		u := c.aDominio(creado)
		u.Rol = p.Rol
		return &u, nil
	}
	u := c.aDominio(*completo)
	return &u, nil
}

// validarRegistro aplica las reglas a los datos del asistente. Los opcionales
// solo se validan si vienen.
func validarRegistro(p models.PeticionRegistro) error {
	errs := []error{
		ValidarNombrePersona("nombre", "El nombre", p.Nombre),
		ValidarNombrePersona("apellido", "El apellido", p.Apellido),
		ValidarTexto("barrio", "El barrio", p.Barrio, 0, 100),
		ValidarTexto("experiencia", "La experiencia", p.Experiencia, 0, 1000),
		ValidarTexto("nacionalidad", "La nacionalidad", p.Nacionalidad, 0, 60),
	}
	if strings.TrimSpace(p.Ciudad) != "" {
		errs = append(errs, ValidarCiudad("ciudad", p.Ciudad))
	}
	if strings.TrimSpace(p.Documento) != "" {
		n := normalizar(p.Nacionalidad)
		errs = append(errs, ValidarDocumento("documento", p.Documento, n == "" || n == "colombiana"))
	}
	if p.DistanciaKm != 0 {
		errs = append(errs, ValidarRango("distanciaKm", "La distancia", p.DistanciaKm, 1, DistanciaMax))
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// idRol resuelve el id de un rol por nombre (sin distinguir mayusculas).
func (c *Cuentas) idRol(ctx context.Context, nombre string) (int, error) {
	var roles []filaRol
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "50")
	if err := c.core.Get(ctx, "roles_usuarios", o, &roles); err != nil {
		return 0, err
	}
	for _, r := range roles {
		if strings.EqualFold(r.Nombre, nombre) {
			return r.Id, nil
		}
	}
	return 0, models.Upstream("core", fmt.Errorf("core.roles_usuarios no tiene el rol %q", nombre))
}

// SolicitarReinicioPassword marca la credencial con un token de reinicio.
//
// Responde igual exista o no la cuenta: decir "ese correo no esta registrado"
// permitiria enumerar usuarios. El envio del correo no se hace aqui porque no
// hay servicio de correo en el sistema; queda el token en la base.
func (c *Cuentas) SolicitarReinicioPassword(ctx context.Context, correo string) error {
	if err := ValidarEmail(correo); err != nil {
		return err
	}

	usuario, err := c.porCorreo(ctx, correo)
	if err != nil || usuario == nil {
		// Un fallo de upstream tampoco se distingue: silencio en ambos casos.
		return nil
	}

	cred, err := c.credencialDe(ctx, usuario.Id)
	if err != nil || cred == nil {
		return nil
	}

	token, _, err := c.auth.Emitir(models.Usuario{
		ID:    strconv.Itoa(usuario.Id),
		Email: usuario.Correo,
		Rol:   models.RolEmpleado,
	})
	if err != nil {
		return nil
	}

	_ = c.core.Fijar(ctx, "credenciales/"+strconv.Itoa(cred.Id), map[string]any{
		"TokenReset":    token,
		"TokenExpiraEn": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
	})
	return nil
}

// DarDeBaja desactiva la cuenta del usuario.
//
// La pantalla de configuracion tiene el boton y advierte de que la accion no
// se puede deshacer, pero no hacia nada.
//
// No se borra nada: las ofertas, contrataciones, pedidos y resenas cuelgan del
// usuario por clave foranea, y el DELETE de este backend es fisico, asi que
// borrarlo romperia el historial de las personas con las que trato. Se
// desactivan la cuenta y su credencial, con lo que deja de poder entrar y de
// aparecer en los listados. Un administrador puede revertirlo.
func (c *Cuentas) DarDeBaja(ctx context.Context, idUsuario, password string) error {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return err
	}
	if password == "" {
		return models.Validacion("Escribe tu contrasena para confirmar.").
			ConCampo("password", "Obligatoria.")
	}

	cred, err := c.credencialDe(ctx, id)
	if err != nil {
		return err
	}
	if cred == nil || !cred.Activo {
		return models.NoEncontrado("esa cuenta")
	}
	// Se exige la contrasena: es una accion destructiva y un equipo prestado
	// con la sesion abierta no debe bastar.
	if err := c.auth.Verificar(cred.PasswordHash, password); err != nil {
		return models.Validacion("La contrasena no es correcta.").
			ConCampo("password", "No coincide.")
	}

	if err := c.core.Fijar(ctx, "credenciales/"+strconv.Itoa(cred.Id), map[string]any{
		"Activo":     false,
		"TokenReset": "",
	}); err != nil {
		return err
	}
	if err := c.core.Fijar(ctx, "usuarios/"+strconv.Itoa(id), map[string]any{
		"Activo":        false,
		"Disponible":    false,
		"PerfilPublico": false,
	}); err != nil {
		// La credencial ya esta desactivada: no puede entrar, pero seguiria
		// saliendo en los listados. Se deshace para no dejarlo a medias.
		_ = c.core.Fijar(ctx, "credenciales/"+strconv.Itoa(cred.Id), map[string]any{"Activo": true})
		return err
	}
	return nil
}

// CambiarPassword sustituye la contrasena del usuario autenticado.
//
// La pantalla de configuracion tenia el formulario y su validacion, pero al
// enviarlo solo encendia el indicador de guardado: la contrasena no cambiaba.
//
// Exige la actual aunque haya sesion: un token robado, o un equipo que alguien
// dejo abierto, no debe bastar para quedarse con la cuenta.
func (c *Cuentas) CambiarPassword(ctx context.Context, idUsuario, actual, nueva string) error {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return err
	}
	if actual == "" {
		return models.Validacion("Escribe tu contrasena actual.").
			ConCampo("passwordActual", "Obligatoria.")
	}
	if err := ValidarPassword(nueva); err != nil {
		return err
	}
	if actual == nueva {
		return models.Validacion("La nueva contrasena debe ser distinta de la actual.").
			ConCampo("passwordNueva", "Debe ser distinta.")
	}

	cred, err := c.credencialDe(ctx, id)
	if err != nil {
		return err
	}
	if cred == nil || !cred.Activo {
		return models.NoEncontrado("esa cuenta")
	}
	if err := c.auth.Verificar(cred.PasswordHash, actual); err != nil {
		return models.Validacion("La contrasena actual no es correcta.").
			ConCampo("passwordActual", "No coincide.")
	}

	hash, err := c.auth.Hash(nueva)
	if err != nil {
		return err
	}
	// Se limpia el token de recuperacion: si habia uno pendiente, dejar de
	// aceptarlo es parte de cambiar la contrasena.
	return c.core.Fijar(ctx, "credenciales/"+strconv.Itoa(cred.Id), map[string]any{
		"PasswordHash":     hash,
		"TokenReset":       "",
		"IntentosFallidos": 0,
	})
}

// Perfil devuelve el usuario autenticado.
func (c *Cuentas) Perfil(ctx context.Context, idUsuario string) (*models.Usuario, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	f, err := c.porID(ctx, id)
	if err != nil {
		return nil, err
	}
	u := c.aDominio(*f)
	return &u, nil
}

// ActualizarPerfil aplica cambios parciales sobre el usuario.
//
// El PUT del backend reemplaza todas las columnas, asi que hay que leer,
// fusionar y reenviar el objeto completo: mandar solo los campos cambiados
// vaciaria el resto.
func (c *Cuentas) ActualizarPerfil(ctx context.Context, idUsuario string, cambios map[string]any) (*models.Usuario, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	actual, err := c.porID(ctx, id)
	if err != nil {
		return nil, err
	}

	texto := func(clave, porDefecto string) string {
		if v, ok := cambios[clave]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return porDefecto
	}
	viene := func(clave string) bool { _, ok := cambios[clave]; return ok }

	// Solo se valida lo que llega. Telefono y ciudad se pueden dejar vacios,
	// pero si se escriben tienen que ser validos.
	if viene("nombre") {
		if err := ValidarNombrePersona("nombre", "El nombre", texto("nombre", "")); err != nil {
			return nil, err
		}
	}
	if viene("apellido") {
		if err := ValidarNombrePersona("apellido", "El apellido", texto("apellido", "")); err != nil {
			return nil, err
		}
	}
	if tel := texto("telefono", ""); viene("telefono") && tel != "" {
		limpio, err := ValidarTelefono("telefono", tel)
		if err != nil {
			return nil, err
		}
		cambios["telefono"] = limpio
	}
	if ciudad := texto("ciudad", ""); viene("ciudad") && ciudad != "" {
		if err := ValidarCiudad("ciudad", ciudad); err != nil {
			return nil, err
		}
	}
	if err := ValidarTexto("nacionalidad", "La nacionalidad", texto("nacionalidad", ""), 0, 60); err != nil {
		return nil, err
	}

	// El correo no se cambia desde aqui: es la identidad de acceso y
	// cambiarlo sin verificacion permitiria secuestrar la cuenta.
	if err := c.core.Fijar(ctx, "usuarios/"+strconv.Itoa(id), map[string]any{
		"Nombre":       texto("nombre", actual.Nombre),
		"Apellido":     texto("apellido", actual.Apellido),
		"Telefono":     texto("telefono", actual.Telefono),
		"Ciudad":       texto("ciudad", actual.Ciudad),
		"Nacionalidad": texto("nacionalidad", actual.Nacionalidad),
		"FotoUrl":      texto("fotoUrl", actual.FotoUrl),
	}); err != nil {
		return nil, err
	}

	// La respuesta del PUT no siempre trae el rol resuelto; se relee.
	completo, err := c.porID(ctx, id)
	if err != nil {
		return nil, err
	}
	u := c.aDominio(*completo)
	return &u, nil
}
