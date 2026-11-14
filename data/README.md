# Datos de referencia y catálogos sintéticos

Este directorio contiene catálogos estáticos usados por el sistema para precarga de clínicas y generación de datos sintéticos en la demostración pública.

## Catálogos sintéticos para la demo (`data/synthetic/`)

Usados por el generador `seed` (fase 10) para poblar clínicas, médicos, consultorios, servicios y pacientes ficticios en Panamá. **Ningún dato en estos archivos ni en los generados corresponde a personas reales.**

| Archivo | Contenido |
|---|---|
| `names.json` | Nombres de pila (femeninos y masculinos) y apellidos frecuentes en Panamá para combinación aleatoria |
| `specialties.json` | Especialidades médicas ambulatorias |
| `services.json` | Servicios con `durationMinutes` y `bufferMinutes` en múltiplos de 5 min, precio en centavos (`priceCents`), `depositMode` (`none`, `deposit`, `full`) y tipo de consultorio |
| `insurers.json` | Aseguradoras de salud en Panamá (incluye opción particular) |
| `locations.json` | Códigos oficiales de provincia (1 al 13), barrios/corregimientos y nombres de clínicas ficticias |

## Formato de cédula panameña y regla antisuperposición

### Formato oficial

1. **Cédula ordinaria:** `<provincia>-<tomo>-<asiento>`
   - `provincia`: entero de `1` a `13` (sin cero a la izquierda; `8` corresponde a Panamá, `13` a Panamá Oeste).
   - `tomo`: 1 a 4 dígitos (en registros históricos o formularios suele mostrarse con ceros o sin ellos, por ejemplo `8-850-1234`).
   - `asiento`: 1 a 6 dígitos.
2. **Prefijos especiales:**
   - `PE-<tomo>-<asiento>`: panameño nacido en el extranjero.
   - `E-<tomo>-<asiento>`: extranjero residente.
   - `N-<tomo>-<asiento>`: naturalizado.

### Cómo generar cédulas sintéticas que no correspondan a personas reales

Para garantizar que ninguna cédula generada coincida por azar con un documento real del Tribunal Electoral:

1. **Usar tomo `0000` (reservado para pruebas en el proyecto):** por ejemplo `8-0000-00001`, `4-0000-00002`, `13-0000-00003`. El tomo `0000` cumple el patrón sintáctico `<provincia>-<tomo>-<asiento>` y no se asigna en emisiones ordinarias reales.
2. **Normalización:** al almacenar o buscar por igualdad determinista (CSFLE), conservar los guiones y ceros tal como los genera el seed (`8-0000-00001`).

### Teléfonos y correos sintéticos

- **Teléfonos móviles:** rango reservado `+507 6000-0000` a `+507 6000-0999`.
- **Teléfonos fijos:** rango reservado `+507 200-0000` a `+507 200-0999`.
- **Correos electrónicos:** usar exclusivamente dominios reservados RFC 2606 (`@demo.calenda.example` o `@example.com`).
