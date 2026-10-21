# Jobsy Mid

Middleware (BFF) entre `Jobsy_Front` (Angular 22) y los 7 microservicios de `Jobsy_API` (Go + Beego).

## Por qué existe

`Jobsy_API` no es un backend de Jobsy: es un editor de tablas de PostgreSQL expuesto por HTTP.
Son 55 tablas × 5 rutas CRUD = 275 endpoints generados con `bee generate scaffold`, sin una sola
operación de negocio. No existe "postularse", existe "inserta una fila en `postulaciones`".

Concretamente, lo que **no** hay en `Jobsy_API` y por tanto tiene que vivir aquí:

| Falta allí | Consecuencia |
|---|---|
| Autenticación | No hay login, ni JWT, ni hash de contraseñas. `core.credenciales.password_hash` existe como columna pero ningún código Go la escribe. |
| Autorización | Ningún handler consulta el rol. El backend obedece cualquier `id_usuarios` que se le pase. |
| Validación | Cero. Se puede crear un usuario con correo vacío, o una reseña con calificación `-5`. |
| Lógica de negocio | Nadie valida stock, calcula un total, descuenta un saldo ni cambia un estado. |
| Transacciones | Cada escritura es independiente. Un checkout son N inserts sin atomicidad. |

> **El mid es la única frontera de seguridad del sistema.** Los 7 servicios deben quedar en red
> privada. Si se exponen, `GET /v1/credenciales` entrega todos los hashes y `GET /v1/sesiones`
> todos los tokens activos, sin autenticación de ningún tipo.

## Las rarezas de los upstream (y dónde se absorben)

Todo esto vive en `services/upstream.go`, para que el resto del código no lo sepa:

1. **Tres envelopes distintos.** `{"data":…}` en Core, Catálogo y Tienda; `{"date":…}` —typo— en
   los POST y PUT de Empleo, Suscripciones, Usuarios y Soporte (56 sitios); y a veces un string
   pelado con el error crudo de PostgreSQL. Se leen las tres formas.
2. **El HTTP status miente.** Es 200 casi siempre, aunque el body diga `"status": 400`. No se
   puede enrutar por código HTTP: hay que mirar `success` dentro del body.
3. **Un 200 con body vacío es un error.** El `Post` de `Core_api` no tiene rama `else` cuando el
   `Unmarshal` falla, así que un JSON inválido responde idéntico a un éxito.
4. **PascalCase.** Los modelos Go no llevan tags `json`, así que los campos van `{"Titulo": …}`.
5. **`PUT` es reemplazo total**, no patch: `o.Update(m)` escribe todas las columnas y pone en
   cero las omitidas. Hay que hacer GET → fusionar → PUT.
6. **Sin `total` en los listados.** Devuelven un array pelado, así que no se puede construir
   "página 3 de 47" honestamente. Ver `models.Pagina`: se pide `limit+1` para saber con certeza
   si hay página siguiente, y `total` se reporta como cota inferior.
7. **`GET /:id` roto en 26 de 55 entidades**, por `LoadRelated` sobre campos no relacionales.
   Por eso `services/empleados.go` lee por `?query=IdUsuarios:N` en vez de por id: la ruta de
   `GetAll` no llama a `LoadRelated`.

## Traducción de enums

El front habla en cadenas (`'jornada_completa'`); la base guarda FK enteras a `catalogo.*`.
`services/catalogo.go` traduce en ambos sentidos. Hay **un desajuste real sin resolver**:

```
front (4)                base (6)
por_horas          <->   por_horas
jornada_completa   <->   tiempo_completo
fin_de_semana      <->   fines_semana
presencial         <->   (no existe)
(no existe)        <->   por_jornadas | interno | dias_especificos
```

`presencial` es una modalidad de lugar, no de jornada: está en la dimensión equivocada. El mid
devuelve un error explícito en vez de inventarse un id. **Es una decisión de producto pendiente.**

