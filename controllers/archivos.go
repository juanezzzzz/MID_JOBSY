package controllers

import (
	"fmt"
	"mime/multipart"
	"io"
	"net/http"

	"api_MID/models"
	"api_MID/services"
)

// ArchivosController sirve los archivos guardados. Es publico: los nombres
// son aleatorios y no se pueden adivinar.
type ArchivosController struct {
	BaseController
}

// Servir entrega un archivo guardado.
func (c *ArchivosController) Servir() {
	if svc.Almacen == nil {
		c.Fallo(models.NoEncontrado("ese archivo"))
		return
	}
	f, tipo, err := svc.Almacen.Abrir(c.Contexto(), c.Ctx.Input.Param(":splat"))
	if err != nil {
		c.Fallo(models.NoEncontrado("ese archivo"))
		return
	}
	defer f.Close()

	c.Ctx.Output.Header("Content-Type", tipo)
	c.Ctx.Output.Header("X-Content-Type-Options", "nosniff")
	// Los archivos llevan nombre aleatorio y no cambian, asi que se pueden
	// cachear largo.
	c.Ctx.Output.Header("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = io.Copy(c.Ctx.ResponseWriter, f)
}

// recibirArchivo procesa una subida multipart y devuelve la url publica.
//
// El nombre del campo es el que usa cada formulario del frontend: "foto" para
// la imagen de perfil y "cv" para la hoja de vida.
//
// El tamano maximo de la peticion entera lo corta Beego antes de leer nada
// (maxuploadsize en app.conf): sin eso un cliente puede agotar la memoria del
// proceso mandando un multipart enorme.
func (c *SesionController) recibirArchivo(campo, carpeta string) (string, error) {
	if svc.Almacen == nil {
		return "", models.NoImplementado("El almacenamiento de archivos no esta configurado.")
	}

	const maxPeticion = 12 << 20
	r := c.Ctx.Request
	if err := r.ParseMultipartForm(maxPeticion); err != nil {
		return "", models.Validacion("El archivo es demasiado grande o el formulario no es valido.")
	}

	_, cabecera, err := r.FormFile(campo)
	if err != nil {
		return "", models.Validacion("Falta el archivo en el campo " + campo + ".")
	}
	return c.guardarArchivo(cabecera, carpeta)
}

// recibirArchivos guarda hasta max archivos del campo (opcional) y devuelve
// sus urls. El formulario ya debe venir como multipart.
func (c *SesionController) recibirArchivos(campo, carpeta string, max int) ([]string, error) {
	if svc.Almacen == nil {
		return nil, models.NoImplementado("El almacenamiento de archivos no esta configurado.")
	}
	r := c.Ctx.Request
	if r.MultipartForm == nil {
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			return nil, models.Validacion("Los archivos son demasiado grandes o el formulario no es valido.")
		}
	}
	cabeceras := r.MultipartForm.File[campo]
	if len(cabeceras) > max {
		return nil, models.Validacion(fmt.Sprintf("Puedes adjuntar como maximo %d archivos.", max))
	}
	urls := make([]string, 0, len(cabeceras))
	for _, h := range cabeceras {
		u, err := c.guardarArchivo(h, carpeta)
		if err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}
	return urls, nil
}

// guardarArchivo valida el tipo real del archivo y lo pasa al almacen.
func (c *SesionController) guardarArchivo(cabecera *multipart.FileHeader, carpeta string) (string, error) {
	archivo, err := cabecera.Open()
	if err != nil {
		return "", models.Validacion("No se pudo leer el archivo.")
	}
	defer archivo.Close()

	// El tipo se toma de la cabecera del propio archivo, no de lo que declare
	// el cliente: el Content-Type del formulario es trivial de falsear.
	cabeza := make([]byte, 512)
	n, _ := archivo.Read(cabeza)
	tipo := http.DetectContentType(cabeza[:n])
	if _, err := archivo.Seek(0, io.SeekStart); err != nil {
		return "", models.Interno(err)
	}

	url, err := svc.Almacen.Guardar(c.Contexto(), carpeta, services.Archivo{
		Nombre: cabecera.Filename,
		Tipo:   tipo,
		Tamano: cabecera.Size,
	}, archivo)
	if err != nil {
		return "", models.Validacion(err.Error())
	}
	return url, nil
}
