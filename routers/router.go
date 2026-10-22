// Package routers declara las rutas del middleware con el prefijo /api que
// espera el frontend (environment.apiUrl acaba en /api).
//
// Quien puede llamar a cada ruta no se decide aqui sino en el tipo del
// controlador: los que embeben SesionController exigen token y los que
// embeben AdminController exigen ademas el rol. Ver controllers/base.go.
package routers

import (
	"api_MID/controllers"

	beego "github.com/beego/beego/v2/server/web"
)

// init instala los filtros globales y el manejo de errores, y registra todas
// las rutas de la API con su controlador.
func init() {
	// Filtros transversales, en orden de ejecucion. Los panicos los recoge
	// controllers.Recuperar, instalado como RecoverFunc de Beego.
	beego.InsertFilterChain("*", controllers.ConPeticionID)
	beego.InsertFilterChain("*", controllers.Registrar)
	beego.InsertFilterChain("*", controllers.CORS)
	beego.InsertFilterChain("*", controllers.LimitarPeticiones(240, 120))

	beego.BConfig.RecoverFunc = controllers.Recuperar
	controllers.RegistrarErrores()

	// ---- Publicas ----
	beego.Router("/api/salud", &controllers.SaludController{}, "get:Salud")

	beego.Router("/api/usuarios/login", &controllers.CuentasController{}, "post:Login")
	beego.Router("/api/usuarios/registro", &controllers.CuentasController{}, "post:Registro")
	beego.Router("/api/usuarios/recuperar", &controllers.CuentasController{}, "post:Recuperar")

	// El buscador de empleados y el de ofertas no exigen sesion en el front.
	// Las rutas literales ganan a las de parametro: /destacados no es un :id.
	beego.Router("/api/usuarios/empleados", &controllers.EmpleadosController{}, "get:Listar")
	beego.Router("/api/usuarios/empleados/destacados", &controllers.EmpleadosController{}, "get:Destacados")
	beego.Router("/api/usuarios/empleados/:id", &controllers.EmpleadosController{}, "get:Detalle")
	beego.Router("/api/usuarios/empleados/:id/resenas", &controllers.EmpleadosController{}, "get:Resenas")
	beego.Router("/api/usuarios/empleados/:id/disponibilidad", &controllers.EmpleadosController{}, "get:Disponibilidad")

	beego.Router("/api/empleo/ofertas", &controllers.OfertasController{}, "get:Listar")
	beego.Router("/api/empleo/ofertas/destacadas", &controllers.OfertasController{}, "get:Destacadas")
	beego.Router("/api/empleo/ofertas/:id", &controllers.OfertasController{}, "get:Detalle")

	beego.Router("/api/tienda/productos", &controllers.CatalogoController{}, "get:Productos")
	beego.Router("/api/tienda/productos/:id", &controllers.CatalogoController{}, "get:DetalleProducto")
	beego.Router("/api/catalogo/categorias", &controllers.CatalogoController{}, "get:Categorias")
	beego.Router("/api/suscripciones/planes", &controllers.CatalogoController{}, "get:Planes")
	beego.Router("/api/tienda/metodos-pago", &controllers.CatalogoController{}, "get:MetodosPago")

	beego.Router("/archivos/*", &controllers.ArchivosController{}, "get:Servir")

	// La notificacion la manda el proveedor de pago, que no trae sesion: la
	// protege la firma, no el token.
	beego.Router("/api/tienda/pagos/notificacion", &controllers.PasarelaController{}, "post:Notificacion")

	// ---- Con sesion ----
	beego.Router("/api/usuarios/perfil", &controllers.PerfilController{}, "get:Perfil;put:ActualizarPerfil")
	beego.Router("/api/usuarios/perfil/foto", &controllers.PerfilController{}, "post:SubirFoto")
	beego.Router("/api/usuarios/password", &controllers.PerfilController{}, "put:CambiarPassword")
	beego.Router("/api/usuarios/baja", &controllers.PerfilController{}, "post:DarDeBaja")
	beego.Router("/api/usuarios/hoja-de-vida", &controllers.PerfilController{}, "get:HojaVida;put:ActualizarHojaVida")
	beego.Router("/api/usuarios/hoja-de-vida/archivo", &controllers.PerfilController{}, "post:SubirCV")
	beego.Router("/api/usuarios/disponibilidad", &controllers.PerfilController{}, "get:MiDisponibilidad;put:FijarDisponibilidad")
	beego.Router("/api/usuarios/documentos", &controllers.PerfilController{}, "get:MisDocumentos;post:SubirDocumento")
	beego.Router("/api/usuarios/resenas", &controllers.PerfilController{}, "post:PublicarResena")
	beego.Router("/api/usuarios/notificaciones", &controllers.PerfilController{}, "get:MisNotificaciones")
	beego.Router("/api/usuarios/notificaciones/:id/leida", &controllers.PerfilController{}, "post:MarcarNotificacionLeida")
	beego.Router("/api/usuarios/configuracion", &controllers.PerfilController{}, "get:MiConfiguracion;put:ActualizarConfiguracion")

	// GET /api/empleo/ofertas es publico (OfertasController); POST exige sesion.
	beego.Router("/api/empleo/ofertas", &controllers.EmpleoController{}, "post:CrearOferta")
	beego.Router("/api/empleo/postulaciones", &controllers.EmpleoController{}, "post:Postular")
	beego.Router("/api/empleo/postulaciones/mias", &controllers.EmpleoController{}, "get:MisPostulaciones")
	beego.Router("/api/empleo/postulaciones/:id", &controllers.EmpleoController{}, "delete:RetirarPostulacion")
	beego.Router("/api/empleo/contrataciones", &controllers.EmpleoController{}, "post:Contratar")
	beego.Router("/api/empleo/contrataciones/mias", &controllers.EmpleoController{}, "get:MisContrataciones")
	beego.Router("/api/empleo/contrataciones/directa", &controllers.EmpleoController{}, "post:ContratarDirecto")
	beego.Router("/api/empleo/contrataciones/:id", &controllers.EmpleoController{}, "get:DetalleContratacion")
	beego.Router("/api/empleo/contrataciones/:id/finalizar", &controllers.EmpleoController{}, "post:FinalizarContratacion")

	beego.Router("/api/tienda/pedidos", &controllers.TiendaController{}, "post:CrearPedido")
	beego.Router("/api/tienda/pedidos/mios", &controllers.TiendaController{}, "get:MisPedidos")
	beego.Router("/api/tienda/cupones/validar", &controllers.TiendaController{}, "post:ValidarCupon")
	beego.Router("/api/tienda/carrito", &controllers.TiendaController{}, "get:MiCarrito;post:AgregarAlCarrito;delete:VaciarCarrito")
	beego.Router("/api/tienda/carrito/:id", &controllers.TiendaController{}, "delete:QuitarDelCarrito;put:FijarEnCarrito")
	beego.Router("/api/tienda/metodos-guardados", &controllers.TiendaController{}, "get:MisMetodosGuardados;post:GuardarMetodoPago")
	beego.Router("/api/tienda/metodos-guardados/:id", &controllers.TiendaController{}, "delete:BorrarMetodoPago")
	beego.Router("/api/tienda/saldo", &controllers.TiendaController{}, "get:MiSaldo")
	beego.Router("/api/tienda/pagos/mios", &controllers.TiendaController{}, "get:MisPagos")

	beego.Router("/api/suscripciones/mi-suscripcion", &controllers.SuscripcionesController{}, "get:MiSuscripcion")
	beego.Router("/api/suscripciones/suscribir", &controllers.SuscripcionesController{}, "post:Suscribir")

	beego.Router("/api/soporte/reportes", &controllers.SoporteController{}, "get:MisReportes;post:CrearReporte")
	beego.Router("/api/soporte/reportes/:id/mensajes", &controllers.SoporteController{}, "get:MensajesReporte;post:ResponderReporte")

	// ---- Administracion ----
	beego.Router("/api/tienda/admin/estadisticas", &controllers.AdminTiendaController{}, "get:Estadisticas")
	beego.Router("/api/tienda/admin/ventas", &controllers.AdminTiendaController{}, "get:Ventas")
	beego.Router("/api/tienda/admin/productos-top", &controllers.AdminTiendaController{}, "get:ProductosTop")
	beego.Router("/api/tienda/admin/pedidos-recientes", &controllers.AdminTiendaController{}, "get:PedidosRecientes")
	beego.Router("/api/tienda/admin/productos", &controllers.AdminTiendaController{}, "get:Productos;post:CrearProducto")
	beego.Router("/api/tienda/admin/productos/:id", &controllers.AdminTiendaController{}, "put:ActualizarProducto;delete:BorrarProducto")
	beego.Router("/api/tienda/admin/productos/:id/stock", &controllers.AdminTiendaController{}, "post:AjustarStock")
	beego.Router("/api/tienda/admin/pedidos/:id/reembolsar", &controllers.AdminTiendaController{}, "post:Reembolsar")
	beego.Router("/api/tienda/admin/categorias", &controllers.AdminTiendaController{}, "post:CrearCategoria")
	beego.Router("/api/tienda/admin/categorias/:id", &controllers.AdminTiendaController{}, "put:RenombrarCategoria;delete:BorrarCategoria")

	beego.Router("/api/soporte/admin/reportes", &controllers.AdminSoporteController{}, "get:Reportes")
	beego.Router("/api/soporte/admin/reportes/:id", &controllers.AdminSoporteController{}, "delete:Retirar")
	beego.Router("/api/soporte/admin/reportes/:id/mensajes", &controllers.AdminSoporteController{}, "get:Mensajes;post:ResponderComoSoporte")
	beego.Router("/api/soporte/admin/reportes/:id/estado", &controllers.AdminSoporteController{}, "put:CambiarEstado")
}
