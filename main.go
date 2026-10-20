package main

import (
	"strings"
	"time"

	"api_MID/controllers"
	_ "api_MID/routers"
	"api_MID/services"

	beego "github.com/beego/beego/v2/server/web"
	"github.com/beego/beego/v2/server/web/filter/cors"
)

func main() {
	origenes := strings.Split(beego.AppConfig.DefaultString("origenes", "http://localhost:4200"), ",")
	beego.InsertFilter("*", beego.BeforeRouter, cors.Allow(&cors.Options{
		AllowOrigins:     origenes,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// Sin esto el controlador de soporte queda con el servicio en nil y se
	// cae en la primera peticion.
	timeout, err := time.ParseDuration(beego.AppConfig.DefaultString("timeout_upstream", "10s"))
	if err != nil {
		timeout = 10 * time.Second
	}
	urlSoporte := beego.AppConfig.DefaultString("url_soporte", "http://localhost:8087")
	controllers.ConfigurarSoporte(services.NuevoSoporte(services.NuevoClienteAPI("soporte", urlSoporte, timeout)))

	beego.Run()
}
