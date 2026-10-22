// Package controllers contiene la capa HTTP del middleware: un controlador de
// Beego por dominio, la escritura de respuestas y los filtros transversales.
//
// El acceso se decide por el tipo del controlador, no ruta por ruta:
//
//	BaseController     publico
//	SesionController   exige un token valido (Prepare)
//	AdminController    exige ademas el rol de administrador (Prepare)
//
// Asi una accion privada no puede quedar publica por olvidar un filtro: para
// exponerla habria que moverla a otro controlador.
package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/beego/beego/v2/server/web"
	beecontext "github.com/beego/beego/v2/server/web/context"

	"api_MID/models"
	"api_MID/services"
)

// Servicios agrupa las dependencias de los controladores.
//
// Beego crea un controlador nuevo en cada peticion, asi que las dependencias
// no pueden vivir en sus campos: se fijan una vez al arrancar con Configurar.
type Servicios struct {
	Cfg              *services.Config
	Auth             *services.ServicioAuth
	Cuentas          *services.Cuentas
	Empleados        *services.Empleados
	Ofertas          *services.Ofertas
	Postulaciones    *services.Postulaciones
	HojasVida        *services.HojasVida
	Tienda           *services.Tienda
	Pagos            *services.Pagos
	Monedero         *services.Monedero
	Contrataciones   *services.Contrataciones
	Resenas          *services.Resenas
	Notificaciones   *services.Notificaciones
	Configuraciones  *services.Configuraciones
	Almacen          services.Almacen
	Carrito          *services.Carrito
	MetodosGuardados *services.MetodosGuardados
	PerfilEmpleado   *services.PerfilEmpleado
	Suscripciones    *services.Suscripciones
	Soporte          *services.Soporte
	Admin            *services.Admin
}

var svc *Servicios

// Configurar deja listas las dependencias. Se llama desde main antes de
// web.Run.
func Configurar(s *Servicios) {
	svc = s
}

// cuerpoError es lo unico que sale hacia el cliente cuando algo falla.
//
// A diferencia de los upstream, aqui el status HTTP y el contenido coinciden,
// y no se filtra jamas el mensaje de PostgreSQL.
type cuerpoError struct {
	Error struct {
		Codigo     models.Codigo     `json:"codigo"`
		Mensaje    string            `json:"mensaje"`
		Campos     map[string]string `json:"campos,omitempty"`
		PeticionID string            `json:"peticionId,omitempty"`
	} `json:"error"`
}

// BaseController es el controlador publico: solo da utilidades de peticion
// y respuesta.
type BaseController struct {
	web.Controller
}

// Responder escribe una respuesta exitosa.
//
// El payload va sin envolver porque es lo que el frontend espera hoy: sus
// servicios tipan la respuesta como el objeto crudo, y ApiResponse<T> esta
// definido en api.model.ts pero no lo usa nadie.
func (c *BaseController) Responder(status int, v any) {
	c.Ctx.Output.Header("X-Content-Type-Options", "nosniff")
	c.Ctx.Output.SetStatus(status)
	c.Data["json"] = v
	if err := c.ServeJSON(); err != nil {
		// La cabecera ya salio; no se puede cambiar el status. Solo queda
		// dejar constancia.
		slog.Error("no se pudo serializar la respuesta", "error", err)
	}
}

// SinContenido responde 204 sin cuerpo, para operaciones que no devuelven dato.
func (c *BaseController) SinContenido() {
	c.Ctx.ResponseWriter.WriteHeader(http.StatusNoContent)
}

// Fallo traduce cualquier error al contrato del mid y lo registra.
func (c *BaseController) Fallo(err error) {
	escribirError(c.Ctx, err)
}

// LeerJSON deserializa el body rechazando campos desconocidos, para que un
// typo del cliente no pase silenciosamente. El limite de tamano lo pone
// Beego con maxmemory en app.conf.
func (c *BaseController) LeerJSON(destino any) error {
	dec := json.NewDecoder(bytes.NewReader(c.Ctx.Input.RequestBody))
	dec.DisallowUnknownFields()

	if err := dec.Decode(destino); err != nil {
		return models.Validacion("El cuerpo de la peticion no es JSON valido.").Con(err)
	}
	// Un segundo objeto pegado al primero es casi siempre un error del cliente.
	if dec.More() {
		return models.Validacion("El cuerpo debe contener un unico objeto JSON.")
	}
	return nil
}

// Contexto es el de la peticion: se cancela si el cliente se va.
func (c *BaseController) Contexto() context.Context {
	return c.Ctx.Request.Context()
}

// ParamID devuelve el :id de la ruta.
func (c *BaseController) ParamID() string {
	return c.Ctx.Input.Param(":id")
}

// Entero lee un parametro del query string como entero; si falta o no es
// numero devuelve 0 (los handlers luego acotan el valor).
func (c *BaseController) Entero(clave string) int {
	n, _ := strconv.Atoi(c.Ctx.Input.Query(clave))
	return n
}

