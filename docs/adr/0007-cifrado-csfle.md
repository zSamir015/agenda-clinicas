# 0007. Cifrado de campos con CSFLE explícito y etiqueta `cse` opcional

Estado: aceptada (2026-11-13)

## Contexto

Los datos personales de pacientes se cifran por campo con una llave por clínica. El driver de Go de MongoDB solo ofrece cifrado en el cliente (CSFLE) compilando con la etiqueta `cse` y enlazando la librería C `libmongocrypt` (cgo). El diseño marcó esto como riesgo de la fase 1.

## Decisión

- CSFLE explícito, con llaves de datos (DEK) por clínica identificadas por alias en `platform.keyvault`.
- Una prueba de humo (`backend/internal/crypto/smoke_test.go`) demostró, contra MongoDB 8.0 real, que funcionan: crear la DEK, cifrado aleatorio y determinista, búsqueda por igualdad sobre el campo determinista, validador `$jsonSchema` que exige `binData` y rechaza texto plano, y fallo al descifrar tras borrar la DEK.
- La etiqueta `cse` es opcional. `go build`, `go test`, `make test-go` y el linter funcionan sin la librería C. Las pruebas de cifrado corren con `make test-crypto` y en un job propio del CI. Solo los binarios que se despliegan se compilan con `-tags cse`.
- Instalación: macOS con `brew install mongodb/brew/libmongocrypt pkgconf cmake`; Linux con el repositorio apt oficial de libmongocrypt. Ese repositorio no publica Ubuntu 26.04, así que el job usa `ubuntu-24.04` y la imagen de producción (fase 10) debe basarse en una versión con paquete.

## Consecuencias

- Los campos cifrados deterministas permiten búsqueda por igualdad (cédula); los aleatorios no se pueden buscar.
- El driver guarda las DEK en caché durante un tiempo (configurable con `SetKeyExpiration`). Borrar una DEK vuelve ilegibles los datos para procesos nuevos, pero un proceso que ya la tiene en memoria puede seguir descifrando hasta que expire.
- Si todas las DEK se envuelven con la misma llave maestra, un respaldo antiguo del key vault junto con esa llave permite descifrar los datos de una clínica borrada. Para una garantía real de supresión habrá que usar una llave de envoltura por clínica o rotar la maestra al borrar (fase 9).
