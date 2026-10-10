package routers

import (
	"api_MID/controllers"
	"api_MID/services"
	beego "github.com/beego/beego/v2/server/web"
)

func init() {
	beego.Router("/api/salud", &controllers.SaludController{})

	// Una ruta fija por tabla para no chocar con /api/catalogo/categorias,
	// que es de la tienda.
	for tabla := range services.TablasCatalogo {
		beego.Router("/api/catalogo/"+tabla, &controllers.CatalogosController{}, "get:Listar")
	}
}
