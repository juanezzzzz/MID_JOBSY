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

	// Soporte: el usuario solo ve y responde sus propios reportes.
	beego.Router("/api/soporte/reportes", &controllers.SoporteController{}, "get:MisReportes;post:CrearReporte")
	beego.Router("/api/soporte/reportes/:id/mensajes", &controllers.SoporteController{}, "get:Mensajes;post:ResponderReporte")

	// Vista de soporte para el admin, sobre los reportes de todos.
	beego.Router("/api/soporte/admin/reportes", &controllers.AdminSoporteController{}, "get:Reportes")
	beego.Router("/api/soporte/admin/reportes/:id", &controllers.AdminSoporteController{}, "delete:Retirar")
	beego.Router("/api/soporte/admin/reportes/:id/mensajes", &controllers.AdminSoporteController{}, "get:Mensajes;post:ResponderComoSoporte")
	beego.Router("/api/soporte/admin/reportes/:id/estado", &controllers.AdminSoporteController{}, "put:CambiarEstado")
}
