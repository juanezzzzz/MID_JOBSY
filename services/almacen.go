// Almacen guarda los archivos que suben los usuarios.
//
// El esquema de Jobsy tiene columnas de url por todas partes (foto_url,
// url_pdf, url_documento, imagen_url, archivo_url) pero no existe ningun
// servicio que reciba el binario: son cadenas que alguien tiene que rellenar a
// mano.
//
// Aqui se define el contrato de almacenamiento y una implementacion en disco,
// suficiente para desarrollo y para un despliegue de un solo servidor. Cambiar
// a S3, Cloud Storage o Cloudinary es escribir otro tipo que cumpla Almacen;
// nada del codigo que sube archivos cambia.

package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Archivo describe lo subido.
type Archivo struct {
	Nombre string
	Tipo   string
	Tamano int64
}

// Almacen guarda y sirve archivos.
type Almacen interface {
	// Guardar escribe el contenido y devuelve la url publica.
	Guardar(ctx context.Context, carpeta string, a Archivo, r io.Reader) (string, error)
	// Abrir devuelve el contenido de una ruta previamente guardada.
	Abrir(ctx context.Context, ruta string) (io.ReadCloser, string, error)
	// Borrar elimina un archivo.
	Borrar(ctx context.Context, ruta string) error
}

// tiposPermitidos limita lo que se acepta por carpeta.
//
// No se acepta cualquier cosa: un archivo subido y luego servido es una via
// clasica para colar contenido ejecutable o un SVG con scripts.
var tiposPermitidos = map[string]map[string]string{
	"fotos": {
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	},
	"cv": {
		"application/pdf": ".pdf",
	},
	"documentos": {
		"image/jpeg":      ".jpg",
		"image/png":       ".png",
		"application/pdf": ".pdf",
	},
	// Fotos de las casas de las ofertas.
	"ofertas": {
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	},
}

// Limites por carpeta, en bytes.
var limites = map[string]int64{
	"fotos":      5 << 20,  // 5 MiB
	"cv":         10 << 20, // 10 MiB
	"documentos": 10 << 20,
	"ofertas":    5 << 20,
}

// AlmacenEnDisco guarda bajo un directorio local y sirve por una url base.
type AlmacenEnDisco struct {
	raiz    string
	urlBase string
}

// formatosAdmitidos lista las extensiones validas: "JPG, PNG o WEBP".
func formatosAdmitidos(permitidos map[string]string) string {
	vistos := map[string]bool{}
	var exts []string
	for _, e := range permitidos {
		e = strings.ToUpper(strings.TrimPrefix(e, "."))
		if !vistos[e] {
			vistos[e] = true
			exts = append(exts, e)
		}
	}
	sort.Strings(exts)
	if len(exts) <= 1 {
		return strings.Join(exts, "")
	}
	return strings.Join(exts[:len(exts)-1], ", ") + " o " + exts[len(exts)-1]
}

// NuevoAlmacenEnDisco crea el almacen bajo el directorio raiz (que crea si no
// existe) y sirve los archivos bajo urlBase. Falla si no puede crear la raiz.
func NuevoAlmacenEnDisco(raiz, urlBase string) (*AlmacenEnDisco, error) {
	if err := os.MkdirAll(raiz, 0o755); err != nil {
		return nil, fmt.Errorf("no se pudo crear el directorio de archivos %q: %w", raiz, err)
	}
	return &AlmacenEnDisco{raiz: raiz, urlBase: strings.TrimRight(urlBase, "/")}, nil
}

// Guardar valida el tipo y el tamano segun la carpeta, escribe el contenido con
// un nombre generado por el servidor (nunca el del cliente) y devuelve su url
// publica. Corta en el limite aunque el cliente mienta en el tamano declarado.
func (d *AlmacenEnDisco) Guardar(ctx context.Context, carpeta string, a Archivo, r io.Reader) (string, error) {
	permitidos, ok := tiposPermitidos[carpeta]
	if !ok {
		return "", fmt.Errorf("carpeta de destino desconocida: %q", carpeta)
	}
	ext, ok := permitidos[strings.ToLower(strings.TrimSpace(a.Tipo))]
	if !ok {
		// El mensaje llega tal cual al usuario: se dicen los formatos validos
		// y no el tipo MIME, que no le dice nada.
		return "", fmt.Errorf("Ese tipo de archivo no se admite aqui. Sube un archivo %s.", formatosAdmitidos(permitidos))
	}
	limite := limites[carpeta]
	if a.Tamano > limite {
		return "", fmt.Errorf("El archivo es demasiado grande: el maximo es %d MB.", limite/(1<<20))
	}

	// El nombre lo pone el servidor. Usar el del cliente permitiria
	// sobrescribir archivos ajenos o salirse del directorio con "../".
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nombre := hex.EncodeToString(b) + ext

	dir := filepath.Join(d.raiz, carpeta)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	destino := filepath.Join(dir, nombre)
	f, err := os.OpenFile(destino, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Se corta en el limite aunque el cliente mienta en el tamano declarado.
	escritos, err := io.Copy(f, io.LimitReader(r, limite+1))
	if err != nil {
		_ = os.Remove(destino)
		return "", err
	}
	if escritos > limite {
		_ = os.Remove(destino)
		return "", fmt.Errorf("el archivo supera el maximo de %d bytes", limite)
	}

	return d.urlBase + "/archivos/" + carpeta + "/" + nombre, nil
}

// Abrir devuelve un lector del archivo en ruta y su Content-Type. Valida la
// ruta para impedir salirse del directorio de archivos.
func (d *AlmacenEnDisco) Abrir(ctx context.Context, ruta string) (io.ReadCloser, string, error) {
	limpia, err := d.rutaSegura(ruta)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(limpia)
	if err != nil {
		return nil, "", err
	}
	return f, tipoPorExtension(filepath.Ext(limpia)), nil
}

// Borrar elimina el archivo en ruta (validandola igual que Abrir).
func (d *AlmacenEnDisco) Borrar(ctx context.Context, ruta string) error {
	limpia, err := d.rutaSegura(ruta)
	if err != nil {
		return err
	}
	return os.Remove(limpia)
}

// rutaSegura impide salirse del directorio de archivos con rutas como
// "../../conf/app.conf".
func (d *AlmacenEnDisco) rutaSegura(ruta string) (string, error) {
	limpia := filepath.Clean(filepath.Join(d.raiz, filepath.FromSlash(ruta)))
	raiz := filepath.Clean(d.raiz)
	if !strings.HasPrefix(limpia, raiz+string(os.PathSeparator)) && limpia != raiz {
		return "", fmt.Errorf("ruta fuera del directorio de archivos: %q", ruta)
	}
	return limpia, nil
}

// tipoPorExtension deduce el Content-Type a partir de la extension del archivo;
// si no la reconoce devuelve application/octet-stream.
func tipoPorExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
