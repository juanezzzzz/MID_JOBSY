package models 

type Plan struck {
	ID string `json:"id"`
	Nombre string `json:"nombre"`
	Tier string `json:"tier"`
	PrecioMensual float64 `json:"precioMensual"`
	Beneficos []string `json:"beneficios"`
	Destacado bool `json:"destacado"`
}

type Suscripcion struct{
	ID   string `json:"id"`
	UsuarioID string `json:"userId"`
	PlanID string `json:"planId"`
	Activa bool `json:"activa"`
	FechaInicio string `json:"fechaInicio`
	FechaRenovacion string `json:"fechaRenovcion"`
}
