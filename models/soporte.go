package models

type Reporte struct {
	Tipo         string `json:"tipo"`
	Asunto       string `json:"asunto"`
	Descripcion  string `json:"descripcion"`
	Estado       string `json:"estado"`
	ReportanteID string `json:"reportanteId"`
	Fecha        string `json:"fecha"`
	ID           string `json:"id"`
}

// MensajeReporte es una intervencion en el hilo de un reporte de soporte.
type MensajeReporte struct {
	ID    string `json:"id"`
	Autor string `json:"autor"`
	Texto string `json:"texto"`
	Fecha string `json:"fecha"`
	EsMio bool   `json:"esMio"`
}
