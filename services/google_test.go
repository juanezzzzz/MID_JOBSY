package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Un token con otra audiencia o sin correo verificado no debe dar sesion.
func TestLoginGoogleRechazaTokenAjeno(t *testing.T) {
	casos := map[string]string{
		"otra audiencia":       `{"aud":"otra-app","iss":"accounts.google.com","email":"a@b.co","email_verified":"true"}`,
		"correo sin verificar": `{"aud":"mi-app","iss":"accounts.google.com","email":"a@b.co","email_verified":"false"}`,
		"emisor falso":         `{"aud":"mi-app","iss":"evil.com","email":"a@b.co","email_verified":"true"}`,
	}
	for nombre, cuerpo := range casos {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(cuerpo)) }))
		urlTokenInfo = srv.URL
		_, err := (&Cuentas{}).LoginGoogle(context.Background(), "mi-app", "tok")
		srv.Close()
		if err == nil {
			t.Errorf("%s: se acepto un token invalido", nombre)
		}
	}
	if _, err := (&Cuentas{}).LoginGoogle(context.Background(), "", "tok"); err == nil {
		t.Error("sin client id configurado no debe permitir el login")
	}
}
