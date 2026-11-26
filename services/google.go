package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"api_MID/models"
)

// urlTokenInfo es el endpoint de Google que valida un ID token y devuelve sus
// claims. Variable para poder apuntarlo a un servidor falso en pruebas.
var urlTokenInfo = "https://oauth2.googleapis.com/tokeninfo"

// LoginGoogle entra con el ID token que entrega Google Identity Services.
// Solo vale para cuentas Jobsy ya registradas con ese correo: el registro
// pide rol, documento y ubicacion, que Google no da.
func (c *Cuentas) LoginGoogle(ctx context.Context, clientID, credencial string) (*models.RespuestaLogin, error) {
	if clientID == "" {
		return nil, models.NoImplementado("El inicio de sesion con Google no esta configurado.")
	}
	if strings.TrimSpace(credencial) == "" {
		return nil, models.Validacion("Falta la credencial de Google.")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, urlTokenInfo+"?id_token="+url.QueryEscape(credencial), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, models.Upstream("google", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, models.NoAutorizado("Google no valido la sesion. Intentalo de nuevo.")
	}
	var claims struct {
		Aud           string `json:"aud"`
		Iss           string `json:"iss"`
		Email         string `json:"email"`
		EmailVerified string `json:"email_verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, models.Upstream("google", fmt.Errorf("tokeninfo ilegible: %w", err))
	}
	// El token debe ser para esta app y de un correo que Google comprobo.
	if claims.Aud != clientID || (claims.Iss != "accounts.google.com" && claims.Iss != "https://accounts.google.com") || claims.EmailVerified != "true" {
		return nil, models.NoAutorizado("La sesion de Google no es valida para Jobsy.")
	}

	usuario, err := c.porCorreo(ctx, claims.Email)
	if err != nil {
		return nil, err
	}
	if usuario == nil || !usuario.Activo {
		return nil, models.NoAutorizado("No hay una cuenta Jobsy con ese correo de Google. Registrate primero.")
	}
	u := c.aDominio(*usuario)
	token, _, err := c.auth.Emitir(u)
	if err != nil {
		return nil, err
	}
	return &models.RespuestaLogin{Token: token, Usuario: u}, nil
}
