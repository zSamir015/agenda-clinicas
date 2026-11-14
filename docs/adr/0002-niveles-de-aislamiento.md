# 0002. Dos niveles de aislamiento por clínica

Estado: aceptada (2026-11-13)

## Contexto

Los datos de pacientes son sensibles. Algunas clínicas pueden exigir que sus datos vivan en una base propia o en otra región. Una base por clínica para todos multiplica migraciones, índices y conexiones.

## Decisión

- Nivel estándar: datos operativos en la base compartida `operations` con `clinicId`, datos de pacientes en `patients_<clinicId>`.
- Nivel dedicado: todo en `clinic_<clinicId>`.
- El registro `platform.clinics` indica el nivel. `TenantResolver` entrega repositorios ya ligados a la base correcta.
- Cada clínica cifra los campos sensibles con su propia llave de datos.

## Consecuencias

- El código de dominio no conoce el nivel de la clínica.
- El runner de migraciones recorre todas las bases y guarda la versión de cada una.
- Mover una clínica de nivel es una operación explícita con verificación.
