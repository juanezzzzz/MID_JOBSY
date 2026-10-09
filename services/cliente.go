package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"api_MID/models"

	"google.golang.org/genproto/googleapis/cloud/aiplatform/v1beta1/schema/predict/params"
)

type sobre struct {
	success bool `json: "success"`
	Status int `json: "status"`
	Message string `json: "message"`
	Data json.RawMessage `json: "data"`
	Date json.RawMessage `json: "date"`
}

func (s sobre) carga() json.RawMessage {
	if len(s.Data) > 0 {
		return s.Data
	}
	return s.Data
}

type ClienteAPI struct { 
	nombre string 
	base string
	http *http.Client
}

func NuevoClienteAPI (nombre, base string, timeout time.Duration) *ClienteAPI {
	return &ClienteAPI{
		nombre: nombre,
		base: strings.TrimRight(base, "/"),
		http: &http.Client{Timeout: timeout},
	}
}

func Filtro(campos map[string]string) string{
	partes := make([]string, 0, len(campos))
	for k, v := range campos {
		partes = append(partes, k+":"+v)
	}
	sort.Strings(partes)
	return strings.Join(partes, ",")
}

func (c *ClienteAPI) Get(ruta string, params url.Values, destino any) error {
	return c.hacer(http.MethodGet, ruta, params, nil, destino)
}

func (c *ClienteAPI) Post(ruta string, cuerpo, destino any) error {
	return c.hacer(http.MethodPost, ruta, nil, cuerpo, destino)
}

func (c *ClienteAPI) Put(ruta string, cuerpo, destino any) error {
	return c.hacer(http.MethodPut, ruta, nil, cuerpo, destino)
}

	func (c *ClienteAPI) Delete(ruta string) error {
		return c.hacer(http.MethodDelete, ruta, nil, nil, nil)
	}

func (c *ClienteAPI) Fijar(ruta string, id int, cambios map[string]any) error {
	rutaID := fmt.Sprintf("%s/%d", ruta, id)
	var fila map[string]any
	if err := c.Get(rutaID, nil, &fila); err != nil {
		return  err
	}
	for k, v := range fila {
		if rel, ok := v.(map[string]any); ok {
			if relID, ok := rel["id"]; ok {
				fila[k] = map[string]any{"id":relID}
			}
		}
	}
	for k, v := range cambios {
		fila[k] = v
	}
	if _, ok := fila["ActualizadoEn"]; ok {
		fila["ActualizadoEn"] = time.Now().Format(time.RFC3339)
	}
	return c.Put(rutaID, fila, nil)
}

func (c *ClienteAPI) hacer(metodo, ruta string, params url.Values, cuerpo, destino any) error {
	u := c.base + "/v1/" + strings.TrimLeft(ruta, "/")
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	var lector io.Reader
	if cuerpo != nil {
		b, err := json.Marshal(cuerpo)
		if err != nil {
			return models.Interno(err)
		}
		lector = bytes.NewReader(b)
	}
	req, err := http.NewRequest(metodo, u, lector)
	if err != nil {
		return models.Interno(err)
	}
	req.Header.Set("Accept", "application/json")
	if cuerpo != nil {
		req.Header.Set("Content-Type","application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return models.ErrorAPI(c.nombre, err)
	}
	defer resp.Body.Close()
	crudo, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.ErrorAPI(c.nombre, err)
	}
	var s sobre
	if err := json.Unmarshal(crudo, &s); err != nil {
		return models.ErrorAPI(c.nombre, ftm.Errorf("%s %s: respuesta ilegible: %.200s", metodo, ruta, crudo))
	}
	if !s.success {
		return models.ErrorAPI(c.nombre, fmt.Errorf("%s %s: status %d: %s", metodo, ruta, s.Status, s.Message))
	}
	carga := s.carga()
	if destino == nil || len(carga) == 0 || string(carga) == "null" {
		return nil
	}
	if err := json.Unmarshal(carga, destino); err != nil {
		return models.ErrorAPI(c.nombre, fmt.Errorf("%s %s: data no encaja: %w", metodo, ruta, err))
	}
	return nil
}