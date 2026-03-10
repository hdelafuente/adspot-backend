# adspot-backend

Backend REST en Go para gestión de **Ad Spots** — espacios publicitarios con soporte de TTL, estados y consulta de anuncios elegibles agrupados por placement.

---

## Requisitos

| Herramienta | Versión mínima |
|---|---|
| Go | 1.22 |
| GCC / Clang | cualquiera (requerido por `go-sqlite3` via CGO) |
| `sqlite3` CLI | solo para `make migrate` manual |

---

## Inicio rápido

```bash
# Instalar dependencias, correr migraciones y levantar el servidor
make run
```

El servidor escucha en `:8080` por defecto. Se puede cambiar con la variable de entorno `PORT`.

---

## Comandos Makefile

| Comando | Descripción |
|---|---|
| `make run` | Aplica migraciones y levanta el servidor |
| `make build` | Compila el binario en `bin/server` |
| `make migrate` | Aplica los archivos `.sql` de `migrations/` usando la CLI de sqlite3 |
| `make test` | Corre todos los tests con `-race` |
| `make clean` | Elimina `bin/` y `adspot.db` |

---

## Estructura del proyecto

```
.
├── cmd/
│   └── server/
│       └── main.go                 # Entrypoint: servidor HTTP, graceful shutdown
├── internal/
│   ├── adspot/
│   │   ├── model.go                # Tipos AdSpot, CreateRequest y constantes
│   │   ├── repository.go           # Acceso a datos (SQLite)
│   │   ├── handler.go              # Handlers HTTP y registro de rutas
│   │   └── handler_test.go         # Tests del endpoint de eligible ads
│   ├── database/
│   │   └── sqlite.go               # Apertura de DB y ejecución de migraciones
│   └── middleware/
│       └── ratelimit.go            # Rate limiter (token bucket por IP)
├── migrations/
│   └── 001_create_adspots.sql      # Esquema inicial
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Modelo de datos

```json
{
  "id":             "string (UUID v4)",
  "title":          "string",
  "imageUrl":       "string",
  "placement":      "home_screen | ride_summary | map_view",
  "status":         "active | inactive",
  "createdAt":      "ISO-8601",
  "deactivatedAt":  "ISO-8601 (opcional)",
  "ttlMinutes":     "number (opcional)"
}
```

**Reglas de negocio:**
- Los nuevos AdSpots son `active` por defecto.
- Si `ttlMinutes` está presente, el spot se considera inactivo desde `createdAt + ttlMinutes`.
- Solo los spots **activos y no expirados** son retornados como *eligible ads*.

---

## Endpoints

### `POST /adspots`

Crea un nuevo AdSpot.

**Body:**
```json
{
  "title": "Promo verano",
  "imageUrl": "https://cdn.example.com/promo.png",
  "placement": "home_screen",
  "ttlMinutes": 60
}
```

**Respuesta `201 Created`:**
```json
{
  "id": "a1b2c3d4-...",
  "title": "Promo verano",
  "imageUrl": "https://cdn.example.com/promo.png",
  "placement": "home_screen",
  "status": "active",
  "createdAt": "2026-03-10T14:00:00Z",
  "ttlMinutes": 60
}
```

---

### `GET /adspots/{id}`

Retorna un AdSpot por su ID.

**Respuesta `200 OK`** — el objeto AdSpot completo.
**Respuesta `404 Not Found`** — si el ID no existe.

---

### `POST /adspots/{id}/deactivate`

Marca el AdSpot como `inactive` y registra `deactivatedAt` con la fecha actual.

**Respuesta `200 OK`** — el objeto AdSpot actualizado.
**Respuesta `404 Not Found`** — si el ID no existe.

---

### `GET /adspots?placement=...&status=active`

Retorna los *eligible ads* agrupados por placement. Solo incluye spots que sean:
- `status = active`
- Sin TTL, **o** cuyo TTL no haya expirado aún.

El parámetro `placement` es opcional; si se omite se retornan todos los placements.

**Respuesta `200 OK`:**
```json
{
  "home_screen": [
    { "id": "...", "title": "...", "placement": "home_screen", "status": "active", ... }
  ],
  "map_view": [
    { "id": "...", "title": "...", "placement": "map_view", "status": "active", ... }
  ]
}
```

---

## Características técnicas

### Graceful shutdown
Al recibir `SIGINT` o `SIGTERM`, el servidor deja de aceptar conexiones nuevas y espera hasta 15 segundos para que las solicitudes en curso terminen antes de cerrar.

### Timeout de requests
Cada request tiene un timeout máximo de **5 segundos** aplicado como middleware global via `chi.Timeout`.

### Rate limiting
Token bucket por IP (soporta `X-Forwarded-For` para proxies): máximo **10 requests por segundo** por host. Las solicitudes que superen el límite reciben `429 Too Many Requests`.

### Migraciones
Al iniciar, el servidor ejecuta automáticamente todos los archivos `.sql` de `migrations/` en orden lexicográfico. Idempotentes gracias a `CREATE TABLE IF NOT EXISTS`.

---

## Tests

```bash
make test
```

El paquete `internal/adspot` incluye tests para el endpoint `GET /adspots` que verifican:

- Solo se retornan spots con `status = active`.
- Spots cuyo TTL ya expiró **no** aparecen en la respuesta.
- Spots desactivados manualmente **no** aparecen en la respuesta.
- El parámetro `placement` filtra correctamente los resultados.
- Los resultados están agrupados por placement.

Los tests usan una base de datos SQLite **en memoria** para aislamiento total.

---

## CI — GitHub Actions

Al hacer merge de un Pull Request a `main`, se ejecuta automáticamente el workflow `.github/workflows/test.yml` que:

1. Hace checkout del código.
2. Configura Go según la versión declarada en `go.mod`.
3. Instala GCC (requerido por `go-sqlite3` vía CGO).
4. Corre `go test -v -race ./...`.

```
main branch
   └── push (merge PR)
         └── workflow: Test
               ├── setup Go
               ├── install GCC
               └── go test -v -race ./...
```

---

## Dependencias

| Paquete | Versión | Uso |
|---|---|---|
| `github.com/go-chi/chi/v5` | v5.2.5 | Router HTTP |
| `github.com/mattn/go-sqlite3` | v1.14.34 | Driver SQLite (CGO) |
