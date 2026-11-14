# Especificación de tokens del sistema de diseño

Los tokens definidos en este documento alimentan la implementación CSS de la fase 2 (`web/`) y se prueban desde ya en la hoja compartida [tokens.css](prototipos/tokens.css).

## Arquitectura de tokens y temas

El sistema separa **tokens primitivos** (`--raw-*-light`, `--raw-*-dark`) de **tokens semánticos** (`--bg-canvas`, `--text-primary`, `--accent`, etc.). Los componentes solo consumen tokens semánticos.

1. Se declara `<meta name="color-scheme" content="light dark">` en el `<head>` y `color-scheme: light dark` en `:root` para que los controles nativos del navegador (scrollbars, inputs, selectores) adapten su superficie antes de pintar la página.
2. En navegadores con soporte de `light-dark()`, los tokens semánticos resuelven automáticamente según el `color-scheme` computado.
3. Como respaldo para navegadores anteriores, los valores claros se asignan por defecto y se sobreescriben dentro de `@media (prefers-color-scheme: dark)` (y con el atributo `[data-theme="dark"]` cuando el usuario fuerza un tema manualmente).
4. En `@media (forced-colors: active)` (Modo de Alto Contraste de Windows), los tokens semánticos mapean a palabras clave del sistema (`Canvas`, `CanvasText`, `Highlight`, `HighlightText`, `GrayText`) y los contenedores refuerzan sus bordes porque `box-shadow` desaparece.

## Paleta de color y verificación WCAG 2.2 AA

Requisitos mínimos aplicados:

- Texto normal: relación de contraste >= 4.5:1 (WCAG 1.4.3).
- Texto grande (>= 24px normal o >= 18.66px negrita) e iconos: >= 3.0:1.
- Límites de componentes interactivos (bordes de inputs, indicadores activos, anillos de foco): >= 3.0:1 (WCAG 1.4.11).

### Tokens de superficie, texto y acento

| Token | Tema claro | Tema oscuro | Uso |
|---|---|---|---|
| `--bg-canvas` | `#f8f9fa` | `#0f1417` | Fondo base de la aplicación |
| `--bg-surface` | `#ffffff` | `#161d21` | Tarjetas, paneles, grilla de calendario |
| `--bg-subtle` | `#eef1f4` | `#1e272c` | Encabezados secundarios, filas alternas, fondos inactivos |
| `--text-primary` | `#11181c` | `#eef2f5` | Títulos, contenido principal, etiquetas de formulario |
| `--text-secondary` | `#4b5563` | `#b2bec7` | Metadatos, descripciones, subtítulos |
| `--text-muted` | `#5c6672` | `#8e9ca6` | Horas secundarias, texto de ayuda, pies |
| `--border-default` | `#cbd2d9` | `#33414a` | Divisores, bordes de tarjetas y celdas |
| `--border-strong` | `#6c7682` | `#6b7c87` | Bordes de inputs y controles interactivos |
| `--accent` | `#0c6478` | `#46b8ce` | Acción principal, enlaces activos, selección |
| `--accent-hover` | `#094e5e` | `#62c7da` | Estado hover del acento |
| `--accent-text` | `#ffffff` | `#071a1f` | Texto sobre `--accent` |
| `--accent-subtle-bg` | `#e0f2f5` | `#112f36` | Fondo de elemento seleccionado o pill informativo |
| `--accent-subtle-text` | `#084857` | `#8ce0f0` | Texto sobre `--accent-subtle-bg` |
| `--focus-ring` | `#0c6478` | `#62c7da` | Contorno `:focus-visible` (3px, offset 2px) |

### Tokens semánticos de estado

Cada estado se acompaña siempre de icono y etiqueta textual.

| Estado | Fondo claro / oscuro | Texto claro / oscuro | Borde claro / oscuro | Uso en citas y sistema |
|---|---|---|---|---|
| `success` | `#e3f5ec` / `#102a1e` | `#0f5132` / `#7ee2b0` | `#2a7e56` / `#38a169` | Cita `confirmed`, conexión SSE activa, pago aprobado |
| `warning` | `#fef3c7` / `#2e2008` | `#78350f` / `#f6c965` | `#9a5b13` / `#d69e2e` | Cita `held` (reserva temporal), aviso de vencimiento |
| `danger` | `#fde8e8` / `#321414` | `#8b1a1a` / `#fca5a5` | `#c53030` / `#e55353` | Cita `cancelled`, `no_show`, error de validación |
| `info` | `#e6f0fa` / `#12253a` | `#1e4273` / `#93c5fd` | `#2b6cb0` / `#4299e1` | Cita `completed`, notas informativas |

### Verificación de contraste (luminancia relativa sRGB)

