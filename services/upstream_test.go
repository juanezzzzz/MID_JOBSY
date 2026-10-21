package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"api_MID/models"
)

// servidorQueResponde levanta un upstream falso que devuelve siempre lo mismo.
func servidorQueResponde(t *testing.T, status int, cuerpo string) *ClienteUpstream {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(cuerpo))
	}))
	t.Cleanup(srv.Close)
	return NuevoClienteUpstream("prueba", srv.URL, 5*time.Second)
}

type carga struct {
	Nombre string `json:"Nombre"`
}

// Los upstream de Empleo, Suscripciones, Usuarios y Soporte escriben "date"
// en vez de "data" en sus POST y PUT. Son 56 sitios; el mid lee ambas.
func TestLeeLaClaveDateAdemasDeData(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"clave data", `{"success":true,"status":200,"message":"ok","data":{"Nombre":"Laura"}}`},
		{"clave date (typo)", `{"success":true,"status":201,"message":"ok","date":{"Nombre":"Laura"}}`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cli := servidorQueResponde(t, http.StatusOK, c.cuerpo)
			var got carga
			if err := cli.Get(context.Background(), "lo-que-sea", nil, &got); err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			if got.Nombre != "Laura" {
				t.Fatalf("Nombre = %q, esperaba %q", got.Nombre, "Laura")
			}
		})
	}
}

// El status real viaja dentro del body: el HTTP es 200 aunque el body diga 404.
// Enrutar por codigo HTTP daria un falso exito.
func TestNoConfiaEnElStatusHTTP(t *testing.T) {
	cuerpo := `{"success":false,"status":404,"message":"no encontrado"}`
	cli := servidorQueResponde(t, http.StatusOK, cuerpo)

	var got carga
	err := cli.Get(context.Background(), "lo-que-sea", nil, &got)
	if err == nil {
		t.Fatal("esperaba error pese al HTTP 200")
	}
	e := models.Desde(err)
	if e.Codigo != models.CodNoEncontrado {
		t.Fatalf("codigo = %q, esperaba %q", e.Codigo, models.CodNoEncontrado)
	}
	if e.Status != http.StatusNotFound {
		t.Fatalf("status = %d, esperaba 404", e.Status)
	}
}

// Cuando GetAll falla, el upstream devuelve el error crudo de PostgreSQL como
// string JSON pelado. No debe llegar al cliente: lleva nombres de tabla.
func TestNoFiltraElErrorCrudoDePostgres(t *testing.T) {
	crudo := `"pq: relation \"empleo.ofertas\" does not exist"`
	cli := servidorQueResponde(t, http.StatusOK, crudo)

	var got []carga
	err := cli.Get(context.Background(), "ofertas", nil, &got)
	if err == nil {
		t.Fatal("esperaba error")
	}
	e := models.Desde(err)
	if e.Codigo != models.CodUpstream {
		t.Fatalf("codigo = %q, esperaba %q", e.Codigo, models.CodUpstream)
	}
	// El mensaje al cliente es generico...
	if contieneTexto(e.Mensaje, "pq:") || contieneTexto(e.Mensaje, "empleo.ofertas") {
		t.Fatalf("el mensaje al cliente filtra detalles internos: %q", e.Mensaje)
	}
	// ...pero la causa conserva el detalle para el log.
	if e.Causa() == nil || !contieneTexto(e.Causa().Error(), "empleo.ofertas") {
		t.Fatalf("la causa deberia conservar el detalle, es: %v", e.Causa())
	}
}

// Core_api responde 200 con body vacio cuando el Unmarshal del POST falla,
// porque al if del error le falta la rama else. Para el cliente eso seria
// indistinguible de un exito.
func TestRespuestaVaciaEsError(t *testing.T) {
	cli := servidorQueResponde(t, http.StatusOK, "")

	var got carga
	err := cli.Post(context.Background(), "usuarios", map[string]string{"Nombre": "x"}, &got)
	if err == nil {
		t.Fatal("un 200 con body vacio no puede tomarse por exito")
	}
	if models.Desde(err).Codigo != models.CodUpstream {
		t.Fatalf("codigo = %q", models.Desde(err).Codigo)
	}
}

// Un listado sin filas llega como success:true con data vacia, y eso es cero
// resultados, no un fallo.
func TestListadoVacioNoEsError(t *testing.T) {
	for _, cuerpo := range []string{
		`{"success":true,"status":200,"message":"Consulta exitosa","data":[]}`,
		`{"success":true,"status":200,"message":"Consulta exitosa","data":null}`,
	} {
		cli := servidorQueResponde(t, http.StatusOK, cuerpo)
		var got []carga
		if err := cli.Get(context.Background(), "usuarios", nil, &got); err != nil {
			t.Fatalf("un listado vacio no deberia ser error (%s): %v", cuerpo, err)
		}
		if len(got) != 0 {
			t.Fatalf("esperaba cero filas, obtuve %d", len(got))
		}
	}
}