Lo mismo con el tamaño: el front separa `tamano` (3 valores) de `esLujo` (booleano); la base mete
`lujoso` como cuarto tamaño, confundiendo magnitud con gama. El mid mantiene la forma del front
porque es la correcta, y traduce.

## Estructura

Mismo esquema que los demas mids Beego (`api_mid_correo`):

```
main.go               carga app.conf, arma los servicios y llama a beego.Run; apagado ordenado
conf/app.conf         configuracion; cada valor sale de una variable de entorno
routers/              rutas (beego.Router) y filtros transversales
controllers/          un controlador por dominio; el acceso lo decide su tipo:
                        BaseController   publico
                        SesionController exige token (Prepare)
                        AdminController  exige ademas rol admin (Prepare)
models/               los DTOs que ve el front (calco de core/models/*.ts) y el
                      contrato de errores; los detalles de PostgreSQL nunca salen
services/             logica de negocio y agregacion, mas la infraestructura:
                        upstream.go   cliente HTTP a los 7 servicios; normaliza sus rarezas
                        auth.go       JWT (HS256) + bcrypt coste 12, autorizacion por rol y fila
                        catalogo.go   traduccion enum <-> FK, con cache y TTL
                        pasarela*.go  cobro y verificacion de firmas del proveedor
                        almacen.go    archivos subidos
                        config.go     lee app.conf; se niega a arrancar sin MID_JWT_SECRETO
tests/                pruebas de la capa HTTP contra el router de Beego
cmd/sembrar-credenciales/  herramienta de desarrollo
```

## Configuración

`conf/app.conf` toma cada valor del entorno con `${VAR||defecto}`, asi que se configura igual
que antes:

```bash
MID_JWT_SECRETO=<mínimo 32 caracteres>   # obligatorio, sin valor por defecto
MID_PUERTO=9090
MID_ENTORNO=dev|prod
MID_ORIGENES=http://localhost:4200        # lista explícita; "*" prohibido en prod
MID_TIMEOUT_UPSTREAM=10s

MID_URL_CORE=http://localhost:8081
MID_URL_USUARIOS=http://localhost:8082
MID_URL_EMPLEO=http://localhost:8083
MID_URL_CATALOGO=http://localhost:8084
MID_URL_TIENDA=http://localhost:8085
MID_URL_SUSCRIPCIONES=http://localhost:8086
MID_URL_SOPORTE=http://localhost:8087
```

Los puertos de los upstream los decide el mid: `Jobsy_API` no define ninguno por defecto
(`httpport = ${VAR}` sin fallback), así que los 7 caen al 8080 de Beego y colisionan.

## Estado

Hecho y verificado (compila, `go vet` limpio, tests en verde):

- Cliente de upstream con normalización de envelopes, timeouts y llamadas en paralelo
- Contrato de errores con status HTTP correcto y sin filtración de detalles internos
- JWT + bcrypt, validación de contraseña y correo, autorización por rol y por propietario
- Middlewares: id de petición, recuperación de pánicos, log estructurado, CORS restrictivo,
  límite de peticiones por IP
- Traducción enum↔catálogo con caché
- Agregación de empleado (`core.usuarios` + `empleo.perfiles_empleado` + especialidades N:M)
- Arranque y apagado ordenado, degradación cuando los upstream no responden

Pendiente:

- `login` y `registro` están esqueletados y devuelven error explícito. No se pueden completar
  hasta que `core.credenciales` tenga `password_hash` poblado: hoy no hay contra qué comparar.
  El registro además necesita escritura compensada en dos tablas sin transacción.
- Dominios sin implementar: ofertas, postulaciones, contrataciones, tienda, pedidos,
  suscripciones, reportes, admin.
- No se ha probado contra los upstream reales: requiere PostgreSQL y los 7 servicios corriendo.
  Y **tres de siete no arrancan contra una base creada desde su repo** (el DDL de `Tienda_APi`
  es una copia del de Core y no crea ninguna de sus 13 tablas; `Core_api` y `Usuarios_Api`
  tienen SQL inválido).
