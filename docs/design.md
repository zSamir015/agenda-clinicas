# Diseño

Estado: borrador para revisión del dueño (2026-11-13).

## 1. Objetivo

Plataforma SaaS de agenda y administración para clínicas y consultorios en Panamá. Cada clínica tiene su propio subdominio, su panel interno y una página pública donde los pacientes reservan, pagan un depósito y reciben recordatorios.

El proyecto es de portafolio, pero se construye con los requisitos de un sistema real: aislamiento entre clínicas, cifrado de datos personales, auditoría, pagos idempotentes y pruebas de concurrencia. La demo pública usa solo datos sintéticos.

### Alcance de la v1

- Clínicas, médicos, consultorios, servicios y horarios.
- Cálculo de disponibilidad y reserva sin citas dobles.
- Reserva en línea por el paciente con depósito opcional.
- Ficha básica del paciente: contacto, aseguradora, consentimientos.
- Recordatorios por correo y WhatsApp.
- Panel de recepción con la agenda actualizada en vivo.
- Dos niveles de aislamiento (estándar y dedicado) y cambio de nivel.

### Fuera de la v1

Expediente clínico (notas, diagnósticos, recetas, adjuntos), facturación electrónica, integración con aseguradoras, apps móviles nativas, varios idiomas.

## 2. Arquitectura general

```
            app.<dominio>              <clinica>.<dominio>
                 |                            |
                 +------------+---------------+
                              |
                        web (Next.js)
                              |
                      HTTP + SSE (OpenAPI)
                              |
                         api (Go) -------- Stripe, WhatsApp, correo
                              |                  ^
                           MongoDB               |
                              |             worker (Go)
                              +------ outbox ----+
```

- `api`: servicio HTTP en Go. Monolito modular con los módulos `clinics`, `schedule`, `patients`, `appointments`, `notifications`, `payments`, `auth` y `audit`. Cada módulo expone una interfaz de servicio; ninguno lee colecciones de otro.
- `worker`: proceso en Go que consume el outbox, envía notificaciones, procesa webhooks, libera holds vencidos y ejecuta migraciones por clínica.
- `web`: Next.js con App Router y TypeScript. Sirve el panel interno en `app.<dominio>` y la página pública de cada clínica en `<slug>.<dominio>`.
- `openapi/`: contrato de la API. El servidor Go se genera con `oapi-codegen` (modo estricto) y el cliente TypeScript con `openapi-typescript` y `openapi-fetch`. El contrato se escribe primero.

Decisiones de librerías en Go: `net/http` de la librería estándar para el ruteo, `log/slog` para logs, `go.mongodb.org/mongo-driver/v2`, configuración por variables de entorno.

## 3. Datos y aislamiento

### Niveles

| Nivel | Datos operativos | Datos de pacientes |
|---|---|---|
| Estándar | Base compartida `operations`, con `clinicId` en cada documento | Base propia `patients_<clinicId>` |
| Dedicado | Base propia `clinic_<clinicId>` | La misma base `clinic_<clinicId>` |

La base `platform` guarda el registro de clínicas (`clinics`): slug, nombre, nivel, región, id de la llave de datos, estado. También guarda usuarios del personal, sesiones y el estado de las migraciones.

### Resolución de la clínica

`TenantResolver` toma el slug (del subdominio o del token de sesión), consulta `platform.clinics` con una caché en memoria de 60 segundos y devuelve un `TenantContext` con los handles de base ya resueltos. Los repositorios solo se construyen a partir de un `TenantContext`. En el nivel estándar el repositorio agrega `clinicId` a cada filtro y a cada documento insertado.

Una prueba recorre todos los repositorios contra dos clínicas y falla si algún método devuelve o modifica datos de la otra.

### Migraciones

Cada cambio de esquema es una migración numerada en Go (índices, validación `$jsonSchema`, transformaciones). El runner aplica las migraciones a `platform`, a `operations` y a cada base por clínica, y guarda en `platform.migrations` la versión de cada base. Es idempotente y se reanuda donde quedó.

### Cambio de nivel

Comando `tenantctl move <slug> --to dedicated`: pone la clínica en modo de solo lectura, copia sus documentos a `clinic_<clinicId>`, compara conteos y sumas de verificación, cambia el nivel en `platform.clinics` y borra los datos viejos. La prueba de este comando corre con datos sintéticos de varias clínicas.

## 4. Cifrado y datos personales

