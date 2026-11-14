# 0003. Prevención de citas dobles con celdas bloqueadas

Estado: aceptada (2026-11-13)

## Contexto

Dos pacientes o recepcionistas pueden intentar el mismo horario al mismo tiempo, en varias instancias de la API. La garantía tiene que darla la base de datos.

## Decisión

El tiempo se divide en celdas de 5 minutos. Una reserva inserta, en una transacción, un documento en `slot_locks` por cada celda y cada recurso (médico y consultorio), con índice único sobre `(clinicId, resourceId, cellStart)`. El conflicto aparece como error de clave duplicada y la transacción se aborta.

## Consecuencias

- Las citas empiezan y duran en múltiplos de 5 minutos.
- Una cita de 30 minutos crea 12 documentos de bloqueo. El costo es bajo.
- Los holds vencidos se liberan con el worker y con un índice TTL de respaldo.
