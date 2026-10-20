package controllers 

import (
	"net/http"
)

type SuscripcionesController struct{
	SesionController
}

// Mi suscripcion devuelve la suscripcion activa del usuario del token o null
// si no tiene ninguna. GET/api/suscripciones/mi-suscripcion.
func (c *SuscripcionesController) MiSuscripcion(){
	id, err := c.UsuarioActual()
	if err != nil{
		c.Fallo(err)
		return
	}
	s, err := svc.Suscripciones.Mia(c.Contexto(), id)
	if err != nil{
		c.Fallo(err)
		return
	}
	// el frontend tipa subscripcion | null: sins suscripcion se devuelve null
	// no un 404.
	c.Responder(http.StatusOK, s)
}

//Susccribir de alta al usuario en el plan de indicado en el cuerpo (planID) y
// responde 201 con la suscripcion creada. POST/api/suscripciones/escribir


func (c *SuscripcionesController) Suscribir(){
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var p struct{
		PlanID string `json:"planId"`
	}
	if err := c.LeerJSON(&p); err != nil{
		c.fallo(err)
		return
	}
	s, err := svc.Suscripciones.Suscribir(c.Contexto(), id p.PlanID)
	if err != nil{
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, s)
}