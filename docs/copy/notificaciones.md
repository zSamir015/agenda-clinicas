# Textos de notificaciones por correo y WhatsApp

Especificación de mensajes del sistema para la fase 7 (`notifications`). Todos los textos están escritos en español panameño claro, directo y cordial, sin signos de exclamación innecesarios ni lenguaje comercial.

## Reglas generales y privacidad

1. **Sin datos clínicos.** Ningún correo ni mensaje de WhatsApp incluye diagnósticos, motivos de consulta, resultados ni notas médicas. En plantillas de WhatsApp se omite incluso el nombre del servicio cuando pueda revelar una especialidad sensible; en su lugar se usa `{{clinic_name}}` y el nombre del profesional (`{{practitioner_name}}`).
2. **Zona horaria explícita.** Todas las fechas y horas (`{{start_local}}`) se formatean en `America/Panama` (por ejemplo: `miércoles 18 de noviembre de 2026, 9:30 a. m.`).
3. **Requisitos de Meta para WhatsApp Cloud API:**
   - Las plantillas usan parámetros posicionales (`{{1}}`, `{{2}}`, etc.) sin saltos de línea al inicio o al final ni más de dos saltos seguidos.
   - Límite de cuerpo: 1024 caracteres (en este documento se mantienen por debajo de 450 caracteres).
   - Límite de pie de mensaje: 60 caracteres.
   - Límite de texto en botones de respuesta rápida (`QUICK_REPLY`): 20 caracteres.
   - Categoría `AUTHENTICATION` para códigos de un solo uso; categoría `UTILITY` para confirmaciones, recordatorios, cancelaciones, reprogramaciones y pagos.

## Catálogo de variables

| Variable | Ejemplo sintético | Descripción |
|---|---|---|
| `{{patient_name}}` | `Mateo Quintero` | Nombre del paciente |
| `{{clinic_name}}` | `Clínica del Istmo` | Nombre de la clínica |
| `{{clinic_address}}` | `Vía España, Torre Médica Piso 4` | Dirección física de la clínica |
| `{{clinic_phone}}` | `+507 200-0100` | Teléfono de contacto de la clínica |
| `{{practitioner_name}}` | `Dra. Valeria Mendoza` | Nombre del médico tratante |
| `{{service_name}}` | `Consulta general` | Nombre del servicio (solo en correo si aplica) |
| `{{start_local}}` | `mié 18 nov 2026, 9:30 a. m.` | Fecha y hora en `America/Panama` |
| `{{previous_start_local}}` | `mar 17 nov 2026, 11:00 a. m.` | Fecha y hora anterior al reprogramar |
| `{{otp_code}}` | `482910` | Código numérico de 6 dígitos |
| `{{otp_ttl_minutes}}` | `10` | Vigencia del código en minutos |
| `{{amount_formatted}}` | `$15.00` | Monto cobrado o reembolsado en USD/PAB |
| `{{cancellation_hours}}` | `24` | Horas mínimas de anticipación para reembolso |
| `{{manage_url}}` | `https://istmo.calenda.pa/citas` | Enlace para ver, reprogramar o cancelar |

---

## 1. Código de acceso (OTP)

### Correo

- **Asunto:** `Tu código de acceso para {{clinic_name}}: {{otp_code}}`
- **Cuerpo:**

```text
Hola, {{patient_name}}:

Tu código de verificación para {{clinic_name}} es:

{{otp_code}}

Este código vence en {{otp_ttl_minutes}} minutos y es de un solo uso. Si no solicitaste este acceso, puedes ignorar este mensaje.
```

### WhatsApp

- **Nombre de plantilla:** `calenda_patient_otp`
- **Categoría Meta:** `AUTHENTICATION`
- **Mapeo de parámetros:** `{{1}}` = `otp_code`, `{{2}}` = `clinic_name`, `{{3}}` = `otp_ttl_minutes`
- **Cuerpo:**

```text
{{1}} es tu código de verificación para {{2}}. Vence en {{3}} minutos. Por tu seguridad, no lo compartas con nadie.
```

- **Botón:** `COPY_CODE` (botón nativo de copiar código de Meta).

---

## 2. Confirmación de reserva

### Correo

