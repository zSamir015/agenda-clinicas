# backend

Todo el código Go: la API, el worker y la herramienta de administración `tenantctl`. Es un solo módulo Go con la estructura estándar:

| Carpeta | Contenido |
|---|---|
| `cmd/api/` | Punto de entrada de la API HTTP |
| `cmd/worker/` | Punto de entrada del worker (outbox, notificaciones, webhooks, holds vencidos) |
| `cmd/tenantctl/` | CLI de administración de clínicas |
| `internal/` | Paquetes del proyecto. Go no permite importarlos desde fuera del módulo |

Cada programa en `cmd/` solo arma las piezas y arranca; la lógica vive en `internal/`. Las pruebas de cada archivo están junto a él (`x.go` y `x_test.go`).

Se crea en la fase 0. Ver `docs/design.md`.