| Pareja evaluada | Contraste claro | Contraste oscuro | Umbral WCAG 2.2 AA |
|---|---|---|---|
| `--text-primary` sobre `--bg-canvas` | 17.01:1 | 16.47:1 | >= 4.5:1 |
| `--text-primary` sobre `--bg-surface` | 17.93:1 | 15.14:1 | >= 4.5:1 |
| `--text-secondary` sobre `--bg-surface` | 7.56:1 | 9.00:1 | >= 4.5:1 |
| `--text-muted` sobre `--bg-surface` | 5.84:1 | 6.05:1 | >= 4.5:1 |
| `--text-muted` sobre `--bg-subtle` | 5.15:1 | 5.40:1 | >= 4.5:1 |
| `--accent-text` sobre `--accent` | 6.75:1 | 7.66:1 | >= 4.5:1 |
| `--accent` sobre `--bg-surface` | 6.75:1 | 7.31:1 | >= 3.0:1 (UI) / 4.5:1 |
| `--accent-subtle-text` sobre `--accent-subtle-bg` | 8.77:1 | 9.46:1 | >= 4.5:1 |
| `--border-strong` sobre `--bg-surface` | 4.61:1 | 3.94:1 | >= 3.0:1 (UI) |
| `--danger-border` sobre `--bg-surface` | 5.47:1 | 4.62:1 | >= 3.0:1 (UI) |
| `--success-text` sobre `--success-bg` | 8.26:1 | 9.76:1 | >= 4.5:1 |
| `--warning-text` sobre `--warning-bg` | 8.15:1 | 10.15:1 | >= 4.5:1 |
| `--danger-text` sobre `--danger-bg` | 7.91:1 | 8.88:1 | >= 4.5:1 |
| `--info-text` sobre `--info-bg` | 8.75:1 | 8.62:1 | >= 4.5:1 |

## Tipografía

- `--font-sans`: `"Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif`
- `--font-mono`: `"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace` (para códigos OTP de 6 dígitos, cédulas, identificadores de cita y horas en grilla).
- `font-variant-numeric: tabular-nums` en horas de agenda, precios y contadores.
- Los controles de formulario mantienen un tamaño mínimo de `1rem` (16px) para evitar zoom automático en iOS.

| Token | Valor | Equivalencia (base 16px) | Uso |
|---|---|---|---|
| `--text-xs` | `0.75rem` | 12px | Etiquetas de estado compactas, horas de grilla secundaria |
| `--text-sm` | `0.875rem` | 14px | Metadatos, texto de ayuda, celdas de tabla |
| `--text-base` | `1rem` | 16px | Texto de cuerpo, inputs, botones |
| `--text-lg` | `1.125rem` | 18px | Subtítulos de sección |
| `--text-xl` | `1.25rem` | 20px | Encabezados de panel y tarjeta |
| `--text-2xl` | `1.5rem` | 24px | Título de pantalla principal |
| `--text-3xl` | `1.875rem` | 30px | Título de portada en página pública |

Alturas de línea sin unidad: `--leading-tight: 1.25`, `--leading-normal: 1.5`, `--leading-relaxed: 1.625`.

## Escala de espaciado

Base de `4px` (`0.25rem`). Los controles interactivos (botones, inputs, celdas clicables de horario) respetan una altura mínima de `48px` (`--space-12`) en pantallas táctiles y formularios públicos.

| Token | Valor | Equivalencia | Uso habitual |
|---|---|---|---|
| `--space-1` | `0.25rem` | 4px | Separación mínima entre etiqueta e input, padding de badges |
| `--space-2` | `0.5rem` | 8px | Espacio entre icono y texto, gap entre controles cercanos |
| `--space-3` | `0.75rem` | 12px | Padding interno de inputs y botones compactos |
| `--space-4` | `1rem` | 16px | Padding estándar de tarjetas, separación entre bloques |
| `--space-5` | `1.25rem` | 20px | Margen entre grupos de formulario |
| `--space-6` | `1.5rem` | 24px | Padding de tarjetas amplias, separación de secciones |
| `--space-8` | `2rem` | 32px | Márgenes de página en escritorio |
| `--space-10` | `2.5rem` | 40px | Altura de botones secundarios en barra de herramientas |
| `--space-12` | `3rem` | 48px | Área táctil mínima en formularios y página pública |
| `--space-16` | `4rem` | 64px | Espaciado vertical mayor |

## Radios y sombras

### Radios

| Token | Valor | Uso |
|---|---|---|
| `--radius-sm` | `0.25rem` (4px) | Bloques de cita en la agenda, etiquetas compactas |
| `--radius-md` | `0.5rem` (8px) | Botones, inputs, elementos de menú |
| `--radius-lg` | `0.75rem` (12px) | Tarjetas, diálogos, contenedores principales |
| `--radius-full` | `9999px` | Píldoras de estado, avatares con iniciales |

### Sombras

En tema oscuro la elevación se expresa primero mediante contraste entre `--bg-canvas`, `--bg-surface` y `--border-default`; las sombras son discretas en ambos temas.

| Token | Tema claro | Tema oscuro |
|---|---|---|
| `--shadow-sm` | `0 1px 2px rgba(17, 24, 28, 0.06)` | `0 1px 2px rgba(0, 0, 0, 0.35)` |
| `--shadow-md` | `0 4px 12px rgba(17, 24, 28, 0.08)` | `0 4px 14px rgba(0, 0, 0, 0.45)` |

## Movimiento

- `--duration-fast: 120ms`: cambios de color y borde en hover o foco.
- `--duration-base: 200ms`: transiciones de estado en paneles y botones.
- `--duration-slow: 300ms`: apertura de diálogos o paneles laterales.
- `--ease-standard: cubic-bezier(0.2, 0, 0, 1)`
- `--ease-out: cubic-bezier(0, 0, 0.2, 1)`

Reglas de movimiento reducido:

- Las transiciones de color (`background-color`, `border-color`, `color`) permanecen activas porque no generan desplazamiento espacial.
- Cualquier desplazamiento (`transform`, `translate`), escala o animación continua se activa de forma opt-in dentro de `@media (prefers-reduced-motion: no-preference)`, o ajusta su duración dentro de `@media (prefers-reduced-motion: reduce)`. No se usa `animation-duration: 0.01ms` de forma global.
