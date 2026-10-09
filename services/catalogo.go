package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"api_MID/models"

	"github.com/beego/beego/v2/core/logs"
	beego "github.com/beego/beego/v2/server/web"
)

// Tablas que expone el mid. La llave es como la pide el front y el valor es
// el recurso en Catalogo_api.
var TablasCatalogo = map[string]string{
	"modalidades":             "modalidades_trabajo",
	"modalidades-suscripcion": "modalidades_suscripcion",
	"estados":                 "estados",
	"tipos":                   "tipos",
	"dias":                    "dias_semana",
	"generos":                 "generos",
	"turnos":                  "turnos",
	"niveles-experiencia":     "niveles_experiencia",
}

var (
	ErrTablaNoExiste      = errors.New("ese catalogo no existe")
	ErrNoEncontrado       = errors.New("el valor no esta en el catalogo")
	ErrCatalogoCaido      = errors.New("no se pudo consultar el servicio de catalogo")
	ErrModalidadSinPareja = errors.New("la modalidad no tiene equivalente en la base")
)

// El front y la base no nombran igual las modalidades de trabajo.
// "presencial" no existe en la base (es de lugar, no de jornada) y la base
// tiene por_jornadas, interno y dias_especificos que el front no pinta.
var modalidadFrontABase = map[string]string{
	"por_horas":        "por_horas",
	"jornada_completa": "tiempo_completo",
	"fin_de_semana":    "fines_semana",
}

var modalidadBaseAFront = map[string]string{
	"por_horas":        "por_horas",
	"tiempo_completo":  "jornada_completa",
	"fines_semana":     "fin_de_semana",
	"por_jornadas":     "jornada_completa",
	"interno":          "jornada_completa",
	"dias_especificos": "por_horas",
}

// Los catalogos casi no cambian, entonces se guardan en memoria un rato
// para no llamar a Catalogo_api en cada peticion.
const duracionCache = 15 * time.Minute

type tablaEnCache struct {
	items     []models.ItemCatalogo
	cargadaEn time.Time
}

var (
	cache   = map[string]tablaEnCache{}
	cacheMu sync.RWMutex
	cliente = &http.Client{Timeout: 10 * time.Second}
)

// respuestaCatalogo es el sobre que devuelven los CRUD de Jobsy_API.
type respuestaCatalogo struct {
	Success bool                  `json:"success"`
	Status  int                   `json:"status"`
	Message string                `json:"message"`
	Data    []models.ItemCatalogo `json:"data"`
}

// ListarCatalogo devuelve las filas de una tabla. Si se pasa contexto (solo
// aplica a estados y tipos) filtra por el.
func ListarCatalogo(tabla, contexto string) ([]models.ItemCatalogo, error) {
	recurso, ok := TablasCatalogo[tabla]
	if !ok {
		return nil, ErrTablaNoExiste
	}
	items, err := traerTabla(recurso)
	if err != nil {
		return nil, err
	}
	if contexto == "" {
		return items, nil
	}
	filtrados := []models.ItemCatalogo{}
	for _, it := range items {
		if it.Contexto == contexto {
			filtrados = append(filtrados, it)
		}
	}
	return filtrados, nil
}

func traerTabla(recurso string) ([]models.ItemCatalogo, error) {
	cacheMu.RLock()
	guardada, hay := cache[recurso]
	cacheMu.RUnlock()
	if hay && time.Since(guardada.cargadaEn) < duracionCache {
		return guardada.items, nil
	}

	items, err := pedirACatalogo(recurso)
	if err != nil {
		// Si ya habia datos se siguen usando, es mejor que dejar al front
		// sin listas porque el CRUD se cayo un momento.
		if hay {
			logs.Warn("catalogo %s: se usa la cache anterior: %v", recurso, err)
			return guardada.items, nil
		}
		return nil, err
	}

	cacheMu.Lock()
	cache[recurso] = tablaEnCache{items: items, cargadaEn: time.Now()}
	cacheMu.Unlock()
	return items, nil
}

func pedirACatalogo(recurso string) ([]models.ItemCatalogo, error) {
	base := strings.TrimRight(beego.AppConfig.DefaultString("url_catalogo", "http://localhost:8084"), "/")
	// Beego trae 10 filas por defecto, con eso se quedarian estados por fuera
	url := fmt.Sprintf("%s/v1/%s?limit=500", base, recurso)

	resp, err := cliente.Get(url)
	if err != nil {
		logs.Error("catalogo %s: %v", recurso, err)
		return nil, ErrCatalogoCaido
	}
	defer resp.Body.Close()

	var cuerpo respuestaCatalogo
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		logs.Error("catalogo %s: respuesta invalida: %v", recurso, err)
		return nil, ErrCatalogoCaido
	}
	if resp.StatusCode != http.StatusOK || !cuerpo.Success {
		logs.Error("catalogo %s: status %d: %s", recurso, resp.StatusCode, cuerpo.Message)
		return nil, ErrCatalogoCaido
	}
	return cuerpo.Data, nil
}

// buscarID encuentra el id de una fila por nombre (y contexto si aplica).
func buscarID(recurso, contexto, nombre string) (int, error) {
	items, err := traerTabla(recurso)
	if err != nil {
		return 0, err
	}
	nombre = strings.ToLower(strings.TrimSpace(nombre))
	for _, it := range items {
		if contexto != "" && it.Contexto != contexto {
			continue
		}
		if strings.ToLower(it.Nombre) == nombre {
			return it.Id, nil
		}
	}
	return 0, ErrNoEncontrado
}

// buscarNombre es el camino inverso: del id al nombre.
func buscarNombre(recurso string, id int) (string, error) {
	items, err := traerTabla(recurso)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Id == id {
			return it.Nombre, nil
		}
	}
	return "", ErrNoEncontrado
}

// ---- Traducciones que usan los demas modulos del mid ----

// IDModalidad pasa la modalidad que manda el front al id de la base.
func IDModalidad(modalidad string) (int, error) {
	nombreBase, ok := modalidadFrontABase[modalidad]
	if !ok {
		return 0, ErrModalidadSinPareja
	}
	return buscarID("modalidades_trabajo", "", nombreBase)
}

// ModalidadDesdeID pasa el id de la base a la modalidad que entiende el front.
func ModalidadDesdeID(id int) (string, error) {
	nombre, err := buscarNombre("modalidades_trabajo", id)
	if err != nil {
		return "", err
	}
	modalidad, ok := modalidadBaseAFront[nombre]
	if !ok {
		return "", ErrModalidadSinPareja
	}
	return modalidad, nil
}

// IDEstado busca un estado dentro de su contexto (oferta, postulacion,
// contratacion, pedido...). El mismo nombre se repite entre contextos.
func IDEstado(contexto, nombre string) (int, error) {
	return buscarID("estados", contexto, nombre)
}

func EstadoDesdeID(id int) (string, error) {
	return buscarNombre("estados", id)
}

// IDTipo busca un tipo dentro de su contexto (pago, producto_tienda,
// notificacion...).
func IDTipo(contexto, nombre string) (int, error) {
	return buscarID("tipos", contexto, nombre)
}

func TipoDesdeID(id int) (string, error) {
	return buscarNombre("tipos", id)
}

// IDDiaSemana recibe el codigo corto del dia ("lun", "mar"...).
func IDDiaSemana(codigo string) (int, error) {
	return buscarID("dias_semana", "", codigo)
}

func DiaSemanaDesdeID(id int) (string, error) {
	return buscarNombre("dias_semana", id)
}
