# 0005. Pagos detrás de una interfaz de proveedor

Estado: aceptada (2026-11-13)

## Contexto

Stripe tiene la mejor experiencia de desarrollo y modo de prueba, pero no opera para comercios en Panamá. Un despliegue real necesitaría una pasarela local.

## Decisión

El módulo de pagos depende de la interfaz `PaymentProvider`. La v1 implementa Stripe Connect Express en modo de prueba. Cada servicio elige entre sin cobro, depósito (fijo o porcentaje) o pago total.

## Consecuencias

- Agregar Tilopay o PagueloFacil es escribir un adaptador, sin tocar el dominio.
- Hay más casos de reembolso que probar; se cubren con una tabla de casos.
