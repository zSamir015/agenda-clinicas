# Identidad visual

## Nombre provisional

**Calenda**

El nombre alude al calendario y al registro ordenado del tiempo, sin caer en juegos de palabras ni términos en inglés. En la interfaz del panel se muestra como marca discreta en la barra superior (`Calenda · Clínica del Istmo`). En la página pública de reservas predomina el nombre de la clínica y la marca aparece solo en el pie como proveedor de la agenda.

## Propósito y carácter

Calenda es un instrumento de trabajo diario para recepción y médicos, y una puerta de entrada clara para pacientes que reservan desde el teléfono. La estética busca **precisión clínica y serenidad operativa**:

- En el **panel interno** (recepción, médicos, administración): alta densidad de información bien ordenada, alineación tabular de horas y montos, y contraste nítido para jornadas largas frente al monitor.
- En la **página pública**: una columna central respirada, pasos explícitos y lenguaje directo para completar una reserva en menos de dos minutos desde un móvil.

## Tono de voz

- **Directo y sin adornos.** Frases cortas, verbos en infinitivo o presente. Nada de signos de exclamación, humor forzado ni jerga de marketing.
- **Explícito en el estado y el tiempo.** Toda cita muestra su estado con texto e icono, nunca solo con color. Toda hora indica implícita o explícitamente la zona `America/Panama` (UTC-5).
- **Respetuoso con el paciente.** En la página pública se usa trato neutral y claro ("Elige un servicio", "Ingresa el código de 6 dígitos que enviamos a tu correo o WhatsApp"). En el panel interno se usa lenguaje operativo ("Confirmar llegada", "Liberar bloqueo", "Reprogramar cita").

## Referencias

| Referencia | Qué tomamos de ella | Por qué aplica aquí |
|---|---|---|
| Linear y Notion Calendar | Jerarquía tipográfica compacta, uso de `tabular-nums`, navegación por teclado y separación clara entre navegación y lienzo | La agenda semanal concentra muchas citas en poco espacio; el ruido visual cansa a recepción |
| NHS Service Manual | Formularios de un solo tema por bloque, etiquetas visibles, mensajes de error concretos y alto contraste | Los pacientes reservan desde móviles variados y necesitan entender cada paso sin ambigüedad |
| Stripe Dashboard | Estados de pago sobrios, desglose de montos y retroalimentación clara de acciones irreversibles | Cada servicio puede requerir depósito o pago total con política de reembolso |

## Qué no queremos parecer

- **Librerías de componentes con estilo por defecto** (Material UI, Bootstrap, Tailwind UI / Shadcn sin personalizar): bordes negros puros, radios genéricos o sombras difusas idénticas a cualquier plantilla.
- **Software médico heredado**: tablas grises con bordes 3D, azul rey saturado, menús anidados y formularios densos sin jerarquía.
- **Páginas de arranque o marketing**: gradientes neón, tarjetas flotantes con glassmorphism, ilustraciones vectoriales decorativas o animaciones llamativas que retrasan la tarea.

## Principios del sistema

1. **Propiedad total del diseño.** Los tokens viven en variables CSS propias (`docs/ui/tokens.md`) y los componentes interactivos se construyen sobre primitivas sin estilos de Radix UI.
2. **Accesibilidad verificada desde el token.** Cada pareja de primer plano y fondo cumple WCAG 2.2 nivel AA desde la definición de la paleta, tanto en tema claro como en tema oscuro, y conserva bordes visibles en Modo de Alto Contraste (`forced-colors`).
3. **Color como refuerzo, nunca como único canal.** Los estados de cita (`confirmed`, `held`, `completed`, `cancelled`, `no_show`) combinan color semántico, icono, estilo de borde y etiqueta textual.
4. **Respeto por las preferencias del usuario.** Tema claro y oscuro automático mediante `prefers-color-scheme` y `color-scheme`, más reducción de movimiento mediante `prefers-reduced-motion`.