- Cifrado explícito a nivel de campo (CSFLE) con `libmongocrypt` desde el driver de Go.
- Cada clínica tiene su propia llave de datos (DEK) en la colección `platform.keyvault`. Las DEK se cifran con una llave maestra: archivo local en desarrollo y AWS KMS en producción.
- La cédula usa cifrado determinista para poder buscar por igualdad. El teléfono, el correo, la fecha de nacimiento y el nombre completo usan cifrado aleatorio. La búsqueda por nombre usa un campo aparte con prefijos normalizados en HMAC.
- Borrar una clínica destruye su DEK (crypto-shredding). Así se cumple el derecho de supresión de la Ley 81 de 2019, incluso en respaldos antiguos.
- Ningún dato personal aparece en logs, errores, trazas ni métricas. Los tipos de dominio que contienen datos personales implementan `slog.LogValuer` y se registran redactados.

## 5. Agenda y reservas

### Colecciones

| Colección | Contenido |
|---|---|
| `practitioners` | Médico, especialidad, servicios que atiende |
| `rooms` | Consultorio o equipo, con tipo |
| `services` | Duración, precio, depósito, margen entre citas, tipo de consultorio requerido |
| `availability_rules` | Horario semanal por médico y consultorio, con vigencia |
| `blocks` | Vacaciones, feriados y bloqueos puntuales |
| `appointments` | Paciente, médico, consultorio, servicio, inicio, fin, estado, historial de cambios |
| `slot_locks` | Una celda ocupada de un recurso |

### Prevención de citas dobles

El tiempo se divide en celdas de 5 minutos. Toda cita empieza y dura en múltiplos de 5. Reservar inserta, en una transacción, un documento en `slot_locks` por cada celda y por cada recurso (médico y consultorio). `slot_locks` tiene un índice único sobre `(clinicId, resourceId, cellStart)`. Si dos reservas compiten, una transacción falla con clave duplicada y la API responde `409 slot_unavailable`.

### Estados de una cita

`held`, `confirmed`, `completed`, `cancelled`, `no_show`.

- `held` dura 10 minutos, o 15 si hay pago en curso. Los locks de un hold llevan `expiresAt`.
- El worker libera los holds vencidos cada minuto. Un índice TTL sobre `expiresAt` es el respaldo.
- Si una reserva choca con un lock vencido, la misma transacción lo reemplaza.
- Reprogramar suelta los locks viejos y toma los nuevos en una sola transacción.

### Disponibilidad

Reglas semanales, menos bloqueos, menos celdas ocupadas, filtrado por duración del servicio y tipo de consultorio. Las fechas se guardan en UTC y se muestran en `America/Panama`.

## 6. Autenticación, roles y auditoría

### Personal de la clínica

- Correo y contraseña con Argon2id, más segundo factor TOTP obligatorio.
- Sesiones en el servidor (colección `platform.sessions`) con cookie `HttpOnly`, `Secure` y `SameSite=Lax`, más token CSRF en las peticiones que modifican datos. No se usan JWT para el panel.
- Alta por invitación con enlace de un solo uso.
- Una persona puede pertenecer a varias clínicas con un rol distinto en cada una.

### Roles

| Permiso | owner | admin | reception | practitioner |
|---|---|---|---|---|
| Configurar clínica, usuarios y pagos | sí | sí | no | no |
| Gestionar médicos, servicios y horarios | sí | sí | no | solo los suyos |
| Ver y modificar la agenda | sí | sí | sí | solo la suya |
| Ver ficha del paciente | sí | sí | sí | solo sus pacientes |
| Exportar o borrar datos de pacientes | sí | no | no | no |

Los permisos se definen en una tabla en código y se comprueban en la capa de servicio, no en los handlers.

### Pacientes

Sin contraseña. El paciente recibe un código de 6 dígitos por correo o WhatsApp para reservar y para ver, cancelar o reprogramar sus citas. Los códigos vencen en 10 minutos y tienen límite de intentos.

### Administrador de la plataforma

Cuenta separada, solo por CLI en la v1 (`tenantctl`). Crea clínicas, cambia niveles y ejecuta borrados.

### Auditoría

Cada lectura y cambio de datos de un paciente genera un evento en `audit_events` de la base de la clínica: quién, qué, cuándo, desde qué IP. Los eventos se encadenan con un hash del evento anterior, y un comando verifica que la cadena no fue alterada.

### Límites de uso

Límite de peticiones por IP y por usuario en memoria de cada instancia para la v1, y más estricto en el login y en el envío de códigos.

## 7. Pagos

