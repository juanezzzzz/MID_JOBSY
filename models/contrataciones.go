
package models

type FranjaAgenda struct {
	Dia		      string `json:"dia"`
	HoraInicio    string `json:"hora_inicio"`
	HoraFin       string `json:"hora_fin"`
	Nota 		  string `json:"nota,omitempty"`
}

type Contratacion struct {
	ID                 string `json:"id"`
	OfertaID           string `json:"oferta_id"`
	PostulacionID      string `json:"postulacion_id"`
	EmpleadorID        string `json:"empleador_id"`
	EmpleadoID         string `json:"empleado_id"`
	Estado			   string `json:"estado"`
	Direccion		   string `json:"direccion"`
	FechaInicio		   string `json:"fecha_inicio"`
	FechaFin		   string `json:"fecha_fin"`
	DuracionHoras      string `json:"duracion_horas"`
	Tarifa             float64 `json:"tarifa"`
	TotalEstimado	   float64 `json:"total_estimado"`
	Descripcion		   string `json:"descripcion,omitempty"`
	Nota			   string `json:"nota,omitempty"`
	Fecha              string `json:"fecha"`

	//soyEmpleador le dice al frontend que lado de la relacion ocupa quien
	//consulta, para decir que acciones ofrecer.

	SoyEmpleador       bool   `json:"soy_empleador"`
	Agenda			   []FranjaAgenda `json:"agenda,omitempty"`
}