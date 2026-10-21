package models

type Notificacion struct {
	ID      string `json:"id"`
	Titulo  string `json:"titulo"`
	Cuerpo  string `json:"cuerpo"`
	Leida   bool   `json:"leida"`
	Destino string `json:"destino,omitempty"`
	Fecha   string `json:"fecha"`
}

type Configuracion struct {
	PerfilPublico        bool `json:"perfilPublico"`
	MostrarCorreo        bool `json:"mostrarCorreo"`
	MostrarTelefono      bool `json:"mostrarTelefono"`
	HojaVidaVisible      bool `json:"hvVisible"`
	AparecerEnBusquedas  bool `json:"aparecerBusquedas"`
	MostrarUbicacion     bool `json:"mostrarUbicacion"`
	DobleFactor          bool `json:"dobleFactor"`
	NotifCorreo          bool `json:"notifCorreo"`
	NotifPlataforma      bool `json:"notifPlataforma"`
	NotifNuevasOfertas   bool `json:"notifNuevasOfertas"`
	NotifActualizaciones bool `json:"notifActualizaciones"`
	NotifMensajes        bool `json:"notifMensajes"`
	NotifPostulaciones   bool `json:"notifPostulaciones"`
}
