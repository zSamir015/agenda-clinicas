# deploy

Infraestructura del proyecto. Por ahora solo el entorno de desarrollo local; la de producción se agrega en la fase 10 (ver `docs/design.md`, sección 12).

## Desarrollo local

`compose.yaml` levanta dos servicios:

| Servicio | Puerto | Para qué |
|---|---|---|
| MongoDB 8.0 | `27017` | Base de datos, como replica set `rs0` de un nodo |
| Mailpit | `1025` (SMTP), `8025` (web) | Captura los correos que manda el sistema y los muestra en http://localhost:8025 |

MongoDB corre como replica set aunque tenga un solo nodo porque las transacciones y los change streams no funcionan en un servidor suelto. La primera vez que arranca, el healthcheck ejecuta `rs.initiate`; después solo comprueba que el replica set responde. `make db-up` espera a que ambos servicios estén sanos antes de terminar.

Los datos de MongoDB quedan en el volumen `mongo-data` y sobreviven a `make db-down`. `make db-reset` borra el volumen.

Para abrir una consola de MongoDB:

```bash
docker compose -f deploy/compose.yaml exec mongo mongosh
```
