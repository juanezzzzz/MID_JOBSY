package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api_MID/services"

	beego "github.com/beego/beego/v2/server/web"
	. "github.com/smartystreets/goconvey/convey"
)

// catalogoFalso hace de Catalogo_api para no depender de que este corriendo.
func catalogoFalso() *httptest.Server {
	filas := map[string]string{
		"/v1/modalidades_trabajo": `[{"Id":1,"Nombre":"por_horas","Activo":true},{"Id":2,"Nombre":"tiempo_completo","Activo":true},{"Id":3,"Nombre":"fines_semana","Activo":true}]`,
		"/v1/estados":             `[{"Id":10,"Contexto":"oferta","Nombre":"activa","Activo":true},{"Id":11,"Contexto":"pedido","Nombre":"activa","Activo":true}]`,
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := filas[r.URL.Path]
		if !ok {
			data = "[]"
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"status":200,"message":"Consulta exitosa","data":` + data + `}`))
	}))
}

func TestCatalogo(t *testing.T) {
	srv := catalogoFalso()
	defer srv.Close()
	beego.AppConfig.Set("url_catalogo", srv.URL)

	Convey("Subject: modulo catalogo\n", t, func() {
		Convey("GET /api/catalogo/modalidades devuelve la lista", func() {
			r, _ := http.NewRequest("GET", "/api/catalogo/modalidades", nil)
			w := httptest.NewRecorder()
			beego.BeeApp.Handlers.ServeHTTP(w, r)

			var items []map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &items)
			So(w.Code, ShouldEqual, 200)
			So(len(items), ShouldEqual, 3)
			So(items[0]["nombre"], ShouldEqual, "por_horas")
		})

		Convey("Filtra los estados por contexto", func() {
			r, _ := http.NewRequest("GET", "/api/catalogo/estados?contexto=pedido", nil)
			w := httptest.NewRecorder()
			beego.BeeApp.Handlers.ServeHTTP(w, r)

			var items []map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &items)
			So(w.Code, ShouldEqual, 200)
			So(len(items), ShouldEqual, 1)
			So(items[0]["id"], ShouldEqual, float64(11))
		})

		Convey("Traduce la modalidad del front al id de la base y de vuelta", func() {
			id, err := services.IDModalidad("jornada_completa")
			So(err, ShouldBeNil)
			So(id, ShouldEqual, 2)

			modalidad, err := services.ModalidadDesdeID(3)
			So(err, ShouldBeNil)
			So(modalidad, ShouldEqual, "fin_de_semana")
		})

		Convey("presencial no existe en la base", func() {
			_, err := services.IDModalidad("presencial")
			So(err, ShouldEqual, services.ErrModalidadSinPareja)
		})

		Convey("El mismo estado en otro contexto tiene otro id", func() {
			id, err := services.IDEstado("oferta", "activa")
			So(err, ShouldBeNil)
			So(id, ShouldEqual, 10)
		})
	})
}
