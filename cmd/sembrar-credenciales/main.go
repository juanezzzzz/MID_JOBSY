// Comando sembrar-credenciales crea las credenciales de los usuarios de
// prueba, que la semilla de Jobsy_API no crea.
//
// core.credenciales tiene columnas password_hash y salt, pero ningun codigo Go
// del backend las escribe, y su migracion de semilla
// (creacion_credenciales.go) contiene por error una copia del insert de
// roles_usuarios. La tabla queda vacia, de modo que no hay nada contra lo que
// autenticar.
//
// Este comando la rellena usando el mismo bcrypt que valida el middleware, de
// forma que el login funciona de punta a punta. Es una herramienta de
// desarrollo: en produccion las credenciales las crea el registro.
//
//	go run ./cmd/sembrar-credenciales -url http://localhost:8081 -password Jobsy2026
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"api_MID/services"
)

type filaUsuario struct {
	Id       int    `json:"Id"`
	Nombre   string `json:"Nombre"`
	Apellido string `json:"Apellido"`
	Correo   string `json:"Correo"`
	Activo   bool   `json:"Activo"`
}

type filaCredencial struct {
	Id         int `json:"Id"`
	IdUsuarios struct {
		Id int `json:"Id"`
	} `json:"IdUsuarios"`
}

// main crea con bcrypt la credencial de cada usuario de prueba de Core_api
// con la contrasena indicada por -password.
func main() {
	urlCore := flag.String("url", "http://localhost:8081", "URL de Core_api")
	password := flag.String("password", "Jobsy2026", "contrasena en claro para todos los usuarios de prueba")
	flag.Parse()

	if err := services.ValidarPassword(*password); err != nil {
		log.Fatalf("la contrasena no cumple la politica: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	core := services.NuevoClienteUpstream("core", *urlCore, 15*time.Second)
	svc := services.NuevoServicioAuth([]byte("solo-para-hashear-no-se-firma-nada-aqui"), time.Hour)

	var usuarios []filaUsuario
	o := &services.OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "500")
	if err := core.Get(ctx, "usuarios", o, &usuarios); err != nil {
		log.Fatalf("no se pudieron leer los usuarios: %v", err)
	}
	if len(usuarios) == 0 {
		log.Fatal("no hay usuarios en core.usuarios; siembra primero el esquema core")
	}

	// Las credenciales existentes se saltan: este comando es idempotente para
	// poder relanzarlo sin duplicar filas.
	var existentes []filaCredencial
	oc := &services.OpcionesUpstream{Query: map[string][]string{}}
	oc.Query.Set("limit", "500")
	if err := core.Get(ctx, "credenciales", oc, &existentes); err != nil {
		log.Printf("aviso: no se pudieron leer las credenciales existentes (%v); se asume que no hay", err)
	}
	yaTiene := make(map[int]bool, len(existentes))
	for _, c := range existentes {
		yaTiene[c.IdUsuarios.Id] = true
	}

	creadas, saltadas := 0, 0
	for _, u := range usuarios {
		if !u.Activo {
			continue
		}
		if yaTiene[u.Id] {
			fmt.Printf("  = %-34s ya tenia credencial\n", u.Correo)
			saltadas++
			continue
		}

		hash, err := svc.Hash(*password)
		if err != nil {
			log.Fatalf("no se pudo hashear: %v", err)
		}

		// El campo relacional se manda como objeto anidado: asi lo espera el
		// ORM del upstream para un rel(fk).
		cuerpo := map[string]any{
			"IdUsuarios":       map[string]any{"Id": u.Id},
			"PasswordHash":     hash,
			"DobleFactor":      false,
			"IntentosFallidos": 0,
			"Bloqueado":        false,
			"Activo":           true,
		}

		var creada map[string]any
		if err := core.Post(ctx, "credenciales", cuerpo, &creada); err != nil {
			fmt.Fprintf(os.Stderr, "  ! %-34s fallo: %v\n", u.Correo, err)
			continue
		}
		fmt.Printf("  + %-34s credencial creada\n", u.Correo)
		creadas++
	}

	fmt.Printf("\n%d credenciales creadas, %d ya existian. Contrasena: %s\n", creadas, saltadas, *password)
}
