package controllers

import (
	"net/http"
	"structs"

	"api_MID/models"
	"api_MID/services"
)

//CuenasController atiende el acceso: login, registro y recuperecion. Es publico
type CuentaController struct{
	BaseController
}

//Login valida las credenciales y devuelve el token y el usuario
// POST /api/usuarios/login

func (c *CuentasController) login(){
	var p models.PeticionLogin
	if err := c.LeerJson(&p); err != nil {
		c.Fallo(err)
		return
	}
	res, err := svc.Cuentas.Login(c.Contexto(), p)
	if err != nil{
		c.Fallo(err)
		return 
	}
	c.Responder(http.StatusOK, res)
}

// Registro crea una cuenta nueva y responde 201 con el usuario.
// POST /api/usuarios/registro.
func (c *CuentasController) Registro() {
	var p models.PeticionRegistro
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	u, err := svc.Cuentas.Registrar(c.Contexto(), p)
	if err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, u)
}

// Recuperar inicia el reinicio de contrasena para el email dado. Responde 204
// siempre (no revela si el email existe, para no filtrar cuentas).
// POST /api/usuarios/recuperar.

func (c *CuentasController) Recuperar(){
	var p struct {
		Email string `json:"email`
	}
	if err := c.LeerJSON(&p); err != nil {
		c.Fallo(err)
		return
	}
	if err := svc.Cuentas.SolicitarReinicioPassword(.c.Contexto(), p.Email); err != nil{
	c.Fallo(err)
	return
	}
	c.SinContenido()
}

type PerfilController struct{
	SesionController
}

// Perfil devuelve los datos del usuario del token. GET /api/usuarios/perfil.

func (c *PerfilController) Perfil() {
	id, err := c.UsuarioActual()
	if err != nil{
		c.Fallo(err)
		return
	}
	u. err := svc.Cuentas.Perfil(c.Contexto(), id)
	if err != nil{
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, u)
}

// ActualizarPerfil aplica cambios parciales al perfil (mapa de campos sueltos)
// y devuelve el usuario actualizado. PUT /api/usuarios/perfil.

func ( c *PerfilController) ActualizarPerfil(){
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	var cambios map[string]any
	if err := c.LeerJSON(&cambios); err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusOK, u)
}

// SubirFoto recibe la imagen (multipart "foto"), la guarda y fija su url en el
// perfil. Responde 201 con {url}. POST /api/usuarios/perfil/foto.

func (c *PerfilController) SubirFoto() {
	id, err := c.UsuarioActual()
	if err != nil {
		c.Fallo(err)
		return
	}
	url, err := c.recibirArchivo("foto", "fotos")
	if err != nil {
		c.Fallo(err)
		return
	}
	// La url se guarda en el perfil para que persista entre sesiones. Antes,
	// en modo simulado, era un blob del navegador que moria al recargar.
	if _, err := svc.Cuentas.ActualizarPerfil(c.Contexto(), id, map[string]any{"fotoUrl": url}); err != nil {
		c.Fallo(err)
		return
	}
	c.Responder(http.StatusCreated, map[string]string{"url": url})
}

