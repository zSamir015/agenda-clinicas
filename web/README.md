# web

Aplicación Next.js con App Router, TypeScript estricto y Tailwind CSS. Sirve el panel en `app.<dominio>` y la página pública en `<slug>.<dominio>`.

## Desarrollo

```bash
npm install
npm run dev
```

Por defecto consulta la API en `http://localhost:8080` (configurable con `API_URL`).

## Contrato OpenAPI y tipos

Los tipos de la API se generan en `src/lib/api/schema.d.ts` a partir de `../openapi/openapi.yaml` usando `openapi-typescript`:

```bash
npm run gen:api
```

**No edites `src/lib/api/schema.d.ts` a mano.** Cualquier cambio de contrato se hace primero en `openapi/openapi.yaml` y luego se regenera con `make gen`. El cliente HTTP (`src/lib/api/client.ts`) usa `openapi-fetch` sobre esos tipos.

## Verificación

```bash
npm run lint
npm run typecheck
npm run format:check
npm test
npm run build
```
