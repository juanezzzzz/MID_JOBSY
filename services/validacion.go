package services

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"api_MID/models"
)

// Reglas de validacion compartidas por los servicios.
//
// Jobsy_API guarda lo que reciba y el frontend se puede saltar, asi que el mid
// es el unico lugar donde un dato se puede rechazar a tiempo. Los formularios
// del front aplican las mismas reglas (core/validation/reglas.ts) para avisar
// antes de enviar; si se cambia una regla aqui hay que cambiarla alli.

// Limites de negocio.
const (
	TarifaHoraMin = 5000   // COP
	TarifaHoraMax = 200000 // COP
	HorasMin      = 1
	HorasMax      = 12
	EdadMin       = 18
	EdadMax       = 80
	DistanciaMax  = 100 // km
)

var (
	// Calle 10 # 20-30, Cra 84 #65-22 apto 301, Av. 1 No. 2-3, Diagonal 5B bis # 4-10...
	reDireccionUrbana = regexp.MustCompile(`^(avenida calle|avenida carrera|calle|cll|cl|carrera|cra|cr|kra|kr|avenida|av|ac|ak|diagonal|diag|dg|transversal|transv|tv|tr|circular|cq|autopista|via)\.?\s*\d{1,3}\s?[a-z]?(\s?bis)?(\s?[a-z])?\s*(#|no\.?|n°|numero)\s*\d{1,3}\s?[a-z]?\s*-\s*\d{1,3}\b`)
	// Vereda La Esperanza, Km 5 via Aguazul, Manzana 4 Casa 12
	reDireccionRural = regexp.MustCompile(`^(vereda\s+\S+|km\.?\s*\d+|kilometro\s*\d+|(manzana|mz)\.?\s*\w+\s+(casa|lote)\s*\w+)`)
	reHora           = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	reNombre         = regexp.MustCompile(`^[\p{L}]+([ '\-][\p{L}]+)*$`)
	reCiudad         = regexp.MustCompile(`^[\p{L}]+([ .\-][\p{L}]+)*$`)
	reCedula         = regexp.MustCompile(`^\d{6,10}$`)
	reDocExtranjero  = regexp.MustCompile(`^[A-Za-z0-9]{5,15}$`)
	reTelefono       = regexp.MustCompile(`^(3\d{9}|60\d{8})$`)
	reUltimos4       = regexp.MustCompile(`^\d{4}$`)
	reNumeroCuenta   = regexp.MustCompile(`^\d{6,20}$`)
	reAno            = regexp.MustCompile(`^\d{4}$`)
	reHorario        = regexp.MustCompile(`^([a-z, ]+)\s*[·\-|]\s*(\d{2}:\d{2})\s+a\s+(\d{2}:\d{2})$`)
	reLimpiarTel     = regexp.MustCompile(`[\s\-().]`)
)

// DiasHorario son los dias que admite un horario, en el orden de la semana.
var DiasHorario = []string{"lun", "mar", "mie", "jue", "vie", "sab", "dom"}

func invalido(campo, mensaje string) error {
	return models.Validacion(mensaje).ConCampo(campo, mensaje)
}

// ValidarTexto exige entre min y max caracteres (sin contar espacios de los
// extremos). Con min 0 el campo es opcional.
func ValidarTexto(campo, etiqueta, valor string, min, max int) error {
	n := utf8.RuneCountInString(strings.TrimSpace(valor))
	if n == 0 && min > 0 {
		return invalido(campo, etiqueta+" es obligatorio.")
	}
	if n > 0 && n < min {
		return invalido(campo, fmt.Sprintf("%s debe tener al menos %d caracteres.", etiqueta, min))
	}
	if n > max {
		return invalido(campo, fmt.Sprintf("%s admite como maximo %d caracteres.", etiqueta, max))
	}
	return nil
}

// ValidarRango exige un numero entre min y max.
func ValidarRango(campo, etiqueta string, valor, min, max float64) error {
	if valor < min || valor > max {
		return invalido(campo, fmt.Sprintf("%s debe estar entre %s y %s.", etiqueta, numero(min), numero(max)))
	}
	return nil
}

