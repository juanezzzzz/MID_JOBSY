package services

import "testing"

func TestNormalizar(t *testing.T) {
	casos := map[string]string{
		"Cuidado de niños":            "cuidado de ninos",
		"  Cocina   y  Alimentación ": "cocina y alimentacion",
		"FEMENINO":                    "femenino",
	}
	for entrada, esperado := range casos {
		if got := normalizar(entrada); got != esperado {
			t.Errorf("normalizar(%q) = %q, se esperaba %q", entrada, got, esperado)
		}
	}
}

func TestNivelExperiencia(t *testing.T) {
	casos := []struct {
		texto string
		nivel string
		anios int
	}{
		{"", "sin_experiencia", 0},
		{"Ej: 6 anos de experiencia en hogares de Yopal", "experto", 6},
		{"2 años cuidando niños", "intermedio", 2},
		{"He trabajado en varias casas", "intermedio", 0},
	}
	for _, c := range casos {
		nivel, anios := nivelExperiencia(c.texto)
		if nivel != c.nivel || anios != c.anios {
			t.Errorf("nivelExperiencia(%q) = (%q, %d), se esperaba (%q, %d)", c.texto, nivel, anios, c.nivel, c.anios)
		}
	}
}

func TestValidarFechaNacimiento(t *testing.T) {
	if f, err := ValidarFechaNacimiento("1995-04-12"); err != nil || f != "1995-04-12T00:00:00Z" {
		t.Errorf("fecha valida: (%q, %v)", f, err)
	}
	for _, mala := range []string{"12/04/1995", "2999-01-01", "no es fecha"} {
		if _, err := ValidarFechaNacimiento(mala); err == nil {
			t.Errorf("ValidarFechaNacimiento(%q) deberia fallar", mala)
		}
	}
}
