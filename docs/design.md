# Diseño

Estado: borrador para revisión del dueño (2026-11-13). Partes 1 a 7 discutidas y decididas con el dueño.

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

- Método principal: passkeys (WebAuthn) con `go-webauthn/webauthn`. Una persona puede registrar varias passkeys (computadora, teléfono).
- Respaldo: contraseña con Argon2id más TOTP obligatorio, y diez códigos de recuperación de un solo uso guardados como hash.
- Sesiones en el servidor (colección `platform.sessions`) con cookie `HttpOnly`, `Secure` y `SameSite=Lax`, más token CSRF en las peticiones que modifican datos. No se usan JWT para el panel, para poder revocar una sesión al instante.
- Alta por invitación con enlace de un solo uso. Durante la activación se registra la primera passkey o se configura la contraseña con TOTP.
- Una persona puede pertenecer a varias clínicas con un rol distinto en cada una.

### Roles y permisos

- El catálogo de permisos es fijo y vive en el código: `appointments.read`, `appointments.write`, `schedule.manage`, `patients.read`, `patients.write`, `patients.export`, `patients.delete`, `payments.read`, `payments.refund`, `users.manage`, `roles.manage`, `clinic.settings`.
- Cada clínica define sus roles como combinaciones de esos permisos. Cada permiso tiene un alcance: `own` (solo lo asignado a la persona, por ejemplo un médico y sus pacientes) o `all`.
- Al crear una clínica se generan cuatro roles de plantilla: `owner`, `admin`, `reception` y `practitioner`. Todos se pueden editar salvo `owner`, que tiene todos los permisos.
- Nadie puede otorgar un permiso o alcance que no tiene. No se puede quitar el rol `owner` a la última persona que lo tiene.
- Los permisos se comprueban en la capa de servicio, no en los handlers. Los roles se guardan en `platform.roles` y se cachean por sesión; cambiar un rol invalida esa caché.
- Cada cambio de rol o de membresía genera un evento de auditoría.

### Pacientes

- Sin cuenta ni contraseña. El paciente recibe un código de 6 dígitos por WhatsApp o correo para reservar y para ver, cancelar o reprogramar sus citas.
- El código vence en 10 minutos, se guarda como hash, admite 5 intentos y como máximo se envían 3 códigos por hora a un mismo teléfono o correo.
- Tras validar el código se abre una sesión de paciente de 30 minutos, limitada a sus propias citas.

### Administrador de la plataforma

Cuenta separada, solo por CLI en la v1 (`tenantctl`). Crea clínicas, cambia niveles y ejecuta borrados.

### Auditoría

Cada lectura y cambio de datos de un paciente genera un evento en `audit_events` de la base de la clínica: quién, qué, cuándo, desde qué IP. Cada evento guarda el hash del anterior. `tenantctl audit verify <slug>` recorre la cadena y señala el primer evento alterado o faltante.

### Límites de uso

Límite de peticiones por IP y por usuario en memoria de cada instancia para la v1, más estricto en el login, en la verificación de códigos y en el envío de códigos.

## 7. Pagos

### Interfaz

El dominio de pagos depende de una interfaz `PaymentProvider` con estas operaciones: conectar la cuenta de la clínica, crear un cobro, verificar y traducir un webhook a un evento interno, y reembolsar. Ningún otro módulo conoce al proveedor.

- Adaptador de la v1: Stripe Connect con cuentas Express, en modo de prueba. Cada clínica conecta su cuenta desde el panel y el cobro se hace en nombre de la clínica.
- Stripe no opera para comercios en Panamá. Un despliegue real usaría un adaptador local (Tilopay o PagueloFacil). El documento `docs/payments-providers.md` describirá qué necesita ese adaptador; no se implementa en la v1.

### Modalidad de cobro por servicio

Cada servicio define una de tres modalidades:

| Modalidad | Qué se cobra al reservar |
|---|---|
| `none` | Nada |
| `deposit` | Monto fijo o porcentaje del precio |
| `full` | El precio completo |

### Flujo

