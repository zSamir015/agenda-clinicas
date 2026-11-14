# 0001. MongoDB como base de datos

Estado: aceptada (2026-11-13)

## Contexto

El dueño ya usó Postgres en la mayoría de sus proyectos y quiere aprender otra base. El dominio necesita transacciones (reservas), validación de esquema, cifrado de campos y cambios en vivo para la agenda.

## Decisión

MongoDB como replica set, con transacciones multi-documento, validación `$jsonSchema` en todas las colecciones, cifrado explícito a nivel de campo (CSFLE) y change streams.

## Consecuencias

- MongoDB no tiene restricciones de exclusión por rangos de tiempo. La prevención de citas dobles se resuelve con `slot_locks` (ADR 0003).
- Las transacciones requieren replica set también en desarrollo y en CI.
- CSFLE desde Go requiere `libmongocrypt` y cgo.