// SesionController exige un token valido antes de cualquier accion.
type SesionController struct {
	BaseController
}

// Prepare valida el token y deja los claims en el contexto de la peticion.
func (c *SesionController) Prepare() {
	token, err := services.TokenDesdeCabecera(c.Ctx.Input.Header("Authorization"))
	if err != nil {
		c.Fallo(err)
		c.StopRun()
	}
	claims, err := svc.Auth.Validar(token)
	if err != nil {
		c.Fallo(err)
		c.StopRun()
	}
	c.Ctx.Request = c.Ctx.Request.WithContext(services.ConClaims(c.Ctx.Request.Context(), claims))
}

// UsuarioActual devuelve el id del usuario del token.
func (c *SesionController) UsuarioActual() (string, error) {
	cl, ok := services.ClaimsDesdeCtx(c.Contexto())
	if !ok {
		return "", models.NoAutorizado("")
	}
	return cl.UsuarioID, nil
}

// UsuarioConRol es UsuarioActual exigiendo ademas uno de los roles dados.
// Responde 403 si el token es valido pero el rol no corresponde.
func (c *SesionController) UsuarioConRol(roles ...models.Rol) (string, error) {
	id, err := c.UsuarioActual()
	if err != nil {
		return "", err
	}
	if err := services.ExigirRol(c.Contexto(), roles...); err != nil {
		return "", err
	}
	return id, nil
}

// AdminController exige ademas el rol de administrador.
//
// El guard del frontend lee el rol de localStorage y es manipulable; la
// decision real se toma aqui, contra el token firmado.
type AdminController struct {
	SesionController
}

// Prepare corre antes de cada accion admin: primero valida el token (reusando
// SesionController.Prepare) y luego exige el rol de administrador; si falta,
// corta con StopRun.
func (c *AdminController) Prepare() {
	c.SesionController.Prepare()
	if err := services.ExigirRol(c.Contexto(), models.RolAdmin); err != nil {
		c.Fallo(err)
		c.StopRun()
	}
}

// escribirError es comun a controladores, filtros y al recover de Beego, que
// no tienen controlador.
func escribirError(ctx *beecontext.Context, err error) {
	e := models.Desde(err)
	pid := PeticionID(ctx.Request.Context())

	// El 5xx es culpa nuestra o del upstream: se registra con la causa
	// completa. El 4xx es del cliente y no merece ruido en el log.
	if e.Status >= 500 {
		slog.Error("fallo al atender la peticion",
			"peticion_id", pid,
			"metodo", ctx.Request.Method,
			"ruta", ctx.Request.URL.Path,
			"codigo", e.Codigo,
			"causa", e.Causa(),
		)
	} else {
		slog.Debug("peticion rechazada",
			"peticion_id", pid,
			"metodo", ctx.Request.Method,
			"ruta", ctx.Request.URL.Path,
			"codigo", e.Codigo,
		)
	}

	ctx.Output.Header("X-Content-Type-Options", "nosniff")
	ctx.Output.SetStatus(e.Status)
	if err := ctx.Output.JSON(nuevoCuerpoError(e, pid), false, false); err != nil {
		slog.Error("no se pudo serializar el error", "error", err)
	}
}

// nuevoCuerpoError arma el JSON de error del contrato a partir de un
// models.Error y el id de peticion.
func nuevoCuerpoError(e *models.Error, pid string) cuerpoError {
	var cuerpo cuerpoError
	cuerpo.Error.Codigo = e.Codigo
	cuerpo.Error.Mensaje = e.Mensaje
	cuerpo.Error.Campos = e.Campos
	cuerpo.Error.PeticionID = pid
	return cuerpo
}

// errorHTTP adapta el contrato de errores a los manejadores de Beego (404,
// 405, 413...), que reciben un http.HandlerFunc y escriben ellos el status.
func errorHTTP(e *models.Error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(nuevoCuerpoError(e, PeticionID(r.Context())))
	}
}

// RegistrarErrores sustituye las paginas HTML de error de Beego por JSON con
// el contrato del mid.
func RegistrarErrores() {
	web.ErrorHandler("404", errorHTTP(models.NoEncontrado("esa ruta")))
	web.ErrorHandler("405", errorHTTP(&models.Error{
		Codigo:  models.CodValidacion,
		Mensaje: "Metodo no permitido.",
		Status:  http.StatusMethodNotAllowed,
	}))
	web.ErrorHandler("413", errorHTTP(models.Validacion("El cuerpo de la peticion es demasiado grande.")))
}

// esAbort distingue el corte normal de StopRun de un panico de verdad.
func esAbort(rec any) bool {
	err, ok := rec.(error)
	return ok && errors.Is(err, web.ErrAbort)
}
