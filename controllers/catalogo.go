package controllers

import (
	"errors"
	"net/http"
	"strings"

	"api_MID/services"

	beego "github.com/beego/beego/v2/server/web"
)

// CatalogosController expone las listas maestras (modalidades, estados,
// generos, turnos...) para que el front llene sus selects.
// Las categorias de la tienda no van aqui, esas las sirve el modulo tienda.
type CatalogosController struct {
	beego.Controller
}

// Listar responde GET /api/catalogo/{tabla}?contexto=...
func (c *CatalogosController) Listar() {
	tabla := strings.TrimPrefix(c.Ctx.Input.URL(), "/api/catalogo/")
	contexto := c.GetString("contexto")

	items, err := services.ListarCatalogo(tabla, contexto)
	if err != nil {
		c.responderError(err)
		return
	}
	c.Data["json"] = items
	c.ServeJSON()
}

func (c *CatalogosController) responderError(err error) {
	status := http.StatusInternalServerError
	codigo := "INTERNO"
	switch {
	case errors.Is(err, services.ErrTablaNoExiste), errors.Is(err, services.ErrNoEncontrado):
		status, codigo = http.StatusNotFound, "NO_ENCONTRADO"
	case errors.Is(err, services.ErrCatalogoCaido):
		status, codigo = http.StatusBadGateway, "UPSTREAM"
	}
	c.Ctx.Output.SetStatus(status)
	c.Data["json"] = map[string]interface{}{
		"error": map[string]string{"codigo": codigo, "mensaje": err.Error()},
	}
	c.ServeJSON()
}
