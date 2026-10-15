package services

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api_MID/models"

	beego "github.com/beego/beego/v2/server/web"
)

// responderData escribe el sobre que usan los CRUD de Jobsy_API.
func responderData(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"success":true,"status":200,"message":"ok","data":`+data+`}`)
}

// catalogoDeReportes hace de Catalogo_api con los tipos y estados de reporte.
func catalogoDeReportes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tipos":
			responderData(w, `[{"Id":40,"Contexto":"reporte","Nombre":"tecnico","Activo":true},{"Id":41,"Contexto":"reporte","Nombre":"pago","Activo":true}]`)
		case "/v1/estados":
			responderData(w, `[{"Id":50,"Contexto":"reporte","Nombre":"abierto","Activo":true},{"Id":53,"Contexto":"reporte","Nombre":"cerrado","Activo":true}]`)
		default:
			responderData(w, `[]`)
		}
	}))
	t.Cleanup(srv.Close)
	beego.AppConfig.Set("url_catalogo", srv.URL)
	// El catalogo guarda las tablas en memoria; se vacia para no arrastrar
	// datos de otra prueba.
	cacheMu.Lock()
	cache = map[string]tablaEnCache{}
	cacheMu.Unlock()
}

// soporteFalso hace de Soporte_API. El reporte 7 es del usuario 3 y esta
// abierto; el 8 es del usuario 3 y esta cerrado; el 9 es de otro usuario.
// Guarda el ultimo cuerpo que recibio por POST.
func soporteFalso(t *testing.T, ultimoPost *map[string]any) *Soporte {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			json.NewDecoder(r.Body).Decode(ultimoPost)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/reportes":
			responderData(w, `{"Id":12,"IdUsuarios":3,"IdTipo":40,"Descripcion":"No carga la app","IdEstado":50,"Activo":true,"CreadoEn":"2026-10-14T10:00:00Z"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/reporte_mensajes":
			responderData(w, `{"Id":90,"IdRemitente":3,"Mensaje":"Sigue igual","Activo":true,"CreadoEn":"2026-10-14T11:00:00Z"}`)
		case r.URL.Path == "/v1/reportes":
			responderData(w, `[{"Id":7,"IdUsuarios":3,"IdTipo":41,"Descripcion":"Me cobraron dos veces","IdEstado":50,"Activo":true,"CreadoEn":"2026-10-13T09:00:00Z"}]`)
		case r.URL.Path == "/v1/reportes/7":
			responderData(w, `{"Id":7,"IdUsuarios":3,"IdTipo":41,"IdEstado":50,"Activo":true}`)
		case r.URL.Path == "/v1/reportes/8":
			responderData(w, `{"Id":8,"IdUsuarios":3,"IdTipo":40,"IdEstado":53,"Activo":true}`)
		case r.URL.Path == "/v1/reportes/9":
			responderData(w, `{"Id":9,"IdUsuarios":4,"IdTipo":40,"IdEstado":50,"Activo":true}`)
		case r.URL.Path == "/v1/reporte_mensajes":
			responderData(w, `[{"Id":80,"IdRemitente":3,"Mensaje":"Hola","Activo":true},{"Id":81,"IdRemitente":1,"Mensaje":"Lo revisamos","Activo":true}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return NuevoSoporte(NuevoClienteAPI("soporte", srv.URL, 5*time.Second))
}

// codigo devuelve el codigo de un *models.Error, o "" si no lo es.
func codigo(err error) models.Codigo {
	var e *models.Error
	if errors.As(err, &e) {
		return e.Codigo
	}
	return ""
}

func TestMisReportesTraduceTipoYEstado(t *testing.T) {
	catalogoDeReportes(t)
	s := soporteFalso(t, &map[string]any{})

	reportes, err := s.MisReportes("3")
	if err != nil {
		t.Fatal(err)
	}
	if len(reportes) != 1 {
		t.Fatalf("quiero 1 reporte, llegaron %d", len(reportes))
	}
	r := reportes[0]
	if r.ID != "7" || r.Tipo != "pago" || r.Estado != "abierto" || r.ReportanteID != "3" {
		t.Fatalf("reporte = %+v", r)
	}
}

func TestCrearReporteArrancaAbierto(t *testing.T) {
	catalogoDeReportes(t)
	var enviado map[string]any
	s := soporteFalso(t, &enviado)

	r, err := s.CrearReporte("3", PeticionReporte{Tipo: "tecnico", Descripcion: "  No carga la app  "})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "12" || r.Estado != "abierto" {
		t.Fatalf("reporte = %+v", r)
	}
	// encoding/json decodifica los numeros como float64.
	if enviado["IdTipo"] != 40.0 || enviado["IdEstado"] != 50.0 || enviado["Descripcion"] != "No carga la app" {
		t.Fatalf("se envio a Soporte_API: %v", enviado)
	}
}

func TestCrearReporteValidaLaEntrada(t *testing.T) {
	catalogoDeReportes(t)
	s := soporteFalso(t, &map[string]any{})

	casos := map[string]PeticionReporte{
		"sin tipo":        {Descripcion: "algo"},
		"sin descripcion": {Tipo: "tecnico", Descripcion: "   "},
		"tipo inventado":  {Tipo: "queja", Descripcion: "algo"},
		"muy larga":       {Tipo: "tecnico", Descripcion: strings.Repeat("a", maxDescripcionReporte+1)},
	}
	for nombre, p := range casos {
		if _, err := s.CrearReporte("3", p); codigo(err) != models.CodValidacion {
			t.Errorf("%s: quiero un error de validacion, llego %v", nombre, err)
		}
	}
}

func TestMensajesMarcaLosPropios(t *testing.T) {
	catalogoDeReportes(t)
	s := soporteFalso(t, &map[string]any{})

	mensajes, err := s.Mensajes("3", "7")
	if err != nil {
		t.Fatal(err)
	}
	if len(mensajes) != 2 || !mensajes[0].EsMio || mensajes[1].EsMio {
		t.Fatalf("mensajes = %+v", mensajes)
	}
}

func TestReporteDeOtroUsuarioEs404(t *testing.T) {
	catalogoDeReportes(t)
	s := soporteFalso(t, &map[string]any{})

	if _, err := s.Mensajes("3", "9"); codigo(err) != models.CodNoEncontrado {
		t.Fatalf("quiero 404 para un reporte ajeno, llego %v", err)
	}
}

func TestReporteCerradoNoAceptaMensajes(t *testing.T) {
	catalogoDeReportes(t)
	s := soporteFalso(t, &map[string]any{})

	if _, err := s.Responder("3", "8", "hola"); codigo(err) != models.CodConflicto {
		t.Fatalf("quiero un conflicto en un reporte cerrado, llego %v", err)
	}
	m, err := s.Responder("3", "7", "Sigue igual")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "90" || !m.EsMio {
		t.Fatalf("mensaje = %+v", m)
	}
}
