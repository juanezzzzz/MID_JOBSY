// Jobsy Mid arranca el middleware de Jobsy sobre Beego.
//
// El mid es la unica puerta de entrada al sistema: los 7 servicios de
// Jobsy_API no tienen autenticacion de ningun tipo y deben quedar en una red
// privada, inalcanzables desde Internet.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"api_MID/controllers"
	_ "api_MID/routers"
	"api_MID/services"

	beego "github.com/beego/beego/v2/server/web"
)

// main configura el middleware, arranca el servidor Beego y hace un apagado
// ordenado al recibir la senal de parada.
func main() {
	if err := configurar(); err != nil {
		slog.Error("el middleware no pudo arrancar", "error", err)
		os.Exit(1)
	}

	// Beego fija Read y WriteTimeout con servertimeout; lo demas se pone aqui.
	beego.BeeApp.Server.ReadHeaderTimeout = 10 * time.Second
	beego.BeeApp.Server.IdleTimeout = 120 * time.Second

	// Apagado ordenado: dejamos terminar las peticiones en curso. web.Run
	// vuelve en cuanto el servidor deja de escuchar, asi que hay que esperar
	// a que Shutdown termine antes de salir.
	apagado := make(chan struct{})
	go func() {
		defer close(apagado)
		paro := make(chan os.Signal, 1)
		signal.Notify(paro, os.Interrupt, syscall.SIGTERM)
		s := <-paro
		slog.Info("apagando", "senal", s.String())
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := beego.BeeApp.Server.Shutdown(ctx); err != nil {
			slog.Error("el apagado no termino a tiempo", "error", err)
		}
	}()

	beego.Run()
	<-apagado
}

