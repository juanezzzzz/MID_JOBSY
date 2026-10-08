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

func (c *BaseController) Entero(clave string, porDefecto int) int {
	n, err := strconv.Atoi(c.GetString(clave))
	if err != nil {
		return porDefecto
	}
	return n
}