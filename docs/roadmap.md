# Hoja de ruta

Cada fase termina en algo que funciona, tiene pruebas y se puede mostrar. Las fases corresponden a milestones de GitHub. Las estimaciones son horas de trabajo efectivo de una persona y ya incluyen pruebas y documentación.

## Resumen

| Fase | Nombre | Horas |
|---|---|---|
| 0 | Fundaciones | 12 a 16 |
| 1 | Plataforma y aislamiento | 25 a 32 |
| 2 | Autenticación, roles y base del diseño | 42 a 52 |
| 3 | Catálogo y disponibilidad | 22 a 28 |
| 4 | Reservas y calendario | 38 a 46 |
| 5 | Pacientes y auditoría | 16 a 22 |
| 6 | Pagos | 24 a 31 |
| 7 | Notificaciones | 20 a 26 |
| 8 | Interfaz del panel y página pública | 40 a 54 |
| 9 | Nivel dedicado y borrado | 14 a 18 |
| 10 | Producción y demo | 24 a 30 |
| | **Total** | **277 a 355** |

## Calendario estimado

| Dedicación | Duración |
|---|---|
| 10 horas por semana | 7 a 8,5 meses |
| 15 horas por semana | 4,5 a 6 meses |
| 20 horas por semana | 3,5 a 4,5 meses |

Con asistentes de código el tiempo suele bajar entre un 25 y un 40 por ciento en el código repetitivo (contrato, CRUD, componentes), pero no en el diseño, la depuración de concurrencia ni la integración con terceros. Una versión mostrable (fases 0 a 4 más la parte mínima de la 8) llega a la mitad del calendario.

## Fase 0. Fundaciones

Resultado: el repositorio compila, las pruebas corren en CI y `docker compose up` levanta la base.

- Estructura de carpetas, `.editorconfig`, `.gitignore`, configuración de VS Code.
- `docker compose` con MongoDB en replica set de un nodo y Mailpit.
- Módulo Go de `api` con `/healthz`, configuración por entorno y `slog`.
- Contrato OpenAPI inicial y generación con `oapi-codegen` y `openapi-typescript`.
- App Next.js con TypeScript estricto, Tailwind, ESLint y Prettier.
- GitHub Actions: lint y pruebas de Go con MongoDB, lint y pruebas de web, verificación del código generado.
- `Makefile` con los comandos comunes, documentados en el README.

## Fase 1. Plataforma y aislamiento

Resultado: se crean clínicas por CLI y cada petición queda ligada a su clínica sin fugas posibles.

- Base `platform` y colección `clinics`.
- `TenantResolver`, `TenantContext` y repositorios ligados a la clínica.
- Runner de migraciones con versión por base.
- Prueba aislada de CSFLE con `libmongocrypt` en macOS y en CI.
- Llave de datos por clínica y cifrado de campos.
- `tenantctl create` y `tenantctl list`.
- Prueba de aislamiento entre clínicas sobre todos los repositorios.
- ADR de niveles de aislamiento y de cifrado.

## Fase 2. Autenticación, roles y base del diseño

Resultado: el personal entra con passkey (o contraseña y TOTP) y solo ve lo que su rol permite. El sistema de diseño tiene sus tokens y componentes base.

- Tokens del sistema de diseño (color, tipografía, espaciado, movimiento, tema claro y oscuro) y componentes base sobre Radix.
- Usuarios, membresías por clínica e invitaciones.
- Passkeys con `go-webauthn/webauthn`; contraseña con Argon2id, TOTP y códigos de recuperación como respaldo.
- Sesiones en servidor y CSRF.
- Catálogo de permisos, roles por clínica con alcance `own` o `all`, roles de plantilla, reglas anti-escalada.
- Códigos de un solo uso para pacientes con límites de intentos y envíos.
- Límites de peticiones en login y códigos.
- Pantallas de login, invitación, registro de passkey, TOTP y gestión de roles.
- Prueba de passkeys con el autenticador virtual de Playwright.