// Un GetAll con success:false es un fallo aunque el destino sea una lista.
// Antes el middleware lo trataba como lista vacia, y una base caida se veia
// como un listado sin resultados.
func TestFalloDeListadoEsError(t *testing.T) {
	cuerpo := `{"success":false,"status":500,"message":"Error en el servicio GetAll","data":null}`
	cli := servidorQueResponde(t, http.StatusOK, cuerpo)

	var got []carga
	err := cli.Get(context.Background(), "ofertas", nil, &got)
	if err == nil {
		t.Fatal("un GetAll fallido no puede tomarse por lista vacia")
	}
	if models.Desde(err).Codigo != models.CodUpstream {
		t.Fatalf("codigo = %q, esperaba %q", models.Desde(err).Codigo, models.CodUpstream)
	}
}

// Pero si el destino es un objeto unico, "no hay filas" si significa que el
// recurso no existe, y eso debe seguir siendo un error.
func TestRecursoUnicoSinResultadosSiEsError(t *testing.T) {
	cuerpo := `{"success":false,"status":404,"message":"no se encontro el ID","data":null}`
	cli := servidorQueResponde(t, http.StatusOK, cuerpo)

	var got carga
	err := cli.Get(context.Background(), "usuarios/9999", nil, &got)
	if err == nil {
		t.Fatal("esperaba error para un recurso inexistente")
	}
	if models.Desde(err).Codigo != models.CodNoEncontrado {
		t.Fatalf("codigo = %q, esperaba %q", models.Desde(err).Codigo, models.CodNoEncontrado)
	}
}

// Si una de las funciones falla, EnParalelo devuelve ese error.
func TestEnParaleloDevuelveElPrimerError(t *testing.T) {
	falla := errors.New("fallo")
	err := EnParalelo(
		func() error { return nil },
		func() error { return falla },
		func() error { return nil },
	)
	if !errors.Is(err, falla) {
		t.Fatalf("err = %v, esperaba %v", err, falla)
	}
}

// Si ninguna falla, EnParalelo devuelve nil.
func TestEnParaleloSinErroresDevuelveNil(t *testing.T) {
	if err := EnParalelo(func() error { return nil }, func() error { return nil }); err != nil {
		t.Fatalf("esperaba nil, obtuve %v", err)
	}
}

// QueryBeego arma el filtro campo:valor y con un mapa vacio da cadena vacia.
func TestQueryBeegoUneLosPares(t *testing.T) {
	q := QueryBeego(map[string]string{"IdUsuarios": "7"})
	if q != "IdUsuarios:7" {
		t.Fatalf("q = %q", q)
	}
	if QueryBeego(nil) != "" {
		t.Fatal("un mapa vacio deberia dar cadena vacia")
	}
}

// contieneTexto indica si sub aparece en s (sub vacio cuenta como no).
func contieneTexto(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

// indexOf devuelve la posicion de sub en s, o -1 si no aparece.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Parchear tiene que reenviar las columnas que no toca, y reducir las
// relaciones anidadas a su Id.
func TestParchearConservaLoQueNoToca(t *testing.T) {
	var recibido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"success":true,"status":200,"data":{"Id":4,"Stock":10,"PrecioOriginal":12000,"IdTipo":3,"IdCategoriasTienda":{"Id":1,"Nombre":"Aseo"},"ActualizadoEn":"2026-01-01T00:00:00Z"}}`))
		case http.MethodPut:
			cuerpo, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(cuerpo, &recibido)
			_, _ = w.Write([]byte(`{"success":true,"status":200,"data":{}}`))
		}
	}))
	t.Cleanup(srv.Close)
	cli := NuevoClienteUpstream("prueba", srv.URL, 5*time.Second)

	if err := cli.Fijar(context.Background(), "productos/4", map[string]any{"Stock": 7}); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if recibido["Stock"] != float64(7) {
		t.Fatalf("Stock = %v, esperaba 7", recibido["Stock"])
	}
	if recibido["PrecioOriginal"] != float64(12000) || recibido["IdTipo"] != float64(3) {
		t.Fatalf("se perdieron columnas no tocadas: %v", recibido)
	}
	cat, _ := recibido["IdCategoriasTienda"].(map[string]any)
	if len(cat) != 1 || cat["Id"] != float64(1) {
		t.Fatalf("la relacion deberia reducirse a su Id, es %v", recibido["IdCategoriasTienda"])
	}
	if recibido["ActualizadoEn"] == "2026-01-01T00:00:00Z" {
		t.Fatal("ActualizadoEn deberia renovarse")
	}
}
