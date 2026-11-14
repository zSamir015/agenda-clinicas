# Accesibilidad (WCAG 2.2 AA)

Lista de comprobación y guía de verificación para el panel interno (`app.<dominio>`) y la página pública de reserva (`<slug>.<dominio>`). Sirve como criterio de aceptación en la fase 8.

## Principio operativo

Ninguna pantalla se da por terminada con solo pasar un analizador automático. axe-core detecta alrededor de un tercio de las fallas reales (contraste, atributos ARIA rotos, etiquetas faltantes), pero no verifica orden de foco, anuncios en lector de pantalla ni equivalentes por teclado para arrastrar y soltar. Cada flujo crítico requiere verificación automática en CI y prueba manual con teclado y VoiceOver.

---

## Lista de comprobación por superficie

### 1. Color, contraste y temas (`docs/ui/tokens.md`)

| Criterio WCAG | Requisito en el producto |
|---|---|
| 1.4.3 Contraste (mínimo) | Todo texto normal cumple >= 4.5:1 y el texto grande (>= 24 px o >= 18.66 px negrita) >= 3.0:1 tanto en tema claro como en tema oscuro |
| 1.4.11 Contraste no textual | Bordes de inputs, interruptores, indicadores de selección y anillos `:focus-visible` cumplen >= 3.0:1 contra su fondo |
| 1.4.1 Uso del color | Los estados de cita (`confirmed`, `held`, `completed`, `cancelled`, `no_show`), errores de formulario y estado SSE combinan color, icono, estilo de borde y texto |
| 1.4.4 Cambio de tamaño de texto | Ampliar el texto al 200 % en el navegador no recorta contenido ni superpone botones en formularios o tarjetas |
| 1.4.10 Reflujo | A 320 px de ancho (o 1280 px al 400 % de zoom), la página pública de reserva y el login funcionan sin desplazamiento horizontal obligatorio |
| Modo Alto Contraste | En `@media (forced-colors: active)`, tarjetas, citas, badges y botones conservan bordes visibles (`1px solid CanvasText`) |

### 2. Calendario semanal con arrastrar y soltar

| Criterio WCAG | Requisito en el producto |
|---|---|
| 2.5.7 Movimientos de arrastre (WCAG 2.2) | Toda acción de mover o redimensionar una cita mediante arrastrar y soltar tiene alternativa de puntero simple y teclado (controles de hora/duración/médico en el panel lateral de la cita) |
| 2.1.1 Teclado | Todas las citas y franjas horarias son alcanzables con `Tab` y teclas de flecha; `Enter` o `Space` abre el detalle; `Escape` cierra el panel o cancela una edición |
| 2.4.7 Foco visible | Cada bloque de cita y botón de barra de herramientas muestra un contorno `:focus-visible` de 3 px con desplazamiento de 2 px |
| 2.4.11 Foco no oculto (WCAG 2.2) | Al tabular por la grilla, los encabezados fijos (`sticky`) y la línea de hora actual no tapan el elemento enfocado |
| 4.1.3 Mensajes de estado | Las actualizaciones recibidas por SSE y los cambios al mover una cita se anuncian con `aria-live="polite"` sin robar el foco al usuario |

### 3. Formularios de reserva pública

| Criterio WCAG | Requisito en el producto |
|---|---|
| 1.3.1 Información y relaciones | Cada `<input>`, `<select>` y `<textarea>` tiene un `<label for>` visible; grupos de 2 a 5 opciones (servicios, médicos) usan `<fieldset>`, `<legend>` y `<input type="radio">` visibles |
| 3.3.1 / 3.3.2 / 3.3.3 Errores y etiquetas | Los campos obligatorios se marcan en la etiqueta y con `required`; los errores se enlazan con `aria-describedby`, activan `aria-invalid="true"` y explican cómo corregir el dato (por ejemplo, formato de teléfono `+507`) |
| 2.4.3 Orden de foco | Al fallar el envío (`submit`), el foco pasa al resumen de errores o al primer campo inválido; no se valida ni se anuncia por `aria-live` en cada pulsación de tecla |
| 2.5.8 Tamaño del objetivo (WCAG 2.2) | Mínimo absoluto de 24 x 24 CSS px; en botones de horario, radios y campos de reserva se aplica `--space-12` (48 px de altura mínima) |
| 3.3.7 Entrada redundante (WCAG 2.2) | Al volver atrás entre pasos de reserva se conservan servicio, médico y horario elegidos sin pedirlos de nuevo |

### 4. Autenticación: Passkeys, TOTP y códigos OTP