// configurar carga app.conf y deja listas las dependencias de los
// controladores.
func configurar() error {
	cfg, err := services.CargarConfig()
	if err != nil {
		return err
	}

	nivel := slog.LevelInfo
	if cfg.Entorno == "dev" {
		nivel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: nivel})))

	// Un cliente por upstream: cada uno con su timeout y su pool.
	core := services.NuevoClienteUpstream(cfg.Core.Nombre, cfg.Core.BaseURL, cfg.TimeoutUpstream)
	empleo := services.NuevoClienteUpstream(cfg.Empleo.Nombre, cfg.Empleo.BaseURL, cfg.TimeoutUpstream)
	cat := services.NuevoClienteUpstream(cfg.Catalogo.Nombre, cfg.Catalogo.BaseURL, cfg.TimeoutUpstream)
	tienda := services.NuevoClienteUpstream(cfg.Tienda.Nombre, cfg.Tienda.BaseURL, cfg.TimeoutUpstream)
	susc := services.NuevoClienteUpstream(cfg.Suscripciones.Nombre, cfg.Suscripciones.BaseURL, cfg.TimeoutUpstream)
	soporte := services.NuevoClienteUpstream(cfg.Soporte.Nombre, cfg.Soporte.BaseURL, cfg.TimeoutUpstream)
	usuarios := services.NuevoClienteUpstream(cfg.Usuarios.Nombre, cfg.Usuarios.BaseURL, cfg.TimeoutUpstream)

	catSvc := services.NuevoCatalogo(cat, 15*time.Minute)

	// Los catalogos se precargan al arrancar. Si no se puede, seguimos: el
	// paquete recarga bajo demanda y preferimos arrancar degradado a no
	// arrancar, porque el healthcheck ya reportara el problema.
	ctxCarga, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	if err := catSvc.Precargar(ctxCarga); err != nil {
		slog.Warn("no se pudieron precargar los catalogos; se reintentara bajo demanda", "error", err)
	}
	if err := catSvc.CargarTamanos(ctxCarga, empleo); err != nil {
		slog.Warn("no se pudieron cargar los tamanos de inmueble", "error", err)
	}

	// Pasarelas de cobro. El registro decide con que proveedor se cobra cada
	// metodo; integrar uno real es anadir aqui un tipo que cumpla el contrato,
	// sin tocar el codigo que cobra.
	//
	// Hoy tarjeta, PSE, Nequi y Daviplata usan la pasarela simulada porque no
	// hay credenciales de ningun proveedor. El saldo interno si mueve dinero
	// de verdad: vive en nuestra base.
	monedero := services.NuevoMonedero(tienda)
	registroPago := services.NuevoRegistroPasarelas()
	for _, metodo := range []string{"tarjeta", "pse", "nequi", "daviplata"} {
		registroPago.Registrar(services.NuevaPasarelaSimulada(metodo, 150*time.Millisecond))
	}
	registroPago.Registrar(services.NuevaPasarelaSaldo(monedero))
	slog.Info("pasarelas de cobro registradas", "metodos", registroPago.Metodos())

	// Almacenamiento de archivos. En disco basta para desarrollo y para un
	// despliegue de un solo servidor; con varias instancias hay que cambiarlo
	// por un almacen compartido, que es escribir otro tipo con el mismo
	// contrato.
	var almacenArchivos services.Almacen
	if a, err := services.NuevoAlmacenEnDisco(cfg.DirArchivos, cfg.URLPublica); err != nil {
		slog.Warn("sin almacenamiento de archivos: las subidas responderan 501", "error", err)
	} else {
		almacenArchivos = a
		slog.Info("almacenamiento de archivos listo", "directorio", cfg.DirArchivos)
	}

	svcAuth := services.NuevoServicioAuth(cfg.JWTSecreto, cfg.JWTDuracion)
	avisos := services.NuevoNotificaciones(usuarios, catSvc)
	svcOfertas := services.NuevoOfertas(empleo, catSvc)
	svcEmpleados := services.NuevoEmpleados(core, empleo, catSvc)
	svcPostulaciones := services.NuevoPostulaciones(empleo, catSvc).ConAvisos(avisos)
	svcPagos := services.NuevoPagos(tienda, catSvc, registroPago, monedero)
	svcTienda := services.NuevoTienda(tienda, catSvc, svcPagos)
	controllers.Configurar(&controllers.Servicios{
		Cfg:              cfg,
		Auth:             svcAuth,
		Cuentas:          services.NuevoCuentas(core, svcAuth).ConPerfil(services.NuevoPerfilRegistro(core, empleo, cat, catSvc)),
		Empleados:        svcEmpleados,
		Ofertas:          svcOfertas,
		Empleador:        services.NuevoEmpleador(empleo, catSvc, svcOfertas, svcPostulaciones, svcEmpleados, avisos),
		Postulaciones:    svcPostulaciones,
		HojasVida:        services.NuevoHojasVida(empleo, catSvc),
		Tienda:           svcTienda,
		Pagos:            svcPagos,
		Monedero:         monedero,
		Suscripciones:    services.NuevoSuscripciones(susc, catSvc),
		Soporte:          services.NuevoSoporte(soporte, catSvc),
		Admin:            services.NuevoAdmin(tienda, core, catSvc, svcTienda),
		Contrataciones:   services.NuevoContrataciones(empleo, catSvc, svcPagos).ConAvisos(avisos).ConUsuarios(core),
		Resenas:          services.NuevoResenas(empleo, catSvc),
		Notificaciones:   avisos,
		Configuraciones:  services.NuevoConfiguraciones(usuarios),
		Almacen:          almacenArchivos,
		Carrito:          services.NuevoCarrito(tienda, catSvc),
		MetodosGuardados: services.NuevoMetodosGuardados(tienda, catSvc),
		PerfilEmpleado:   services.NuevoPerfilEmpleado(empleo, catSvc),
	})

	slog.Info("middleware listo",
		"puerto", cfg.Puerto,
		"entorno", cfg.Entorno,
		"origenes", cfg.OrigenesPermitidos,
	)
	return nil
}
