package routers

import (
	"api_MID/controllers"
	"api_MID/services"
	beego "github.com/beego/beego/v2/server/web"
)

func init() {
	beego.Router("/api/salud", &controllers.SaludController{})

	for tabla := range services.TablasCatalogo {
		beego.Router("/api/catalogo/"+tabla, &controllers.CatalogosController{}, "get:Listar")
	}
}
