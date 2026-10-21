package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	beego "github.com/beego/beego/v2/server/web"

	"api_MID/controllers"
	"api_MID/models"
	_ "api_MID/routers"
	"api_MID/services"
)

var auth *services.ServicioAuth

// init arranca Beego con el app.conf real y unas dependencias minimas: lo que
// se prueba aqui es la capa HTTP (rutas, acceso, contrato de errores), no la
// logica de negocio, que tiene sus propios tests en services.
func init() {
	// app.conf lee el secreto del entorno al parsearse.
	os.Setenv("MID_JWT_SECRETO", strings.Repeat("s", 32))

	_, file, _, _ := runtime.Caller(0)
	apppath, _ := filepath.Abs(filepath.Dir(filepath.Join(file, ".."+string(filepath.Separator))))
	beego.TestBeegoInit(apppath)

	cfg, err := services.CargarConfig()
	if err != nil {
		panic(err)
	}

	// Un upstream que siempre falla, para comprobar que eso sale como 502.
	caido := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	tienda := services.NuevoClienteUpstream("tienda", caido.URL, cfg.TimeoutUpstream)

	auth = services.NuevoServicioAuth(cfg.JWTSecreto, cfg.JWTDuracion)
	controllers.Configurar(&controllers.Servicios{
		Cfg:      cfg,
		Auth:     auth,
		Monedero: services.NuevoMonedero(tienda),
		// Pagos queda nil a proposito: su ruta sirve para probar la
		// recuperacion de panicos.
	})

	// web.Run es quien monta la cadena de filtros; aqui no se llama.
	beego.BeeApp.Handlers.Init()
}

// token emite un JWT de prueba para un usuario con el rol dado.
func token(t *testing.T, rol models.Rol) string {
	t.Helper()
	tk, _, err := auth.Emitir(models.Usuario{ID: "7", Email: "prueba@jobsy.co", Rol: rol})
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

// pedir lanza una peticion contra la app Beego y devuelve la respuesta grabada.
func pedir(t *testing.T, metodo, ruta, cuerpo string, cabeceras map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	if cuerpo != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range cabeceras {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	beego.BeeApp.Handlers.ServeHTTP(w, r)
	return w
}

// codigoError lee el campo error.codigo del contrato de errores.
func codigoError(t *testing.T, w *httptest.ResponseRecorder) models.Codigo {
	t.Helper()
	var cuerpo struct {
		Error struct {
			Codigo     models.Codigo `json:"codigo"`
			PeticionID string        `json:"peticionId"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("el error no es JSON: %v\n%s", err, w.Body.String())
	}
	if cuerpo.Error.PeticionID == "" {
		t.Errorf("el error no trae peticionId: %s", w.Body.String())
	}
	return cuerpo.Error.Codigo
}

// La ruta de salud responde 200 y trae X-Request-Id.
func TestSalud(t *testing.T) {
	w := pedir(t, "GET", "/api/salud", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperaba 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"estado"`) {
		t.Errorf("cuerpo inesperado: %s", w.Body.String())
	}
	if w.Header().Get("X-Request-Id") == "" {
		t.Error("falta X-Request-Id")
	}
}

// Una ruta privada sin token responde 401 con el codigo de no autorizado.
func TestRutaPrivadaSinTokenEs401(t *testing.T) {
	w := pedir(t, "GET", "/api/tienda/saldo", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperaba 401: %s", w.Code, w.Body.String())
	}
	if c := codigoError(t, w); c != models.CodNoAutorizado {
		t.Errorf("codigo = %s", c)
	}
}

// El rol se decide contra el token firmado, no contra lo que diga el front.
func TestPanelAdminConRolEmpleadoEs403(t *testing.T) {
	w := pedir(t, "GET", "/api/tienda/admin/estadisticas", "", map[string]string{
		"Authorization": "Bearer " + token(t, models.RolEmpleado),
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, esperaba 403: %s", w.Code, w.Body.String())
	}
	if c := codigoError(t, w); c != models.CodProhibido {
		t.Errorf("codigo = %s", c)
	}
}

// Con token valido la peticion llega al servicio; un upstream caido sale como
// 502 y no como el error crudo.
func TestUpstreamCaidoEs502(t *testing.T) {
	w := pedir(t, "GET", "/api/tienda/saldo", "", map[string]string{
		"Authorization": "Bearer " + token(t, models.RolEmpleado),
	})
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, esperaba 502: %s", w.Code, w.Body.String())
	}
	if c := codigoError(t, w); c != models.CodUpstream {
		t.Errorf("codigo = %s", c)
	}
}

// Un panico en un handler se convierte en un 500 con el formato de error.
func TestPanicoEs500ConElContrato(t *testing.T) {
	w := pedir(t, "GET", "/api/tienda/pagos/mios", "", map[string]string{
		"Authorization": "Bearer " + token(t, models.RolEmpleado),
	})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, esperaba 500: %s", w.Code, w.Body.String())
	}
	if c := codigoError(t, w); c != models.CodInterno {
		t.Errorf("codigo = %s", c)
	}
}

// Un cuerpo con campos que no existen se rechaza con 400.
func TestCampoDesconocidoEs400(t *testing.T) {
	w := pedir(t, "POST", "/api/usuarios/login", `{"usuario":"a","clave":"b","otro":1}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperaba 400: %s", w.Code, w.Body.String())
	}
	if c := codigoError(t, w); c != models.CodValidacion {
		t.Errorf("codigo = %s", c)
	}
}

// Una ruta que no existe responde 404 en JSON, no la pagina de Beego.
func TestRutaInexistenteEs404JSON(t *testing.T) {
	w := pedir(t, "GET", "/api/no-existe", "", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperaba 404: %s", w.Code, w.Body.String())
	}
	if c := codigoError(t, w); c != models.CodNoEncontrado {
		t.Errorf("codigo = %s", c)
	}
}

// Sin secreto configurado la notificacion de la pasarela no se acepta.
func TestNotificacionSinSecretoEs501(t *testing.T) {
	w := pedir(t, "POST", "/api/tienda/pagos/notificacion", `{"pagoId":"1","estado":"aprobado"}`, nil)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, esperaba 501: %s", w.Code, w.Body.String())
	}
}

// CORS solo devuelve Allow-Origin a los origenes de la lista blanca.
func TestCORSSoloOrigenesPermitidos(t *testing.T) {
	w := pedir(t, "OPTIONS", "/api/salud", "", map[string]string{"Origin": "http://localhost:4200"})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, esperaba 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:4200" {
		t.Errorf("Allow-Origin = %q", got)
	}

	w = pedir(t, "OPTIONS", "/api/salud", "", map[string]string{"Origin": "https://malicioso.example"})
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("un origen no permitido recibio Allow-Origin = %q", got)
	}
}
