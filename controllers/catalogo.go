package controllers

import (
	"net/http"
	"strings"

	"api_MID/services"
)

// CatalogosController expone las listas maestras (modalidades, estados,
// generos, turnos...) para que el front llene sus selects.
// Las categorias de la tienda no van aqui, esas las sirve el modulo tienda.
type CatalogosController struct {
	BaseController
}

// Listar responde GET /api/catalogo/{tabla}?contexto=...
func (c *CatalogosController) Listar() {
	tabla := strings.TrimPrefix(c.Ctx.Input.URL(), "/api/catalogo/")
	contexto := c.GetString("contexto")

	items, err := services.ListarCatalogo(tabla, contexto)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, items)
}
