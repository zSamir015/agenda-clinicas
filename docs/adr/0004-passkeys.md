# 0004. Passkeys como acceso principal del personal

Estado: aceptada (2026-11-13)

## Contexto

El personal accede a datos de salud. Las contraseñas se reutilizan y se roban por phishing. El dueño quiere aprender WebAuthn.

## Decisión

- Personal: passkeys con `go-webauthn/webauthn` como método principal. Contraseña con Argon2id más TOTP obligatorio y códigos de recuperación como respaldo.
- Sesiones en servidor con cookie `HttpOnly`, no JWT.
- Pacientes: sin cuenta, con código de un solo uso por WhatsApp o correo.

## Consecuencias

- La fase 2 crece entre 8 y 10 horas.
- Las pruebas end-to-end usan el autenticador virtual del navegador.
- Revocar el acceso de una persona es inmediato.