- **Asunto:** `Cita confirmada en {{clinic_name}} ({{start_local}})`
- **Cuerpo:**

```text
Hola, {{patient_name}}:

Tu cita quedó confirmada con los siguientes datos:

- Clínica: {{clinic_name}}
- Profesional: {{practitioner_name}}
- Fecha y hora: {{start_local}} (hora de Panamá)
- Dirección: {{clinic_address}}

Si necesitas cancelar o reprogramar con al menos {{cancellation_hours}} horas de anticipación, puedes hacerlo en:
{{manage_url}}

Teléfono de la clínica: {{clinic_phone}}
```

### WhatsApp

- **Nombre de plantilla:** `calenda_appointment_confirmed`
- **Categoría Meta:** `UTILITY`
- **Mapeo de parámetros:** `{{1}}` = `patient_name`, `{{2}}` = `clinic_name`, `{{3}}` = `practitioner_name`, `{{4}}` = `start_local`, `{{5}}` = `clinic_address`
- **Cuerpo:**

```text
Hola, {{1}}. Tu cita en {{2}} quedó confirmada con {{3}} para el {{4}} (hora de Panamá). Dirección: {{5}}.
```

- **Pie (<= 60 caracteres):** `Gestiona tu cita en línea si necesitas cambiarla.`
- **Botón URL:** `Ver mi cita` (`{{manage_url}}`)

---

## 3. Recordatorio a 24 horas

### Correo

- **Asunto:** `Recordatorio: cita mañana en {{clinic_name}} ({{start_local}})`
- **Cuerpo:**

```text
Hola, {{patient_name}}:

Te recordamos tu cita en {{clinic_name}}:

- Profesional: {{practitioner_name}}
- Fecha y hora: {{start_local}} (hora de Panamá)
- Dirección: {{clinic_address}}

Si no puedes asistir, por favor reprográmala o cancélala en:
{{manage_url}}

Teléfono de la clínica: {{clinic_phone}}
```

### WhatsApp

- **Nombre de plantilla:** `calenda_reminder_24h`
- **Categoría Meta:** `UTILITY`
- **Mapeo de parámetros:** `{{1}}` = `patient_name`, `{{2}}` = `clinic_name`, `{{3}}` = `practitioner_name`, `{{4}}` = `start_local`
- **Cuerpo:**

```text
Hola, {{1}}. Te recordamos tu cita en {{2}} con {{3}} mañana {{4}} (hora de Panamá). Por favor confirma si asistirás.
```

- **Botones (`QUICK_REPLY`, <= 20 caracteres cada uno):**
  1. `Confirmar cita` (payload webhook: `confirm_appointment`)
  2. `Cancelar cita` (payload webhook: `cancel_appointment`)

---

## 4. Recordatorio a 2 horas

### Correo

- **Asunto:** `Tu cita en {{clinic_name}} es en 2 horas ({{start_local}})`
- **Cuerpo:**

```text
Hola, {{patient_name}}:

Tu cita en {{clinic_name}} con {{practitioner_name}} inicia en aproximadamente 2 horas:

- Fecha y hora: {{start_local}} (hora de Panamá)
- Dirección: {{clinic_address}}

Si tienes algún inconveniente para llegar, comunícate al {{clinic_phone}}.
```

### WhatsApp

- **Nombre de plantilla:** `calenda_reminder_2h`
- **Categoría Meta:** `UTILITY`
- **Mapeo de parámetros:** `{{1}}` = `patient_name`, `{{2}}` = `clinic_name`, `{{3}}` = `start_local`, `{{4}}` = `clinic_address`, `{{5}}` = `clinic_phone`
- **Cuerpo:**

```text
Hola, {{1}}. Tu cita en {{2}} es hoy a las {{3}} (hora de Panamá) en {{4}}. Si tienes algún inconveniente, llama al {{5}}.
```

---

## 5. Cancelación

### Correo

- **Asunto:** `Cita cancelada en {{clinic_name}} ({{start_local}})`
- **Cuerpo:**

```text
Hola, {{patient_name}}:

Tu cita programada en {{clinic_name}} con {{practitioner_name}} para el {{start_local}} (hora de Panamá) fue cancelada.

Si deseas agendar una nueva fecha, puedes hacerlo en:
{{manage_url}}

Teléfono de la clínica: {{clinic_phone}}
```

