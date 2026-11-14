# openapi

Contrato de la API en `openapi.yaml`. Se escribe antes que el código: el servidor Go se genera con `oapi-codegen` y los tipos de TypeScript de la web con `openapi-typescript`. Después de cambiar el contrato, `make gen` regenera todo y el CI falla si el código generado no coincide.

Los errores usan `application/problem+json` (RFC 9457) con un campo `code` estable que la web puede usar para decidir qué mostrar.
