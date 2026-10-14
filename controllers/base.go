package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/beego/beego/v2/core/logs"
 	beego "github.com/beego/beego/v2/server/web"
 	"api_MID/models"
)

type BaseController struct {
	beego.Controller
}

func (c *BaseController) Responder(status int, datos any) {
	c.Ctx.Output.SetStatus(status)
	c.Data["json"] = datos
	_ = c.ServeJSON()
}

func (c *BaseController) Fallo(err error) {
	var e *models.Error
	if !errors.As(err, &e){
		e = models.Interno(err)
	}
	if e.Status >= 500 {
		logs.Error("%s - > %v", c.Ctx.Input.Method(), c.Ctx.Input.URL(), e)
	}
	c.Responder(e.Status, map[string]any{"error": e})
}

func (c *BaseController) LeerJSON(destino any) error {
	dec := json.NewDecoder(bytes.NewReader(c.Ctx.Input.RequestBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		return models.Validacion("El cuerpo de la peticion no es valido: " + err.Error())
	}
	return nil
}

func (c *BaseController) Contexto() context.Context {
	return c.Ctx.Request.Context()
}

func (c *BaseController) ParamID() string {
	return c.Ctx.Input.Param(":id")
}

func (c *BaseController) Entero(clave string, porDefecto int) int {
	n, err := strconv.Atoi(c.GetString(clave))
	if err != nil {
		return porDefecto
	}
	return n
}
type SesionController struct {
	BaseController
}

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

func (c *SesionController) UsuarioActual() (string, error) {
	cl, ok := services.ClaimsDesdeCtx(c.Contexto())
	if !ok {
		return "", models.NoAutorizado("")
	}
	return cl.UsuarioID, nil
}

type AdminController struct {
	SesionController
}

func (c *AdminController) Prepare() {
	c.SesionController.Prepare()
	if err := services.ExigirRol(c.Contexto(), models.RolAdmin); err != nil {
		c.Fallo(err)
		c.StopRun()
	}
}

func escribirError(ctx *beecontext.Context, err error) {
	e := models.Desde(err)
	pid := PeticionID(ctx.Request.Context())


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

func nuevoCuerpoError(e *models.Error, pid string) cuerpoError {
	var cuerpo cuerpoError
	cuerpo.Error.Codigo = e.Codigo
	cuerpo.Error.Mensaje = e.Mensaje
	cuerpo.Error.Campos = e.Campos
	cuerpo.Error.PeticionID = pid
	return cuerpo
}

func errorHTTP(e *models.Error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(nuevoCuerpoError(e, PeticionID(r.Context())))
	}
}

func RegistrarErrores() {
	web.ErrorHandler("404", errorHTTP(models.NoEncontrado("esa ruta")))
	web.ErrorHandler("405", errorHTTP(&models.Error{
		Codigo:  models.CodValidacion,
		Mensaje: "Metodo no permitido.",
		Status:  http.StatusMethodNotAllowed,
	}))
	web.ErrorHandler("413", errorHTTP(models.Validacion("El cuerpo de la peticion es demasiado grande.")))
}

func esAbort(rec any) bool {
	err, ok := rec.(error)
	return ok && errors.Is(err, web.ErrAbort)
}