| Criterio WCAG | Requisito en el producto |
|---|---|
| 3.3.8 Autenticación accesible (WCAG 2.2) | Ningún flujo bloquea el pegado (`paste`) ni el autocompletado de gestores de contraseñas o passkeys; no se usan CAPTCHAs basados en pruebas cognitivas |
| Autocompletado WebAuthn | El campo de correo en login dedicado incluye `autocomplete="username webauthn"`, `autofocus` y `aria-describedby` hacia las instrucciones de acceso |
| Campo único para OTP y TOTP | El código de 6 dígitos usa un único `<input type="text" inputmode="numeric" autocomplete="one-time-code" pattern="\d{6}">` (no seis cajas separadas) |
| Visibilidad de contraseña | El botón de mostrar/ocultar contraseña es `<button type="button">` con `aria-pressed` y etiqueta clara |

### 5. Diálogos, menús y tiempo

| Criterio WCAG | Requisito en el producto |
|---|---|
| 2.1.2 Sin trampa de teclado | Los diálogos y menús (sobre Radix UI) atrapan el foco mientras están abiertos, cierran con `Escape` y devuelven el foco al botón que los abrió |
| 4.1.2 Nombre, función, valor | Todo diálogo expone `aria-labelledby` y `aria-describedby`; todo botón solo con icono tiene `aria-label` descriptivo |
| 2.2.1 Tiempo ajustable | El temporizador de hold (10 o 15 minutos) muestra el tiempo restante en texto accesible y avisa antes de vencer sin recargar la página por sorpresa |
| 2.3.1 / Movimiento | Ningún elemento parpadea más de 3 veces por segundo; el movimiento espacial respeta `@media (prefers-reduced-motion: reduce)` |

---

## Pruebas manuales

### Solo teclado

Ejecutar sin usar ratón ni trackpad en cada pantalla:

1. **Login (`app.<dominio>/login`):** verificar que `Tab` recorre correo, botón de passkey, contraseña, botón mostrar/ocultar, TOTP y botón de envío en orden lógico; confirmar que `Enter` envía y `Space` activa botones.
2. **Agenda semanal:** navegar entre la barra de herramientas, los filtros, los bloques de cita y el panel lateral; seleccionar una cita con `Enter`, cambiar su horario desde el panel lateral (sin arrastrar) y verificar que el foco no queda atrapado ni oculto.
3. **Reserva pública:** completar los 3 pasos únicamente con `Tab`, flechas (para radios), `Space` y `Enter`, incluyendo el pegado de un código de 6 dígitos en el campo OTP.

### VoiceOver en macOS (Safari o Chrome)

1. Activar VoiceOver con `Cmd + F5`.
2. Abrir el rotor con `Ctrl + Option + U` y revisar:
   - **Encabezados:** jerarquía `h1`, `h2`, `h3` sin saltos ni títulos vacíos.
   - **Puntos de referencia (`landmarks`):** `header`, `nav`, `main`, `aside`, `footer` con etiquetas diferenciadas cuando hay más de un `nav`.
   - **Controles de formulario:** cada campo anuncia su etiqueta, si es obligatorio, su texto de ayuda y su mensaje de error al activarse `aria-invalid="true"`.
3. Verificar que al actualizarse el estado de una cita o el resultado del login se escucha el anuncio de `aria-live="polite"` sin interrumpir lo que el usuario está escribiendo.

---

## Automatización con axe-core en Playwright

En la fase 8 se integra `@axe-core/playwright` en la suite E2E para correr análisis automáticos sobre las rutas críticas en tema claro y tema oscuro.

### Instalación prevista (en `web/`)

```bash
npm install -D @axe-core/playwright
```

### Patrón de prueba reutilizable

```ts
import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const wcagTags = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

for (const colorScheme of ["light", "dark"] as const) {
  test(`página pública de reserva sin violaciones WCAG 2.2 AA (${colorScheme})`, async ({ page }) => {
    await page.emulateMedia({ colorScheme });
    await page.goto("/reserva");

    const results = await new AxeBuilder({ page })
      .withTags(wcagTags)
      .analyze();

    expect(results.violations).toEqual([]);
  });
}
```

### Rutas mínimas cubiertas en CI (fase 8)

- `/login` (estado inicial y estado con mensaje de error de validación).
- `/agenda` (grilla semanal con citas de ejemplo y panel de detalle abierto).
- `/reserva` (los tres pasos del flujo público de reserva, en tema claro y oscuro).
- Diálogo modal de confirmación abierto (cancelación de cita o acción destructiva).
