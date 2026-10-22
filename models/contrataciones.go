package models

type FranjaAgenda struct {
	Dia        string `json:"dia"`
	HoraInicio string `json:"horaInicio"`
	HoraFin    string `json:"horaFin"`
	Nota       string `json:"nota,omitempty"`
}

type Contratacion struct {
	ID            string  `json:"id"`
	OfertaID      string  `json:"ofertaId"`
	PostulacionID string  `json:"postulacionId"`
	EmpleadorID   string  `json:"empleadorId"`
	EmpleadoID    string  `json:"empleadoId"`
	Estado        string  `json:"estado"`
	Direccion     string  `json:"direccion"`
	FechaInicio   string  `json:"fechaInicio"`
	HoraInicio    string  `json:"horaInicio"`
	DuracionHoras float64 `json:"duracionHoras"`
	Tarifa        float64 `json:"tarifa"`
	TotalEstimado float64 `json:"totalEstimado"`
	Descripcion   string  `json:"descripcion,omitempty"`
	Nota          string  `json:"nota,omitempty"`
	Fecha         string  `json:"fecha"`

	// Titulo de la oferta y nombre de la otra parte (el empleado si consulta
	// el empleador, y al reves), para pintar la lista sin mas peticiones.
	Titulo        string `json:"titulo,omitempty"`
	ContraparteID string `json:"contraparteId,omitempty"`
	Contraparte   string `json:"contraparte,omitempty"`

	// SoyEmpleador le dice al frontend que lado de la relacion ocupa quien
	// consulta, para decidir que acciones ofrecer.
	SoyEmpleador bool           `json:"soyEmpleador"`
	Agenda       []FranjaAgenda `json:"agenda,omitempty"`
}