- Si el servicio cobra algo, la reserva crea un hold de 15 minutos y un cobro con el proveedor (Stripe Checkout en la v1). El webhook de pago completado confirma la cita.
- Los webhooks se verifican por firma y se registran en `processed_events` con índice único sobre el id del evento, para procesar cada uno una sola vez.
- Si el pago llega después de que venció el hold y el horario ya se ocupó, se reembolsa de forma automática y se avisa al paciente.

### Cancelaciones

- Cada clínica configura la anticipación mínima para reembolso (24 horas por defecto).
- Cancelación a tiempo: reembolso completo. Cancelación tardía o inasistencia: la clínica retiene lo cobrado.
- Una tabla de casos en las pruebas cubre cada combinación de modalidad, momento de cancelación, inasistencia y pago tardío, con el resultado esperado.

## 8. Notificaciones

- Patrón outbox: el cambio de estado de una cita y el mensaje a enviar se escriben en la misma transacción. El worker lee el outbox con un change stream y un respaldo por sondeo.
- Reintentos con espera exponencial y un máximo de intentos. Los mensajes fallidos quedan marcados para revisión.
- Los canales implementan la interfaz `Messenger`.
- WhatsApp: Cloud API de Meta directa, con el número de prueba (hasta 5 destinatarios verificados) y plantillas aprobadas. Los botones de confirmar o cancelar llegan por webhook y cambian el estado de la cita.
- Correo: SMTP con Mailpit en desarrollo y Resend o Postmark en producción.
- Mensajes: confirmación de reserva, recordatorios, cancelación, reprogramación y códigos de acceso.
- Recordatorios a 24 y 2 horas por defecto. Cada clínica puede cambiar los tiempos o desactivar uno.

## 9. Web

- Next.js con App Router, TypeScript estricto y Tailwind CSS.
- Sistema de diseño propio: tokens (color, tipografía, espaciado, radios, sombras, movimiento) en variables CSS, con tema claro y oscuro, y componentes propios construidos sobre primitivas de Radix sin estilos. No se usa una librería de componentes con estilo. Los tokens y los componentes base (botón, campo, diálogo, menú, aviso) se hacen en la fase 2; el resto, a medida que lo pide cada pantalla.
- Calendario de la agenda propio: vistas de día y semana, columnas por médico o consultorio, arrastrar para mover y redimensionar citas, línea de la hora actual, y navegación por teclado.
- El middleware resuelve el subdominio: `app.` lleva al panel y cualquier otro slug a la página pública de esa clínica.
- Los datos se piden con TanStack Query sobre el cliente generado del contrato.
- La agenda de recepción se actualiza en vivo por Server-Sent Events. La API los alimenta con change streams.
- Vistas del panel: agenda, ficha del paciente, configuración de servicios y horarios, usuarios y roles, pagos.
- Página pública: elegir servicio, médico y horario, identificarse con código, pagar si corresponde y recibir confirmación.
- Todo el panel y la página pública cumplen WCAG 2.2 AA.

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
- La infraestructura se configura a mano y queda documentada en `deploy/README.md`. Terraform queda fuera de la v1.
- Entornos: local, `preview` por PR para la web y `production` para la demo.
- Respaldos: los de Atlas, más un `mongodump` semanal cifrado como prueba de restauración.

En local todo corre con `docker compose`: MongoDB como replica set de un nodo y Mailpit. La API, el worker y la web corren en el host.

## 13. Pruebas

- Go: pruebas unitarias por módulo y de integración contra MongoDB real usando testcontainers.
- Pruebas obligatorias: aislamiento entre clínicas, 100 reservas simultáneas al mismo horario con un solo ganador, reprogramación concurrente, hold vencido durante el pago, idempotencia de webhooks, verificación de la cadena de auditoría, cambio de nivel, escalada de permisos (nadie otorga lo que no tiene), tabla de casos de cobro y cancelación.
- Web: pruebas de componentes con Vitest y Testing Library, y flujos completos con Playwright (login con passkey usando el autenticador virtual, reserva con pago, recepción mueve una cita en el calendario).
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
| Passkeys difíciles de probar | Pruebas con el autenticador virtual de Playwright (CDP WebAuthn) |
| Sistema de diseño propio alarga la fase 8 | Tokens y componentes base desde la fase 2, el resto según cada pantalla |
| Alcance grande para una persona | Cada fase termina en algo que funciona y se puede mostrar |
