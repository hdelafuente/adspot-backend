# Postmortem — adspot-backend

Análisis de riesgos de la implementación actual y hoja de ruta para llevar el servicio a nivel productivo en AWS.

---

## Índice

1. [Riesgos actuales](#riesgos-actuales)
   - [🔴 Autenticación y autorización ausentes](#1-autenticación-y-autorización-ausentes--crítico)
   - [🟢 SQL Injection](#2-sql-injection--seguro)
   - [🟠 Validación de inputs insuficiente](#3-validación-de-inputs-insuficiente--alto)
   - [🟠 Rate limiter vulnerable a spoofing](#4-rate-limiter-vulnerable-a-x-forwarded-for-spoofing--alto)
   - [🟠 SQLite no apta para multi-instancia](#5-sqlite-no-apta-para-entornos-multi-instancia--alto-bloqueante-en-prod)
   - [🟢 Logging estructurado](#6-logging-estructurado--resuelto)
   - [🟡 Configuración hardcodeada](#7-configuración-hardcodeada--medio)
   - [🟡 Health checks ausentes](#8-health-checks-ausentes--medio)
2. [Pasos para producción en AWS](#pasos-para-producción-en-aws)

---

## Riesgos actuales

### 1. Autenticación y autorización ausentes — 🔴 CRÍTICO

**Descripción**
Ninguno de los 4 endpoints verifica la identidad del llamante. No existe middleware de autenticación, validación de tokens JWT ni control de roles.

**Impacto**
| Endpoint | Riesgo |
|---|---|
| `POST /adspots` | Cualquier cliente puede crear adspots sin restricción |
| `GET /adspots/{id}` | Exposición de datos a usuarios no autorizados |
| `POST /adspots/{id}/deactivate` | Cualquier cliente puede desactivar todos los adspots |
| `GET /adspots` | Lectura irrestricta del inventario de ads |

**Recomendación**
Implementar autenticación basada en **JWT (Bearer token)** con dos roles:

| Rol | Permisos |
|---|---|
| `viewer` | `GET /adspots`, `GET /adspots/{id}` |
| `admin` | Todos los endpoints anteriores + `POST /adspots`, `POST /adspots/{id}/deactivate` |

Flujo sugerido:
1. Agregar un middleware `RequireAuth` que valide el token JWT en el header `Authorization: Bearer <token>`.
2. Agregar un middleware `RequireRole("admin")` para los endpoints de escritura.
3. El claim `role` dentro del JWT determina el nivel de acceso.
4. Usar una librería como `github.com/golang-jwt/jwt/v5` para la validación.

```
POST /adspots             → RequireAuth → RequireRole("admin") → handler
GET  /adspots/{id}        → RequireAuth → handler
POST /adspots/{id}/deact  → RequireAuth → RequireRole("admin") → handler
GET  /adspots             → RequireAuth → handler
```

---

### 2. SQL Injection — 🟢 SEGURO

**Descripción**
Revisado el código de `internal/adspot/repository.go`: **no existe vulnerabilidad de SQL injection**. Todas las queries usan sentencias parametrizadas con placeholders `?`.

```go
// Ejemplo en repository.go — Create
r.db.ExecContext(ctx,
    `INSERT INTO adspots (...) VALUES (?, ?, ?, ?, ?, ?, ?)`,
    spot.ID, spot.Title, spot.ImageURL, ...  // nunca concatenados
)

// Ejemplo en repository.go — ListEligible
q += " AND placement = ?"
args = append(args, placement) // agregado al slice de args, no a la query
```

El único punto que podría generar dudas es la expresión SQLite en `ListEligible`:
```sql
datetime(created_at, '+' || ttl_minutes || ' minutes')
```
Este cálculo es seguro porque `ttl_minutes` es una **columna de la base de datos** de tipo `INTEGER`, no un valor proveniente del request HTTP.

**Acción requerida:** Ninguna. Mantener la práctica de queries parametrizadas en futuros desarrollos.

---

### 3. Validación de inputs insuficiente — 🟠 ALTO

**Descripción**
El handler de `POST /adspots` solo verifica que `title`, `imageUrl` y `placement` no sean strings vacíos. No hay límites de longitud, validación de formato de URL ni restricciones sobre `ttlMinutes`.

**Impacto**
- `title` sin límite de largo → posible abuso de almacenamiento / DoS lento.
- `imageUrl` sin validación de formato → se pueden guardar valores que no son URLs válidas.
- `ttlMinutes` acepta valores negativos o cero → comportamiento indefinido (el spot nunca expiraría o ya nacería expirado).

**Recomendación**

| Campo | Regla |
|---|---|
| `title` | Requerido, max 255 caracteres |
| `imageUrl` | Requerido, URL válida (`url.ParseRequestURI`), max 2048 caracteres |
| `ttlMinutes` | Opcional; si está presente: mínimo 1, máximo 10080 (1 semana) |

---

### 4. Rate limiter vulnerable a X-Forwarded-For spoofing — 🟠 ALTO

**Descripción**
El middleware en `internal/middleware/ratelimit.go` prioriza el header `X-Forwarded-For` para identificar el host:

```go
func remoteIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        return xff  // ← valor arbitrario enviado por el cliente
    }
    ...
}
```

**Impacto**
Un atacante puede enviar un header `X-Forwarded-For` diferente en cada request, generando un bucket de tokens nuevo en cada llamada y evadiendo por completo el límite de 10 req/s.

```
# Ataque: cada request tiene su propio bucket → límite inefectivo
curl -H "X-Forwarded-For: 1.2.3.1" http://api/adspots
curl -H "X-Forwarded-For: 1.2.3.2" http://api/adspots
curl -H "X-Forwarded-For: 1.2.3.3" http://api/adspots
```

Adicionalmente, el header puede contener múltiples IPs (`client, proxy1, proxy2`) y el código devuelve el string completo en lugar de extraer solo la IP del cliente.

**Recomendación**
1. Controlar via variable de entorno si se confía en `X-Forwarded-For` (`TRUST_PROXY=true`), habilitarlo solo cuando hay un reverse proxy conocido.
2. Parsear únicamente la primera IP del header: `strings.TrimSpace(strings.Split(xff, ",")[0])`.
3. En AWS, el ALB añade la IP real del cliente como última entrada del header; configurar la lógica para leer esa posición.

---

### 5. SQLite no apta para entornos multi-instancia — 🟠 ALTO (bloqueante en prod)

**Descripción**
SQLite almacena toda la base de datos en un único archivo local (`adspot.db`). No expone ningún protocolo de red y no admite escrituras concurrentes desde múltiples procesos.

**Impacto**
- Escalar horizontalmente a 2+ instancias es imposible: cada instancia tendría su propia copia desincronizada de los datos.
- No hay backups automáticos ni replicación.
- Una pérdida del disco implica pérdida total de datos.

**Recomendación**
Migrar a **Amazon RDS for PostgreSQL** para producción:
- Admite múltiples conexiones concurrentes y escrituras desde cualquier instancia.
- Backups automáticos, snapshots y failover multi-AZ.
- Las queries son compatibles en su mayor parte; el driver se reemplaza por `lib/pq` o `pgx`.

---

### 6. Logging estructurado — 🟢 RESUELTO

**Implementación**
Se reemplazó el `log` package estándar y el middleware de texto de Chi por un sistema de logging JSON completo basado en `log/slog` (stdlib, sin dependencias nuevas). Archivos introducidos:

| Archivo | Rol |
|---|---|
| `internal/logger/logger.go` | Factory (`New`), helpers de contexto (`WithContext`, `FromContext`) |
| `internal/middleware/logger.go` | Middleware HTTP — emite un evento JSON por request |

El nivel de logging es configurable via variable de entorno `LOG_LEVEL` (`debug` / `info` / `warn` / `error`; default: `info`).

**Schema de logs**

Todos los eventos son una línea JSON en stdout. Campos por tipo:

```jsonc
// HTTP request (un evento por request)
{
  "time": "2026-03-10T14:00:00.200Z", "level": "INFO", "msg": "request",
  "request_id": "abc-123", "method": "POST", "path": "/adspots",
  "status": 201, "duration_ms": 12, "remote_ip": "10.0.0.5",
  "user_agent": "curl/7.88", "bytes": 245
}

// Evento de negocio — mutación exitosa
{
  "time": "...", "level": "INFO", "msg": "adspot created",
  "request_id": "abc-123", "adspot_id": "uuid", "placement": "home_screen"
}

// Error interno — emitido antes de responder 500
{
  "time": "...", "level": "ERROR", "msg": "create adspot failed",
  "request_id": "abc-123", "error": "insert adspot: database is locked"
}

// Startup / shutdown
{ "time": "...", "level": "INFO", "msg": "server listening", "addr": ":8080" }
```

**Queries de ejemplo**

```bash
# Todos los errores 5xx
cat app.log | jq 'select(.status >= 500)'

# Requests lentos (> 500 ms)
cat app.log | jq 'select(.duration_ms > 500)'

# Trazar un request completo por ID
cat app.log | jq 'select(.request_id == "abc-123")'

# Errores internos con su causa
cat app.log | jq 'select(.level == "ERROR") | {request_id, msg, error}'
```

En **CloudWatch Logs Insights**:
```
fields @timestamp, request_id, status, duration_ms, path
| filter status >= 500
| sort @timestamp desc
```

**Pendiente**
El audit trail de quién ejecutó cada acción (`actor`) solo será completo una vez que se implemente autenticación JWT (Riesgo #1). Con el `request_id` como correlación ya es posible asociar cada mutación a un request individual.

---

### 7. Configuración hardcodeada — 🟡 MEDIO

**Descripción**
Varios parámetros críticos están fijos en el código fuente (`cmd/server/main.go`):

| Valor | Ubicación | Hardcoded |
|---|---|---|
| Ruta de la DB | `main.go:22` | `"adspot.db"` |
| Rate limit RPS | `main.go:41` | `10` |
| Read/Write timeout | `main.go:56-58` | `10s / 10s / 60s` |
| Request timeout | `main.go:37` | `5s` |

**Impacto**
Cambiar cualquier parámetro requiere recompilar el binario. Imposible tener configuraciones distintas para dev, staging y producción sin modificar el código.

**Recomendación**
Leer todos los valores desde variables de entorno con defaults sensatos:

```
DB_PATH            (default: adspot.db)
PORT               (default: 8080)
RATE_LIMIT_RPS     (default: 10)
REQUEST_TIMEOUT    (default: 5s)
READ_TIMEOUT       (default: 10s)
WRITE_TIMEOUT      (default: 10s)
```

En AWS, gestionar estos valores con **AWS Systems Manager Parameter Store** o **Secrets Manager** (para los sensibles como el DSN de la base de datos).

---

### 8. Health checks ausentes — 🟡 MEDIO

**Descripción**
No existe ningún endpoint de salud. El servidor no expone una forma de que la infraestructura verifique si está operativo.

**Impacto**
- El Application Load Balancer (ALB) de AWS no puede determinar si una instancia está sana antes de enviarle tráfico.
- ECS Fargate marcará las tareas como unhealthy de forma incorrecta o no detectará fallos reales.

**Recomendación**
Agregar dos endpoints:

```
GET /health/live   → 200 OK siempre que el proceso esté corriendo
GET /health/ready  → 200 OK solo si la conexión a la DB responde (db.Ping())
```

El ALB debe apuntar al endpoint `/health/ready` para su health check.

---

## Pasos para producción en AWS

Los siguientes pasos asumen una cuenta AWS con permisos para crear los recursos mencionados.

---

### Paso 1 — Migrar la base de datos a Amazon RDS (PostgreSQL)

Crear una instancia **RDS for PostgreSQL** en una subred privada (sin acceso público). Reemplazar el driver `go-sqlite3` por `pgx/v5` o `lib/pq`. Migrar el schema de `migrations/001_create_adspots.sql` (la sintaxis es compatible). El DSN de conexión se gestiona como secreto en el Paso 6.

```
VPC
└── Private Subnet
      └── RDS PostgreSQL (Multi-AZ para alta disponibilidad)
```

---

### Paso 2 — Implementar autenticación JWT y roles

Antes de exponer el servicio a internet, implementar el middleware de autenticación descrito en el [Riesgo #1](#1-autenticación-y-autorización-ausentes--crítico). El JWT secret se almacena en AWS Secrets Manager.

---

### Paso 3 — Contenedorizar la aplicación

Crear un `Dockerfile` multistage para producir un binario mínimo:

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o server ./cmd/server

# Final stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /server
COPY migrations/ /migrations/
ENTRYPOINT ["/server"]
```

---

### Paso 4 — Publicar la imagen en Amazon ECR

Crear un repositorio en **Amazon Elastic Container Registry** y configurar el pipeline de CI/CD para publicar la imagen en cada merge a `main`.

```bash
aws ecr create-repository --repository-name adspot-backend
docker build -t adspot-backend .
docker tag adspot-backend:latest <account>.dkr.ecr.<region>.amazonaws.com/adspot-backend:latest
docker push <account>.dkr.ecr.<region>.amazonaws.com/adspot-backend:latest
```

---

### Paso 5 — Deploy en Amazon ECS Fargate

Crear un cluster de **ECS Fargate** con una Task Definition que use la imagen del paso anterior. Fargate elimina la necesidad de gestionar servidores EC2.

```
ECS Cluster
└── Service (desired: 2 tasks mínimo)
      └── Task (Fargate)
            └── Container: adspot-backend
                  ├── PORT=8080
                  ├── DB_DSN → Secrets Manager
                  └── LOG_LEVEL=info
```

---

### Paso 6 — Gestionar secretos con AWS Secrets Manager

Almacenar en Secrets Manager:
- `adspot/db-dsn` → connection string de PostgreSQL
- `adspot/jwt-secret` → clave de firma JWT

Referenciarlos en la Task Definition de ECS como variables de entorno inyectadas en runtime (nunca en el código ni en la imagen Docker).

---

### Paso 7 — Configurar el Application Load Balancer con HTTPS

Crear un **ALB** en subredes públicas con:
- Listener en puerto **443 (HTTPS)** usando un certificado de **AWS Certificate Manager (ACM)**.
- Redirect automático de HTTP (80) → HTTPS (443).
- Target Group apuntando a las tareas ECS en el puerto 8080.
- Health check configurado en `GET /health/ready` con umbral de 2 checks consecutivos.

```
Internet
  └── ALB (público, HTTPS/443, certificado ACM)
        └── Target Group → ECS Tasks (privadas, puerto 8080)
```

---

### Paso 8 — Centralizar logs en CloudWatch Logs

Configurar el log driver `awslogs` en la Task Definition de ECS:

```json
"logConfiguration": {
  "logDriver": "awslogs",
  "options": {
    "awslogs-group": "/ecs/adspot-backend",
    "awslogs-region": "us-east-1",
    "awslogs-stream-prefix": "ecs"
  }
}
```

Con logging estructurado en JSON (ver Riesgo #6), los logs son consultables con **CloudWatch Logs Insights**.

---

### Paso 9 — Alertas con CloudWatch Alarms

Configurar alarmas mínimas sobre las métricas del ALB y ECS:

| Alarma | Métrica | Umbral sugerido |
|---|---|---|
| Alta tasa de errores | `HTTPCode_Target_5XX_Count` | > 10 en 5 min |
| Rate limit activo | `HTTPCode_Target_429_Count` | > 50 en 1 min |
| Latencia elevada | `TargetResponseTime` p99 | > 3s |
| CPU alta en ECS | `CPUUtilization` | > 80% |
| Sin instancias sanas | `HealthyHostCount` | < 1 |

Las alarmas deben notificar a un topic de **Amazon SNS** (email, Slack, PagerDuty).

---

### Paso 10 — Auto-scaling en ECS

Configurar **Application Auto Scaling** sobre el servicio ECS para escalar horizontalmente según demanda:

- **Scale out**: cuando `CPUUtilization > 70%` durante 2 minutos → agregar 1 tarea.
- **Scale in**: cuando `CPUUtilization < 30%` durante 5 minutos → remover 1 tarea.
- **Mínimo:** 2 tareas (alta disponibilidad). **Máximo:** configurable según carga esperada.

---

### Paso 11 — Pipeline CI/CD completo

Extender el GitHub Action existente (`.github/workflows/test.yml`) para incluir build y deploy:

```
Push a main
  └── [Job 1] test        → go test -race ./...
  └── [Job 2] build       → docker build + push a ECR      (depende de Job 1)
  └── [Job 3] deploy      → ecs update-service --force-new-deployment  (depende de Job 2)
```

El deploy usa rolling update de ECS: las tareas nuevas pasan el health check del ALB antes de que las viejas sean terminadas, garantizando zero-downtime deployments.

---

### Resumen de infraestructura final

```
GitHub Actions (CI/CD)
       │
       ▼
Amazon ECR (imágenes Docker)
       │
       ▼
┌──────────────────────────────────────────┐
│  VPC                                     │
│  ┌─────────────────────────────────────┐ │
│  │  Public Subnets                     │ │
│  │  └── ALB (HTTPS + certificado ACM)  │ │
│  └─────────────────────────────────────┘ │
│  ┌─────────────────────────────────────┐ │
│  │  Private Subnets                    │ │
│  │  ├── ECS Fargate (2+ tasks)         │ │
│  │  └── RDS PostgreSQL (Multi-AZ)      │ │
│  └─────────────────────────────────────┘ │
└──────────────────────────────────────────┘
       │                    │
       ▼                    ▼
CloudWatch Logs      Secrets Manager
CloudWatch Alarms    Parameter Store
SNS (notificaciones)
```
