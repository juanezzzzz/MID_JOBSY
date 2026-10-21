package controllers

import (
	"net/http"

	"api_MID/models"
	"api_MID/services"
)

// CuentasController atiende el acceso: login, registro y recuperacion. Es publico.
type CuentasController struct {
	BaseController
}

// Login valida las credenciales y devuelve el token y el usuario.
// POST /api/usuarios/login.
func (c *CuentasController) Login() {
	var p models.PeticionLogin
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	res, err := svc.Cuentas.Login(c.Contexto(), p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, res)
}

// Registro crea una cuenta nueva y responde 201 con el usuario.
// POST /api/usuarios/registro.
func (c *CuentasController) Registro() {
	var p models.PeticionRegistro
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	u, err := svc.Cuentas.Registrar(c.Contexto(), p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, u)
}

// Recuperar inicia el reinicio de contrasena para el email dado. Responde 204
// siempre (no revela si el email existe, para no filtrar cuentas).
// POST /api/usuarios/recuperar.
func (c *CuentasController) Recuperar() {
	var p struct {
		Email string `json:"email"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	if err := svc.Cuentas.SolicitarReinicioPassword(c.Contexto(), p.Email); err != nil {
		c.Fallo(err)
		return
	}
	c.SinContenido()
}

// PerfilController atiende todo lo que el usuario gestiona de si mismo.
type PerfilController struct {
	SesionController
}

// Perfil devuelve los datos del usuario del token. GET /api/usuarios/perfil.
func (c *PerfilController) Perfil() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	u, err := svc.Cuentas.Perfil(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, u)
}

// ActualizarPerfil aplica cambios parciales al perfil (mapa de campos sueltos)
// y devuelve el usuario actualizado. PUT /api/usuarios/perfil.
func (c *PerfilController) ActualizarPerfil() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var cambios map[string]any
	if err := c.LeerJSON(&cambios); err != nil {
		c.Fallo(err)
		return
	}
	u, err := svc.Cuentas.ActualizarPerfil(c.Contexto(), id, cambios)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, u)
}

// SubirFoto recibe la imagen (multipart "foto"), la guarda y fija su url en el
// perfil. Responde 201 con {url}. POST /api/usuarios/perfil/foto.
func (c *PerfilController) SubirFoto() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	url, err := c.recibirArchivo("foto", "fotos")
	if err != nil {
		c.Fallo(err)
		return
	}
	// La url se guarda en el perfil para que persista entre sesiones. Antes,
	// en modo simulado, era un blob del navegador que moria al recargar.
	if _, err := svc.Cuentas.ActualizarPerfil(c.Contexto(), id, map[string]any{"fotoUrl": url}); err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, map[string]string{"url": url})
}

// CambiarPassword cambia la contrasena exigiendo la actual. Responde 204.
// PUT /api/usuarios/password.
func (c *PerfilController) CambiarPassword() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		Actual string `json:"passwordActual"`
		Nueva  string `json:"passwordNueva"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	if err := svc.Cuentas.CambiarPassword(c.Contexto(), id, p.Actual, p.Nueva); err != nil {
		c.Fallo(err)
		return
	}
	c.SinContenido()
}

// DarDeBaja desactiva la cuenta del usuario, confirmando con su contrasena.
// Responde 204. POST /api/usuarios/baja.
func (c *PerfilController) DarDeBaja() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct {
		Password string `json:"password"`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	if err := svc.Cuentas.DarDeBaja(c.Contexto(), id, p.Password); err != nil {
		c.Fallo(err)
		return
	}
	c.SinContenido()
}

// HojaVida devuelve la hoja de vida del usuario. GET /api/usuarios/hoja-de-vida.
func (c *PerfilController) HojaVida() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	hv, err := svc.HojasVida.Mia(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, hv)
}

// ActualizarHojaVida guarda la hoja de vida completa y devuelve la version
// resultante. PUT /api/usuarios/hoja-de-vida.
func (c *PerfilController) ActualizarHojaVida() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var hv models.HojaVida
	if err := c.LeerJSON(&hv); err != nil {
		c.Fallo(err)
		return
	}
	res, err := svc.HojasVida.Actualizar(c.Contexto(), id, hv)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, res)
}

// SubirCV recibe el archivo del CV (multipart "cv"), lo guarda y fija su url en
// la hoja de vida. Responde 201 con {url}. POST /api/usuarios/hoja-de-vida/archivo.
func (c *PerfilController) SubirCV() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	url, err := c.recibirArchivo("cv", "cv")
	if err != nil {
		c.Fallo(err)
		return
	}
	if _, err := svc.HojasVida.Actualizar(c.Contexto(), id, models.HojaVida{ArchivoURL: url}); err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, map[string]string{"url": url})
}

// MiDisponibilidad lista las franjas horarias en que el empleado esta
// disponible. GET /api/usuarios/disponibilidad.
func (c *PerfilController) MiDisponibilidad() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.PerfilEmpleado.Disponibilidad(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// FijarDisponibilidad reemplaza las franjas de disponibilidad del empleado por
// las recibidas. PUT /api/usuarios/disponibilidad.
func (c *PerfilController) FijarDisponibilidad() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var franjas []models.FranjaAgenda
	if err := c.LeerJSON(&franjas); err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.PerfilEmpleado.FijarDisponibilidad(c.Contexto(), id, franjas)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// MisDocumentos lista los documentos que el usuario ha subido.
// GET /api/usuarios/documentos.
func (c *PerfilController) MisDocumentos() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.PerfilEmpleado.Documentos(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// SubirDocumento recibe un archivo (multipart "documento") y su "tipo", lo
// guarda y lo registra. Responde 201. POST /api/usuarios/documentos.
func (c *PerfilController) SubirDocumento() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	url, err := c.recibirArchivo("documento", "documentos")
	if err != nil {
		c.Fallo(err)
		return
	}
	d, err := svc.PerfilEmpleado.SubirDocumento(c.Contexto(), id, c.Ctx.Request.FormValue("tipo"), url)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, d)
}

// PublicarResena crea una resena del usuario sobre otra persona de una
// postulacion compartida. Responde 201. POST /api/usuarios/resenas.
func (c *PerfilController) PublicarResena() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p services.PeticionResena
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	res, err := svc.Resenas.Publicar(c.Contexto(), id, p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, res)
}

// MisNotificaciones lista las notificaciones del usuario.
// GET /api/usuarios/notificaciones.
func (c *PerfilController) MisNotificaciones() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	items, err := svc.Notificaciones.Mias(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}

// MarcarNotificacionLeida marca como leida la notificacion :id del usuario.
// Responde 204. POST /api/usuarios/notificaciones/:id/leida.
func (c *PerfilController) MarcarNotificacionLeida() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	if err := svc.Notificaciones.MarcarLeida(c.Contexto(), id, c.ParamID()); err != nil {
		c.Fallo(err)
		return
	}
	c.SinContenido()
}

// MiConfiguracion devuelve las preferencias del usuario (notificaciones,
// privacidad, etc.). GET /api/usuarios/configuracion.
func (c *PerfilController) MiConfiguracion() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	dato, err := svc.Configuraciones.Mias(c.Contexto(), id)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, dato)
}

// ActualizarConfiguracion guarda las preferencias del usuario y devuelve la
// version resultante. PUT /api/usuarios/configuracion.
func (c *PerfilController) ActualizarConfiguracion() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var dato models.Configuracion
	if err := c.LeerJSON(&dato); err != nil {
		c.Fallo(err)
		return
	}
	res, err := svc.Configuraciones.Actualizar(c.Contexto(), id, dato)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, res)
}
