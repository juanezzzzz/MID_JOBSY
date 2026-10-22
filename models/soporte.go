package models

type Reporte struct {
	Tipo         string `json:"tipo"`
	Asunto       string `json:"asunto"`
	Descripcion  string `json:"descripcion"`
	Estado       string `json:"estado"`
	ReportanteID string `json:"reportanteId"`
	Fecha        string `json:"fecha"`
	ID           string `json:"id"`

	// Resumen del hilo, para pintar la bandeja y detectar respuestas nuevas
	// sin pedir cada conversacion.
	TotalMensajes int             `json:"totalMensajes"`
	UltimoMensaje *ResumenMensaje `json:"ultimoMensaje,omitempty"`
	// Actividad es la fecha del ultimo mensaje, o la de creacion si no hay.
	Actividad string `json:"actividad"`
	// Reportante es el nombre de quien abrio el reporte (solo en el panel admin).
	Reportante string `json:"reportante,omitempty"`
}

// ResumenMensaje es el ultimo mensaje de un hilo, visto desde la lista.
type ResumenMensaje struct {
	Autor string `json:"autor"`
	Texto string `json:"texto"`
	Fecha string `json:"fecha"`
}

// MensajeReporte es una intervencion en el hilo de un reporte de soporte.
type MensajeReporte struct {
	ID    string `json:"id"`
	Autor string `json:"autor"`
	Texto string `json:"texto"`
	Fecha string `json:"fecha"`
	EsMio bool   `json:"esMio"`
}
