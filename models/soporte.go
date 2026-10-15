package models

// Reporte es un caso de soporte abierto por un usuario (un problema tecnico,
// un cobro, una queja sobre otro usuario o una sugerencia).
//
// La base guarda el tipo y el estado como claves foraneas a catalogo.tipos y
// catalogo.estados (contexto 'reporte'); aqui viajan como texto.
type Reporte struct {
	ID           string `json:"id"`
	Tipo         string `json:"tipo"`
	Descripcion  string `json:"descripcion"`
	Estado       string `json:"estado"`
	ReportanteID string `json:"reportanteId"`
	ArchivoURL   string `json:"archivoUrl,omitempty"`
	Fecha        string `json:"fecha"`
}

// MensajeReporte es una intervencion en el hilo de un reporte.
type MensajeReporte struct {
	ID      string `json:"id"`
	AutorID string `json:"autorId"`
	Texto   string `json:"texto"`
	Fecha   string `json:"fecha"`

	// EsMio le dice al front de que lado pintar la burbuja.
	EsMio bool `json:"esMio"`
}
