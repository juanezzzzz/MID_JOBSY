package controllers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"api_MID/models"
	"api_MID/services"
)

// PasarelaController recibe las notificaciones del proveedor de pago.
//
// No lleva sesion: quien llama es el proveedor, no una persona. Lo unico que
// la distingue de la peticion de un desconocido es la firma, asi que sin
// secreto configurado la ruta no atiende.
type PasarelaController struct {
	BaseController
}

// Notificacion atiende la confirmacion asincrona del proveedor.
//
// El cuerpo se lee crudo y se verifica ANTES de interpretarlo: se firma lo que
// llego, byte a byte, no una version reserializada. Beego ya lo copio en
// RequestBody (copyrequestbody en app.conf), con el limite de maxmemory.
func (c *PasarelaController) Notificacion() {
	cuerpo := c.Ctx.Input.RequestBody

	firma := c.Ctx.Input.Header("X-Jobsy-Firma")
	if err := services.VerificarFirma(firma, cuerpo, svc.Cfg.SecretoPasarela, time.Now()); err != nil {
		if errors.Is(err, services.ErrSinSecreto) {
			// Sin clave no se puede comprobar nada. Se dice que no esta
			// implementado en vez de aceptar a ciegas.
			c.Fallo(models.NoImplementado(
				"La verificacion de notificaciones no esta configurada."))
			return
		}
		// Al que llama no se le explica que fallo.
		slog.Warn("notificacion de pasarela rechazada",
			"remoto", c.Ctx.Request.RemoteAddr, "motivo", err)
		c.Fallo(models.NoAutorizado("Firma invalida."))
		return
	}

	var n struct {
		PagoID string `json:"pagoId"`
		Estado string `json:"estado"`
	}
	if err := json.Unmarshal(cuerpo, &n); err != nil {
		c.Fallo(models.Validacion("La notificacion no es JSON valido."))
		return
	}
	if n.PagoID == "" || (n.Estado != "aprobado" && n.Estado != "rechazado") {
		c.Fallo(models.Validacion(
			"La notificacion debe traer pagoId y un estado aprobado o rechazado."))
		return
	}

	res, err := svc.Pagos.Confirmar(c.Contexto(), n.PagoID, n.Estado == "aprobado")
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, res)
}
