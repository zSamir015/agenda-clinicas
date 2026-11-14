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

## Datos

La demo usa solo datos sintéticos. El sistema no debe recibir datos reales de pacientes.
