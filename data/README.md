# Datos de referencia

Este directorio contiene catálogos estáticos usados por el sistema al crear clínicas o generar datos de demostración.

## Feriados nacionales de Panamá (`panama-holidays.json`)

Lista de días feriados y de duelo nacional de la República de Panamá para los años **2026 a 2030**, en zona horaria `America/Panama` (UTC-5). La fase 3 los precarga como bloqueos iniciales al dar de alta una clínica.

### Estructura de cada registro

| Campo | Tipo | Descripción |
|---|---|---|
| `date` | `string` (ISO `YYYY-MM-DD`) | Fecha efectiva del descanso o cierre |
| `originalDate` | `string` (opcional) | Fecha calendario original cuando el Artículo 47 traslada el descanso al lunes siguiente |
| `name` | `string` | Nombre oficial en español |
| `movable` | `boolean` | `true` en fechas litúrgicas movibles (Martes de Carnaval, Viernes Santo, Lunes de Carnaval) |
| `shiftedFromSunday` | `boolean` | `true` cuando la fecha original cae en domingo y el descanso obligatorio pasa al lunes |
| `confirmed` | `boolean` | `true` para feriados establecidos por ley sin necesidad de decreto; `false` para fechas sujetas a decreto ejecutivo |
| `requiresDecree` | `boolean` | `true` si la suspensión de labores depende de Decreto Ejecutivo anual |
| `legalSource` | `string` | Norma legal o criterio oficial que respalda la fecha |

### Uso en la precarga de clínicas (fase 3)

- Al crear una clínica, el sembrador precarga por defecto los registros con `confirmed: true` (descanso obligatorio nacional por ley).
- Los registros con `requiresDecree: true` y `confirmed: false` (Lunes de Carnaval, Día de los Difuntos y Día de los Símbolos Patrios) se conservan en el catálogo para que cada clínica decida si los bloquea según su operación o según el decreto emitido cada año.

### Fuentes legales y cálculo

1. **Código de Trabajo de Panamá, Artículo 46**: fija los días de descanso obligatorio por fiesta o duelo nacional: 1 de enero, 9 de enero, Martes de Carnaval, Viernes Santo, 1 de mayo, día de toma de posesión del Presidente titular de la República (1 de julio cada cinco años; aplica en 2029), 3 de noviembre, 5 de noviembre, 10 de noviembre, 28 de noviembre, 8 de diciembre y 25 de diciembre.
2. **Código de Trabajo de Panamá, Artículo 47**: cuando un día de fiesta o duelo nacional coincide con domingo, el lunes siguiente se habilita como descanso obligatorio. En estos casos `date` registra el lunes observado y `originalDate` conserva el domingo original.
3. **Ley 291 de 31 de marzo de 2022**: reforma el Artículo 46 del Código de Trabajo e incorpora el **20 de diciembre** como Día de Duelo Nacional de descanso obligatorio.
4. **Fechas sujetas a decreto (`requiresDecree: true`)**: Lunes de Carnaval, 2 de noviembre (Día de los Difuntos) y 4 de noviembre (Día de los Símbolos Patrios, Ley 2 de 2012) no figuran como descanso obligatorio nacional en el Artículo 46; su cierre total o parcial depende cada año de Decreto Ejecutivo para el sector público y de la decisión de cada clínica en el sector privado.
5. **Fechas movibles**: Martes de Carnaval corresponde a 47 días antes del Domingo de Pascua y Viernes Santo a 2 días antes del Domingo de Pascua según el cómputo gregoriano.