func numero(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	// Miles con punto, como se escriben en Colombia.
	if len(s) > 3 && !strings.Contains(s, ".") {
		var b strings.Builder
		for i, r := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				b.WriteByte('.')
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	return s
}

// ValidarTarifa exige una tarifa por hora razonable en pesos.
func ValidarTarifa(campo string, valor float64) error {
	return ValidarRango(campo, "La tarifa por hora", valor, TarifaHoraMin, TarifaHoraMax)
}

// ValidarDireccion acepta direcciones urbanas colombianas (via, numero, # y
// placa) y las rurales habituales (vereda, kilometro, manzana y casa).
func ValidarDireccion(campo, valor string) error {
	v := strings.TrimSpace(valor)
	if err := ValidarTexto(campo, "La direccion", v, 6, 120); err != nil {
		return err
	}
	n := normalizar(v)
	if !reDireccionUrbana.MatchString(n) && !reDireccionRural.MatchString(n) {
		return invalido(campo, "Escribe una direccion valida, por ejemplo: Calle 10 # 20-30, Cra 84 # 65-22 o Vereda La Esperanza.")
	}
	return nil
}

// ValidarHora exige el formato HH:MM de 24 horas.
func ValidarHora(campo, valor string) error {
	if !reHora.MatchString(strings.TrimSpace(valor)) {
		return invalido(campo, "La hora debe tener el formato HH:MM (por ejemplo 08:00).")
	}
	return nil
}

// minutos convierte HH:MM ya validada en minutos desde medianoche.
func minutos(hhmm string) int {
	h, _ := strconv.Atoi(hhmm[:2])
	m, _ := strconv.Atoi(hhmm[3:])
	return h*60 + m
}

// ValidarHorario acepta "Lun, Mie · 08:00 a 12:00": uno o mas dias de la
// semana y una franja con la hora de inicio antes que la de fin.
func ValidarHorario(campo, valor string) error {
	v := strings.TrimSpace(valor)
	if v == "" {
		return invalido(campo, "Indica el horario: dias y hora de inicio y de fin.")
	}
	m := reHorario.FindStringSubmatch(normalizar(v))
	if m == nil {
		return invalido(campo, "El horario debe indicar dias y horas, por ejemplo: Lun, Mie · 08:00 a 12:00.")
	}
	vistos := map[string]bool{}
	for _, d := range strings.Split(m[1], ",") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if !contiene(DiasHorario, d) {
			return invalido(campo, "Dia no reconocido en el horario: "+d+". Usa Lun, Mar, Mie, Jue, Vie, Sab o Dom.")
		}
		if vistos[d] {
			return invalido(campo, "El horario repite el dia "+d+".")
		}
		vistos[d] = true
	}
	if len(vistos) == 0 {
		return invalido(campo, "Indica al menos un dia en el horario.")
	}
	if ValidarHora(campo, m[2]) != nil || ValidarHora(campo, m[3]) != nil {
		return invalido(campo, "Las horas del horario deben tener el formato HH:MM.")
	}
	if minutos(m[2]) >= minutos(m[3]) {
		return invalido(campo, "En el horario, la hora de fin debe ser posterior a la de inicio.")
	}
	return nil
}

// ValidarNombrePersona acepta letras (con tildes), espacios, apostrofo y guion.
func ValidarNombrePersona(campo, etiqueta, valor string) error {
	v := strings.TrimSpace(valor)
	if err := ValidarTexto(campo, etiqueta, v, 2, 60); err != nil {
		return err
	}
	if !reNombre.MatchString(v) {
		return invalido(campo, etiqueta+" solo puede tener letras.")
	}
	return nil
}

// ValidarCiudad acepta nombres de ciudad (letras, espacios, punto y guion).
func ValidarCiudad(campo, valor string) error {
	v := strings.TrimSpace(valor)
	if err := ValidarTexto(campo, "La ciudad", v, 3, 60); err != nil {
		return err
	}
	if !reCiudad.MatchString(v) {
		return invalido(campo, "La ciudad solo puede tener letras.")
	}
	return nil
}

// ValidarTelefono acepta celulares (3xx) y fijos (60x) colombianos de 10
// digitos; ignora espacios, guiones, parentesis y el prefijo +57.
func ValidarTelefono(campo, valor string) (string, error) {
	v := reLimpiarTel.ReplaceAllString(strings.TrimSpace(valor), "")
	v = strings.TrimPrefix(strings.TrimPrefix(v, "+57"), "57")
	if len(v) == 12 && strings.HasPrefix(v, "57") {
		v = v[2:]
	}
	if !reTelefono.MatchString(v) {
		return "", invalido(campo, "Escribe un telefono colombiano de 10 digitos (celular 3xx o fijo 60x).")
	}
	return v, nil
}

// ValidarDocumento exige una cedula de 6 a 10 digitos, o un documento
// extranjero alfanumerico de 5 a 15 caracteres.
func ValidarDocumento(campo, valor string, esCedula bool) error {
	v := strings.TrimSpace(valor)
	if esCedula {
		if !reCedula.MatchString(v) {
			return invalido(campo, "La cedula debe tener entre 6 y 10 digitos, sin puntos.")
		}
		return nil
	}
	if !reDocExtranjero.MatchString(v) {
		return invalido(campo, "El documento debe tener entre 5 y 15 letras o numeros.")
	}
	return nil
}

