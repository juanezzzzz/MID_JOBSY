package services

import (
	"testing"
	"time"

	"api_MID/models"
)

func TestValidarDireccion(t *testing.T) {
	validas := []string{
		"Calle 10 # 20-30", "Cra 84 # 65-22", "Carrera 5 #1-2", "Cl 7 No. 33-15 apto 201",
		"Av. 1 # 2-3", "Diagonal 5B bis # 4-10", "Transversal 3 N° 12-04", "Vía 40 # 10-5",
		"Vereda La Esperanza", "Km 5 vía Aguazul", "Manzana 4 Casa 12",
	}
	invalidas := []string{"", "1", "asdf", "x", "Calle", "Calle 10", "Calle 10 # 20", "mi casa azul", "# 20-30"}
	for _, d := range validas {
		if err := ValidarDireccion("direccion", d); err != nil {
			t.Errorf("deberia aceptar %q: %v", d, err)
		}
	}
	for _, d := range invalidas {
		if ValidarDireccion("direccion", d) == nil {
			t.Errorf("deberia rechazar %q", d)
		}
	}
}

func TestValidarHorario(t *testing.T) {
	validos := []string{"Lun, Mie · 08:00 a 12:00", "Sab · 07:30 a 15:00", "lun,mar,mié · 13:00 a 17:00", "Dom - 09:00 a 10:00"}
	invalidos := []string{"", "cuando sea", "Lun · 14:00 a 08:00", "Lun · 08:00 a 08:00", "Fiesta · 08:00 a 12:00",
		"Lun, Lun · 08:00 a 12:00", "Lun · 25:00 a 26:00", "08:00 a 12:00"}
	for _, h := range validos {
		if err := ValidarHorario("horario", h); err != nil {
			t.Errorf("deberia aceptar %q: %v", h, err)
		}
	}
	for _, h := range invalidos {
		if ValidarHorario("horario", h) == nil {
			t.Errorf("deberia rechazar %q", h)
		}
	}
}

func TestValidarTelefono(t *testing.T) {
	casos := map[string]string{"3101234567": "3101234567", "+57 310 123 4567": "3101234567", "310-123-4567": "3101234567", "6017654321": "6017654321"}
	for entrada, esperado := range casos {
		got, err := ValidarTelefono("telefono", entrada)
		if err != nil || got != esperado {
			t.Errorf("ValidarTelefono(%q) = (%q, %v), se esperaba %q", entrada, got, err, esperado)
		}
	}
	for _, malo := range []string{"abc", "123", "2101234567", "31012345678", ""} {
		if _, err := ValidarTelefono("telefono", malo); err == nil {
			t.Errorf("deberia rechazar %q", malo)
		}
	}
}

func TestValidarHoraNombreDocumento(t *testing.T) {
	for _, h := range []string{"00:00", "08:30", "23:59"} {
		if ValidarHora("h", h) != nil {
			t.Errorf("hora valida rechazada: %q", h)
		}
	}
	for _, h := range []string{"24:00", "99:99", "8:00", "08-00", ""} {
		if ValidarHora("h", h) == nil {
			t.Errorf("hora invalida aceptada: %q", h)
		}
	}
	for _, n := range []string{"Laura", "María José", "O'Neil", "Rincón-Pérez"} {
		if ValidarNombrePersona("n", "El nombre", n) != nil {
			t.Errorf("nombre valido rechazado: %q", n)
		}
	}
	for _, n := range []string{"123", "a", "Laura2", "  ", "Ana  María"} {
		if ValidarNombrePersona("n", "El nombre", n) == nil {
			t.Errorf("nombre invalido aceptado: %q", n)
		}
	}
	if ValidarDocumento("d", "1020304050", true) != nil || ValidarDocumento("d", "abc", true) == nil || ValidarDocumento("d", "1", true) == nil {
		t.Error("la cedula no se valida bien")
	}
	if ValidarDocumento("d", "AB12345", false) != nil || ValidarDocumento("d", "a", false) == nil {
		t.Error("el documento extranjero no se valida bien")
	}
}

func TestFechasYRangos(t *testing.T) {
	hoy := hoyColombia()
	if ValidarFechaServicio("f", hoy.Format("2006-01-02")) != nil {
		t.Error("hoy deberia ser una fecha de servicio valida")
	}
	if ValidarFechaServicio("f", hoy.AddDate(0, 0, -1).Format("2006-01-02")) == nil {
		t.Error("ayer no deberia ser valida")
	}
	if ValidarFechaServicio("f", hoy.AddDate(2, 0, 0).Format("2006-01-02")) == nil {
		t.Error("dentro de dos anos no deberia ser valida")
	}
	if _, err := ValidarMesPasado("m", "Inicio", hoy.AddDate(1, 0, 0).Format("2006-01")); err == nil {
		t.Error("un mes futuro no deberia ser valido")
	}
	if ValidarAno("a", "abcd") == nil || ValidarAno("a", "2012") != nil {
		t.Error("el ano no se valida bien")
	}
	if ValidarTarifa("t", 1) == nil || ValidarTarifa("t", 99_999_999) == nil || ValidarTarifa("t", 25000) != nil {
		t.Error("la tarifa no se valida bien")
	}
	menor := time.Now().AddDate(-10, 0, 0).Format("2006-01-02")
	if _, err := ValidarFechaNacimiento(menor); err == nil {
		t.Error("un menor de edad no deberia poder registrarse")
	}
}

func TestCiudadServicio(t *testing.T) {
	if c, err := CiudadServicio("ciudad", "medellin"); err != nil || c != "Medellín" {
		t.Errorf("medellin -> (%q, %v)", c, err)
	}
	for _, malo := range []string{"Narnia", "", "x"} {
		if _, err := CiudadServicio("ciudad", malo); err == nil {
			t.Errorf("deberia rechazar %q", malo)
		}
	}
}

func TestValidarEntrega(t *testing.T) {
	if validarEntrega(nil) == nil {
		t.Error("sin datos de entrega deberia fallar")
	}
	e := &models.DatosEntrega{Nombre: " Laura Martínez ", Telefono: "+57 310 123 4567", Direccion: "Calle 10 # 20-30", Detalle: "Apto 301", Ciudad: "yopal"}
	if err := validarEntrega(e); err != nil {
		t.Fatalf("datos validos rechazados: %v", err)
	}
	if e.Telefono != "3101234567" || e.Ciudad != "Yopal" || e.Nombre != "Laura Martínez" {
		t.Errorf("no se normalizo: %+v", e)
	}
	for _, malo := range []models.DatosEntrega{
		{Nombre: "Laura", Telefono: "123", Direccion: "Calle 10 # 20-30", Ciudad: "Yopal"},
		{Nombre: "Laura", Telefono: "3101234567", Direccion: "mi casa", Ciudad: "Yopal"},
		{Nombre: "Laura", Telefono: "3101234567", Direccion: "Calle 10 # 20-30", Ciudad: "Narnia"},
		{Nombre: "123", Telefono: "3101234567", Direccion: "Calle 10 # 20-30", Ciudad: "Yopal"},
	} {
		m := malo
		if validarEntrega(&m) == nil {
			t.Errorf("deberia rechazar %+v", malo)
		}
	}
}
