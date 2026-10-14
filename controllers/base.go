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

type Servicios struct{
	Cfg *Services.Config
}

var svc *Servicios

func Configurar(s *Servicios) {
	svc = s
}

type cuerpoError struct {
	Error struct {
		Codigo models.Codigo `json:"codigo"`
		Mensaje string `json:"mensaje"`
		Campos map[string]string `json:"campos,omitempty"`
		PeticionID string `json:"peticionId,omitempty`
	}`json:"error"`
}

type BaseController struct { 
	web.Controller
}

func (c *BaseController) Responder(status int, v any) {
	c.Ctx.Output.Header("X-Content-Type-Options", "nosniff")
	c.Ctx.Output.SetStatus(status)
	c.Data["json"] = v
	if err := c.ServeJSON(); err != nil {
		slog.Error("no se pudo serializar la respuesta", "error", err)
	}
}

func (c *BaseController) sinContenido() {
	c.Ctx.ResponseWriter.WriteHeader(http.StatusNoContent)
}

func (c *BaseController) fallo(err error) {
	escribirError(c.Ctx, err)
}

func (c *BaseController) LeerJSON(destino any) error {
	dec := json.NewDecoder(bytes.NewReader(c.Ctx.Input.RequestBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		return models.Validacion("El cuerpo de la peticion no es JSON valido").Con(err)
	}
	if dec.More(){
		return models.Validacion("El cuerpo debe contener un unico objeto JSON.")
	}
	return nil
}

func (c *BaseController) contexto() context.Context {
	return c.Ctx.Request.Context()
}

func (c *BaseController) ParamID() string {
	return c.Ctx.Input.Param(":id")
}

func (c *BaseController) Entero(clave string) int {
	n, _ := strconv.Atoi(c.Ctx.Input.Query(clave))
	return n
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
		slog.Error("no se pudo resializar el erro", "error", err)
	}
}

func nuevoCuerpoError(e *models.Error, pid string) cuerpoError {
	var cuerpo cuerpoError
	cuerpo.Error.Codigo = e.Codigo
	cuerpo.Error.Mensaje = e.Mensaje
	cuerpo.Error.Campos = e.Campos
	cuerpo.Error.PeticionID = pid
	return  cuerpo
}

func errorHTTP(e *models.Error) http.HandlerFunc{
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(nuevoCuerpoError(e, PeticionID(r.Context())))
	}
}

func RegistrarErrores () {
	web.ErrorHandler("404", errorHTTP(models.NoEncontrado("esa ruta")))
	web.ErrorHandler("405", errorHTTP(&models.Error{
		Codigo: models.CodValidacion,
		Mensaje: "Metodo no permitido.",
		Status: http.StatusMethodNotAllowed,
	}))
	web.ErrorHandler("413", errorHTTP(models.Validacion("El cuerpo de la peticion es demasiado grande.")))
}

func esAbort(rec any) bool {
	err, ok := rec.(error)
	return ok && errors.Is(err, web.ErrAbort)
}