## Fase 3. Catálogo y disponibilidad

Resultado: la clínica configura médicos, consultorios, servicios y horarios, y la API devuelve los horarios libres correctos.

- CRUD de médicos, consultorios, servicios, reglas y bloqueos, con validación.
- Feriados de Panamá precargados.
- Motor de disponibilidad con pruebas de tabla (reglas solapadas, bloqueos, márgenes, consultorio requerido).
- Pantallas de configuración en el panel.

## Fase 4. Reservas y calendario

Resultado: se reservan, cancelan y reprograman citas sin citas dobles, y la recepción ve la agenda en vivo.

- `slot_locks` con índice único y transacciones.
- Estados, holds, vencimiento y reclamo de locks vencidos.
- Reprogramación atómica.
- Prueba de 100 reservas simultáneas y de reprogramación concurrente.
- SSE con change streams.
- Calendario propio: vistas de día y semana, columnas por médico o consultorio, arrastrar para mover y redimensionar, navegación por teclado.

## Fase 5. Pacientes y auditoría

Resultado: fichas de pacientes cifradas, búsqueda por cédula y nombre, y una bitácora verificable.

- Ficha del paciente con aseguradora y consentimientos.
- Búsqueda por cédula (determinista) y por nombre (prefijos en HMAC).
- `audit_events` encadenados y comando de verificación.
- Pantalla de ficha y de historial de citas.

## Fase 6. Pagos

Resultado: el paciente paga el depósito o el total y la cita se confirma sola.

- Interfaz `PaymentProvider` y adaptador de Stripe Connect Express en modo de prueba.
- Onboarding de la cuenta de la clínica.
- Modalidad por servicio: sin cobro, depósito (fijo o porcentaje) o total.
- Checkout con hold de 15 minutos.
- Webhooks firmados e idempotentes.
- Reembolsos por pago tardío y por cancelación a tiempo, con anticipación configurable.
- Tabla de casos de cobro y cancelación en las pruebas.
- `docs/payments-providers.md` con lo que necesitaría un adaptador local.

## Fase 7. Notificaciones

Resultado: confirmaciones y recordatorios por correo y WhatsApp, con confirmación por botón.

- Outbox en la misma transacción que la cita.
- Worker con change stream, sondeo de respaldo y reintentos.
- Interfaz `Messenger`, adaptador de WhatsApp Cloud API y de correo.
- Plantillas de correo y de WhatsApp.
- Recordatorios a 24 y 2 horas por defecto, configurables por clínica.
- Webhook de respuestas de WhatsApp.

## Fase 8. Interfaz del panel y página pública

Resultado: un producto que se ve terminado.

- Resto de componentes del sistema de diseño propio y documentación de tokens y componentes.
- Página pública de reserva por clínica, de principio a fin, con accesibilidad WCAG 2.2 AA.
- Pulido del panel: estados vacíos, carga, errores, atajos de teclado.
- Playwright: reserva con pago y movimiento de una cita en recepción.

Partes de esta fase se hacen junto a las fases 2 a 7. Aquí queda el pulido final.

## Fase 9. Nivel dedicado y borrado

Resultado: una clínica pasa al nivel dedicado sin perder datos y una clínica borrada queda ilegible.

- `tenantctl move` con copia, verificación y cambio de puntero.
- `tenantctl delete` con destrucción de la llave.
- Pruebas de ambos con datos sintéticos de varias clínicas.

## Fase 10. Producción y demo

Resultado: demo pública con datos sintéticos y documentación completa.

- Dominio, Vercel con subdominios comodín, Fly.io, Atlas, AWS KMS, configurados a mano y documentados en `deploy/README.md`.
- OpenTelemetry y tablero en Grafana Cloud.
- Generador de datos sintéticos y demo con cuentas de prueba.
- Prueba de carga con k6 documentada.
- Prueba de restauración de respaldo documentada.
- README final con capturas, arquitectura y decisiones.
- Tag `v1.0.0`.