### WhatsApp

- **Nombre de plantilla:** `calenda_appointment_cancelled`
- **Categoría Meta:** `UTILITY`
- **Mapeo de parámetros:** `{{1}}` = `patient_name`, `{{2}}` = `clinic_name`, `{{3}}` = `start_local`, `{{4}}` = `clinic_phone`
- **Cuerpo:**

```text
Hola, {{1}}. Tu cita en {{2}} del {{3}} (hora de Panamá) fue cancelada. Para agendar otra fecha, contáctanos al {{4}}.
```

---

## 6. Reprogramación

### Correo

- **Asunto:** `Cita reprogramada en {{clinic_name}} ({{start_local}})`
- **Cuerpo:**

```text
Hola, {{patient_name}}:

Tu cita en {{clinic_name}} con {{practitioner_name}} fue reprogramada:

- Horario anterior: {{previous_start_local}}
- Nuevo horario: {{start_local}} (hora de Panamá)
- Dirección: {{clinic_address}}

Puedes consultar o gestionar tu cita en:
{{manage_url}}
```

### WhatsApp

- **Nombre de plantilla:** `calenda_appointment_rescheduled`
- **Categoría Meta:** `UTILITY`
- **Mapeo de parámetros:** `{{1}}` = `patient_name`, `{{2}}` = `clinic_name`, `{{3}}` = `previous_start_local`, `{{4}}` = `start_local`, `{{5}}` = `practitioner_name`
- **Cuerpo:**

```text
Hola, {{1}}. Tu cita en {{2}} cambió del {{3}} al nuevo horario {{4}} (hora de Panamá) con {{5}}.
```

---

## 7. Depósito pagado

### Correo

- **Asunto:** `Comprobante de pago en {{clinic_name}} ({{amount_formatted}})`
- **Cuerpo:**

```text
Hola, {{patient_name}}:

Recibimos tu pago de {{amount_formatted}} para tu cita en {{clinic_name}}.

- Profesional: {{practitioner_name}}
- Fecha y hora: {{start_local}} (hora de Panamá)
- Monto recibido: {{amount_formatted}}

Tu cita quedó confirmada. Puedes revisar los detalles en:
{{manage_url}}
```

### WhatsApp

- **Nombre de plantilla:** `calenda_payment_received`
- **Categoría Meta:** `UTILITY`
- **Mapeo de parámetros:** `{{1}}` = `patient_name`, `{{2}}` = `amount_formatted`, `{{3}}` = `clinic_name`, `{{4}}` = `start_local`
- **Cuerpo:**

```text
Hola, {{1}}. Confirmamos tu pago de {{2}} en {{3}}. Tu cita del {{4}} (hora de Panamá) quedó confirmada.
```

---

## 8. Reembolso

Cubre tanto el reembolso por cancelación dentro del plazo permitido como el reembolso automático cuando un pago llega después de vencer el hold y el horario ya se ocupó.

### Correo

- **Asunto:** `Reembolso procesado en {{clinic_name}} ({{amount_formatted}})`
- **Cuerpo:**

```text
Hola, {{patient_name}}:

Procesamos un reembolso de {{amount_formatted}} correspondiente a tu reserva en {{clinic_name}} ({{start_local}}).

Motivo: {{refund_reason}}

El monto se verá reflejado en tu método de pago según los tiempos habituales de tu banco. Si tienes consultas, comunícate al {{clinic_phone}}.
```

Valores previstos para `{{refund_reason}}`:
- `Cancelación realizada con anticipación dentro del plazo de la clínica`
- `El horario seleccionado dejó de estar disponible antes de completarse el pago`

### WhatsApp

- **Nombre de plantilla:** `calenda_refund_processed`
- **Categoría Meta:** `UTILITY`
- **Mapeo de parámetros:** `{{1}}` = `patient_name`, `{{2}}` = `amount_formatted`, `{{3}}` = `clinic_name`, `{{4}}` = `clinic_phone`
- **Cuerpo:**

```text
Hola, {{1}}. Procesamos un reembolso de {{2}} por tu reserva en {{3}}. Se reflejará según los tiempos de tu banco. Consultas al {{4}}.
```
