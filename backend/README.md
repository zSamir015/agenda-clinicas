# backend

Todo el código Go: la API, el worker y la herramienta de administración `tenantctl`. Es un solo módulo Go con la estructura estándar:

| Carpeta | Contenido |
|---|---|
| `cmd/api/` | Punto de entrada de la API HTTP |
| `cmd/worker/` | Worker: libera cada minuto las reservas temporales vencidas (`make worker`) |
| `cmd/tenantctl/` | CLI de administración de clínicas (fase 1) |
| `internal/` | Paquetes del proyecto. Go no permite importarlos desde fuera del módulo |

Cada programa en `cmd/` solo arma las piezas y arranca; la lógica vive en `internal/`. Las pruebas de cada archivo están junto a él (`x.go` y `x_test.go`).

## Paquetes de `internal/`

| Paquete | Responsabilidad |
|---|---|
| `config` | Lee y valida la configuración desde variables de entorno |
| `logging` | Logger `slog`: JSON en producción, texto en desarrollo |
| `mongodb` | Cliente de MongoDB; `mongotest` levanta uno real en un contenedor para las pruebas |
| `httpapi` | Handlers de la API. `openapi.gen.go` se genera desde `openapi/openapi.yaml` con `make gen-go` y no se edita a mano |
| `platform` | Registro de clínicas (`platform.clinics`), `TenantResolver` con caché de 60 s y `TenantContext` en el `context.Context` |
| `crypto` | Cifrado de campos por clínica: interfaz `Encryptor`, `Fake` en memoria para pruebas y `ClientEncryptor` (CSFLE, etiqueta `cse`). Llave maestra local en archivo `0600` |

## Correr la API

```bash
cp .env.example .env     # solo la primera vez
make db-up
make api
```

| Endpoint | Respuesta |
|---|---|
| `GET /healthz` | `200 {"status":"ok"}` si el proceso está vivo. No consulta la base |
| `GET /readyz` | `200` si MongoDB responde; `503 application/problem+json` con `code: database_unavailable` si no responde en 2 segundos |