- Stripe Connect con cuentas Express, en modo de prueba. Cada clínica conecta su cuenta desde el panel y la plataforma cobra el depósito en nombre de la clínica.
- Si el servicio tiene depósito, la reserva crea un hold de 15 minutos y una sesión de Stripe Checkout. El webhook `checkout.session.completed` confirma la cita.
- Los webhooks se verifican por firma y se registran en `processed_events` con índice único sobre el id del evento, para procesar cada uno una sola vez.
- Si el pago llega después de que venció el hold y el horario ya se ocupó, se reembolsa de forma automática y se avisa al paciente.
- Cancelar con más de 24 horas de anticipación reembolsa el depósito. La regla es configurable por clínica.

## 8. Notificaciones

- Patrón outbox: el cambio de estado de una cita y el mensaje a enviar se escriben en la misma transacción. El worker lee el outbox con un change stream y un respaldo por sondeo.
- Reintentos con espera exponencial y un máximo de intentos. Los mensajes fallidos quedan marcados para revisión.
- Correo: SMTP con Mailpit en desarrollo y un proveedor transaccional en producción.
- WhatsApp: Cloud API de Meta con número de prueba y plantillas aprobadas. Los botones de confirmar o cancelar llegan por webhook y cambian el estado de la cita.
- Mensajes: confirmación de reserva, recordatorio 24 horas antes, recordatorio 2 horas antes, cancelación y reprogramación.

## 9. Web

- Next.js con App Router, TypeScript estricto, Tailwind CSS y componentes propios sobre Radix.
- El middleware resuelve el subdominio: `app.` lleva al panel y cualquier otro slug a la página pública de esa clínica.
- Los datos se piden con TanStack Query sobre el cliente generado del contrato.
- La agenda de recepción se actualiza en vivo por Server-Sent Events. La API los alimenta con change streams.
- Vistas del panel: agenda del día y de la semana por médico o consultorio, ficha del paciente, configuración de servicios y horarios, usuarios, pagos.
- Página pública: elegir servicio, médico y horario, identificarse con código, pagar y recibir confirmación. Las páginas públicas cumplen WCAG 2.2 AA.

## 10. Errores

- La API responde errores con `application/problem+json` (RFC 9457) y un código estable (`slot_unavailable`, `hold_expired`, `forbidden`, `validation_failed`).
- Los errores internos se envuelven con `%w` y se registran con un id de petición. Al cliente solo le llega el id.
- Las operaciones que crean recursos aceptan la cabecera `Idempotency-Key`.

## 11. Observabilidad

- Logs estructurados con `slog` en JSON, sin datos personales.
- Trazas y métricas con OpenTelemetry, exportadas a Grafana Cloud en el plan gratuito.
- Métricas propias: reservas por minuto, conflictos de reserva, holds vencidos, latencia de envío de notificaciones, webhooks fallidos.
- Endpoints `/healthz` y `/readyz`.

## 12. Despliegue

- `api` y `worker` como contenedores en Fly.io.
- `web` en Vercel, con dominio comodín para los subdominios de las clínicas. Requiere un dominio propio.
- MongoDB Atlas en el plan gratuito para la demo (replica set, permite transacciones).
- AWS KMS para la llave maestra.
- Entornos: local, `preview` por PR para la web y `production` para la demo.
- Respaldos: los de Atlas, más un `mongodump` semanal cifrado como prueba de restauración.

En local todo corre con `docker compose`: MongoDB como replica set de un nodo y Mailpit. La API, el worker y la web corren en el host.

## 13. Pruebas

- Go: pruebas unitarias por módulo y de integración contra MongoDB real usando testcontainers.
- Pruebas obligatorias: aislamiento entre clínicas, 100 reservas simultáneas al mismo horario con un solo ganador, reprogramación concurrente, hold vencido durante el pago, idempotencia de webhooks, verificación de la cadena de auditoría, cambio de nivel.
- Web: pruebas de componentes con Vitest y Testing Library, y flujos completos con Playwright (reserva con pago, recepción mueve una cita).
- Carga: k6 contra el endpoint de reserva, con el número de conflictos y la latencia p95 documentados.
- CI en GitHub Actions: lint, pruebas de Go con MongoDB, pruebas de web, verificación de que el código generado del contrato está al día.

## 14. Datos de la demo

`seed` genera clínicas, médicos, horarios, pacientes y citas sintéticos con nombres y cédulas inventados en formato panameño. Ningún dato real entra al sistema.

## 15. Riesgos

| Riesgo | Mitigación |
|---|---|
| CSFLE con el driver de Go requiere `libmongocrypt` y cgo | Resolverlo en la fase 1 con una prueba aislada antes de construir encima |
| Plantillas de WhatsApp requieren aprobación de Meta | Usar el número de prueba; si se demora, el canal de correo cubre la demo |
| Dominio comodín en Vercel | Comprar el dominio al inicio de la fase 10 |
| Alcance grande para una persona | Cada fase termina en algo que funciona y se puede mostrar |