// hoyColombia es la fecha de hoy en Colombia, que es la que ve el usuario.
func hoyColombia() time.Time {
	loc := time.FixedZone("COT", -5*3600)
	a, m, d := time.Now().In(loc).Date()
	return time.Date(a, m, d, 0, 0, 0, 0, time.UTC)
}

// ValidarFechaServicio exige AAAA-MM-DD desde hoy y hasta un ano adelante.
func ValidarFechaServicio(campo, valor string) error {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(valor))
	if err != nil {
		return invalido(campo, "La fecha debe tener el formato AAAA-MM-DD.")
	}
	hoy := hoyColombia()
	if t.Before(hoy) {
		return invalido(campo, "La fecha del servicio no puede estar en el pasado.")
	}
	if t.After(hoy.AddDate(1, 0, 0)) {
		return invalido(campo, "La fecha del servicio no puede ser de mas de un ano adelante.")
	}
	return nil
}

// ValidarMesPasado exige AAAA-MM (o AAAA-MM-DD) que no este en el futuro.
func ValidarMesPasado(campo, etiqueta, valor string) (time.Time, error) {
	v := strings.TrimSpace(valor)
	var t time.Time
	var err error
	if len(v) == 7 {
		t, err = time.Parse("2006-01", v)
	} else {
		t, err = time.Parse("2006-01-02", v)
	}
	if err != nil {
		return t, invalido(campo, etiqueta+" debe tener el formato AAAA-MM.")
	}
	if t.After(hoyColombia()) {
		return t, invalido(campo, etiqueta+" no puede estar en el futuro.")
	}
	if t.Year() < 1950 {
		return t, invalido(campo, etiqueta+" no es valida.")
	}
	return t, nil
}

// ValidarAno exige un ano de 4 digitos entre 1950 y el actual.
func ValidarAno(campo, valor string) error {
	v := strings.TrimSpace(valor)
	a, err := strconv.Atoi(v)
	if !reAno.MatchString(v) || err != nil || a < 1950 || a > hoyColombia().Year() {
		return invalido(campo, fmt.Sprintf("El ano debe estar entre 1950 y %d.", hoyColombia().Year()))
	}
	return nil
}

// ValidarUltimos4 exige los 4 ultimos digitos de una tarjeta.
func ValidarUltimos4(campo, valor string) error {
	if !reUltimos4.MatchString(strings.TrimSpace(valor)) {
		return invalido(campo, "Indica los 4 ultimos digitos de la tarjeta.")
	}
	return nil
}

// CiudadesServicio son las ciudades donde se puede publicar una oferta o
// pactar un servicio: los municipios de Casanare, las capitales y las areas
// metropolitanas grandes. El front muestra la misma lista (CIUDADES).
var CiudadesServicio = []string{
	"Yopal", "Aguazul", "Chámeza", "Hato Corozal", "La Salina", "Maní", "Monterrey", "Nunchía", "Orocué",
	"Paz de Ariporo", "Pore", "Recetor", "Sabanalarga", "Sácama", "San Luis de Palenque", "Támara",
	"Tauramena", "Trinidad", "Villanueva",
	"Bogotá", "Medellín", "Cali", "Barranquilla", "Cartagena", "Bucaramanga", "Cúcuta", "Pereira",
	"Manizales", "Armenia", "Ibagué", "Villavicencio", "Neiva", "Pasto", "Popayán", "Santa Marta",
	"Montería", "Sincelejo", "Valledupar", "Riohacha", "Tunja", "Florencia", "Quibdó", "Arauca", "Mocoa",
	"Leticia", "San José del Guaviare", "Mitú", "Puerto Carreño", "Inírida", "San Andrés",
	"Envigado", "Itagüí", "Bello", "Sabaneta", "Soacha", "Chía", "Zipaquirá", "Soledad", "Floridablanca",
	"Girón", "Piedecuesta", "Palmira", "Jamundí", "Dosquebradas", "Sogamoso", "Duitama",
}

// CiudadServicio valida que la ciudad este en la lista y la devuelve con su
// escritura oficial ("medellin" -> "Medellín").
func CiudadServicio(campo, valor string) (string, error) {
	n := normalizar(valor)
	for _, c := range CiudadesServicio {
		if normalizar(c) == n {
			return c, nil
		}
	}
	if strings.TrimSpace(valor) == "" {
		return "", invalido(campo, "La ciudad es obligatoria.")
	}
	return "", invalido(campo, "Jobsy aun no opera en "+strings.TrimSpace(valor)+". Elige una ciudad de la lista.")
}
