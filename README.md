# agenda-clinicas

Plataforma SaaS de agenda para clínicas y consultorios en Panamá: reserva en línea sin citas dobles, depósitos con Stripe, recordatorios por correo y WhatsApp, y datos de pacientes cifrados con una llave por clínica.

Estado: diseño. Todavía no hay código.

- Diseño: [docs/design.md](docs/design.md)
- Hoja de ruta y estimaciones: [docs/roadmap.md](docs/roadmap.md)
- Decisiones de arquitectura: [docs/adr](docs/adr)

## Stack

Go (API y worker), Next.js con TypeScript (web), MongoDB (replica set), OpenAPI como contrato entre ambos.

## Estructura

| Carpeta | Contenido |
|---|---|
| `backend/` | Todo el Go: API, worker y `tenantctl` |
| `web/` | Panel de la clínica y página pública de reservas |
| `openapi/` | Contrato de la API |
| `deploy/` | Docker Compose local e infraestructura |
| `docs/` | Diseño, hoja de ruta y ADRs |

## Comandos

Requisitos: Docker (en macOS, Colima), Go 1.27 y Node 26.

Primera vez:

```bash
cp .env.example .env
make db-up
```

| Comando | Qué hace |
|---|---|
| `make db-up` | Levanta MongoDB (replica set) y Mailpit y espera a que estén sanos |
| `make db-down` | Detiene los servicios y conserva los datos |
| `make db-reset` | Detiene los servicios y borra los datos de MongoDB |
| `make test-go` | Pruebas de Go, con MongoDB real en un contenedor |
| `make lint-go` | Linter de Go (`golangci-lint`) |

## Datos

La demo usa solo datos sintéticos. El sistema no debe recibir datos reales de pacientes.
