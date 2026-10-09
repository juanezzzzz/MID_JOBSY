package controllers

import "net/http"

type SaludController struct {
	BaseController
}

func (c *SaludController) Get(){
	c.Responder(http.StatusOK, map[string]string{"estado": "ok"})
}