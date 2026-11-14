# Proveedores de pago locales para Panamá

Investigación técnica y comercial sobre pasarelas de pago disponibles en Panamá para reemplazar el adaptador de prueba de Stripe Connect en un despliegue real (ver `docs/design.md`, sección 7). No se implementa ningún adaptador local en la v1.

## Resumen comparativo

| Proveedor | Costo público (referencia) | API | Modo de prueba (sandbox) | Cobro en nombre de terceros (estilo Connect) | Webhooks | Reembolsos |
|---|---|---|---|---|---|---|
| **Tilopay** | Tarjetas: 3.75 % + $0.50 (+ ITBMS). Yappy: 1 % (+ ITBMS). Sin mensualidad fija reportada | REST + SDKs (redirección y embebido) | Sí (staging/sandbox con tarjetas de prueba) | Capacidades de split/marketplace reportadas; onboarding programático de subcuentas no confirmado sin contacto comercial | Sí (`webhook_payment`, etc.) | Sí (totales y parciales desde panel y API) |
| **PagueloFácil** | Plan estándar: 3.5 % + $0.50 (+ 7 % ITBMS sobre comisión); +1 % tarjetas internacionales; $0.35 declinada; $1.00 + ITBMS por retiro | REST (`api.pfserver.net`) y botón de pago | Sí (`sandbox.paguelofacil.com` / `api-sand.pfserver.net`) | No es self-service estilo Connect; esquemas de agregador/marketplace requieren plan corporativo a medida | Sí (notificación HTTP al comercio con `status` y `operationType`) | Sí (totales y parciales desde panel; disponibilidad de endpoint público de reembolso no confirmada en documentación abierta) |
| **Yappy Comercial (Banco General)** | 1 % + ITBMS (mínimo $0.02 por transacción), debitado diario al comercio | Botón web (`bt-cdn.yappy.cloud`) + QR dinámico / API | Sí, previa solicitud a soporte de Banco General (`yappy@bgeneral.com`) | No. Cada comercio afilia su propia cuenta jurídica de Banco General y registra su dominio y credenciales | Sí (`ipnUrl` con firma criptográfica en cabeceras) | Anulación/devolución gestionada en el panel comercial; reembolso automático por API pública no confirmado |
| **Pasarelas bancarias (BAC Credomatic / Cybersource)** | No público (negociación directa: afiliación, renta mensual y % + fijo por transacción) | REST/SOAP Cybersource, Unified Checkout / Secure Acceptance, 3DS 2 | Sí (portal Cybersource Developer Center) | No nativo para terceros en adquirencia bancaria tradicional; requiere modelo agregador autorizado por el banco | Sí (eventos `tms.v1.transactions` firmados) | Sí (mediante API de Cybersource) |

## Detalle por proveedor

### 1. Tilopay

- **Modelo:** facilitador de pagos en Centroamérica y Panamá. Soporta tarjetas Visa, Mastercard, American Express y pagos con Yappy.
- **Costos:** sin mensualidad fija en su esquema estándar público; 3.75 % + $0.50 + ITBMS para tarjetas y 1 % + ITBMS para Yappy (pueden aplicar condiciones adicionales de Yappy Comercial según contrato).
- **API y sandbox:** ofrece API REST para cobro por redirección o formulario embebido y un entorno de pruebas independiente para simular transacciones aprobadas y declinadas.
- **Webhooks y reembolsos:** soporta webhooks de confirmación (`webhook_payment`) y reembolsos totales o parciales por panel y API.
- **Cobro en nombre de terceros:** menciona soporte para flujos marketplace y división de pagos (split). **No se pudo confirmar** en la documentación pública si existe un flujo autoservicio equivalente a Stripe Connect Express (enlace de onboarding por clínica con KYC delegado) ni qué requisitos exige la Superintendencia de Bancos o el adquirente en Panamá para operar como agregador.
- **Fuentes:** sitio oficial de Tilopay (`tilopay.com`, tarifas Panamá y documentación técnica de webhooks).

### 2. PagueloFácil

- **Modelo:** pasarela panameña con soporte de tarjetas de crédito/débito, Clave, Nequi y Pago Cash.
- **Costos:** plan integrado estándar de 3.5 % + $0.50 por transacción aprobada (+ 7 % ITBMS sobre la comisión), +1 % adicional en tarjetas emitidas fuera de Panamá, $0.35 por transacción declinada y $1.00 + ITBMS por retiro a cuenta bancaria local.
- **API y sandbox:** endpoints separados para producción (`secure.paguelofacil.com`, `api.pfserver.net`) y sandbox (`sandbox.paguelofacil.com`, `api-sand.pfserver.net`) con llaves independientes.
- **Webhooks y reembolsos:** envía webhooks de notificación donde se valida `status == 1` y `operationType` (`CAPTURE`, `AUTH_CAPTURE`, etc.). Permite reembolsos totales y parciales desde su panel administrativo. **No se pudo confirmar** sin cuenta de desarrollador activa el detalle de verificación criptográfica de firma en webhooks ni si la API REST expone el endpoint de reembolso para todos los planes.
- **Cobro en nombre de terceros:** no ofrece onboarding autoservicio de subcuentas. Para operar como plataforma multi-clínica requiere negociación comercial como agregador o almacenar credenciales propias por cada clínica.
- **Fuentes:** documentación y tarifario público de PagueloFácil (`paguelofacil.com`).

### 3. Yappy Comercial (Banco General)

- **Modelo:** billetera móvil de Banco General de uso masivo en Panamá.
- **Costos:** 1 % + ITBMS sobre el monto cobrado (mínimo $0.02 por transacción), sin recargo al paciente.
- **API y sandbox:** botón web cargado desde `bt-cdn.yappy.cloud` con validación por dominio HTTPS registrado, `Merchant ID` y `CLAVE_SECRETA`. El acceso a pruebas requiere coordinación con soporte de Banco General (`yappy@bgeneral.com`).
- **Webhooks (`ipnUrl`) y reembolsos:** notifica por HTTP POST el resultado del pago (aprobado, cancelado o expirado) con firma verificable en cabeceras. **No se pudo confirmar** la existencia de un endpoint público documentado para ejecutar reembolsos automáticos por API (crucial cuando un pago llega después de vencer un hold); en la práctica comercial la devolución suele gestionarse como anulación o desde el panel del comercio.
- **Cobro en nombre de terceros:** no existe un esquema tipo Connect donde la plataforma cree cuentas para terceros. Cada clínica debe tener su propia afiliación a Yappy Comercial y entregar su `Merchant ID` y `CLAVE_SECRETA` cifradas a la plataforma.
- **Fuentes:** documentación oficial de Yappy Comercial (`yappy.com.pa`, `yappycomercial.bgeneral.com`, `bgeneral.com`).

### 4. Pasarelas bancarias (BAC Credomatic / Cybersource y adquirencia local)

- **Modelo:** adquirencia bancaria directa respaldada por Cybersource (Visa).
- **Costos:** **no públicos**; requieren solicitud comercial con el banco y dependen de volumen mensual, rubro (salud) y mezcla de tarjetas locales e internacionales. Suelen incluir afiliación, renta mensual y comisión porcentual + fija.
- **API y sandbox:** API REST/SOAP de Cybersource con firmas HMAC-SHA256, Unified Checkout (reduce el alcance PCI-DSS a SAQ A), 3-D Secure 2 nativo y sandbox gratuito en Cybersource Developer Center.
- **Webhooks y reembolsos:** webhooks firmados (`tms.v1.transactions`) y API completa de capturas, anulaciones y reembolsos.
- **Cobro en nombre de terceros:** no está pensado para que una plataforma SaaS reciba fondos de terceros sin licencia o contrato expreso de facilitador de pagos (PayFac). Cada clínica tendría que afiliarse directamente con el banco y conectar sus credenciales.
- **Fuentes:** portales oficiales de BAC Credomatic (`baccredomatic.com`) y Cybersource (`cybersource.com`).

## Puntos sin confirmar (requieren contacto comercial o sandbox)

1. **Disponibilidad de API de onboarding multi-comercio en Panamá:** ningún proveedor local documenta abiertamente un equivalente autoservicio a Stripe Connect Express. Para cobrar en nombre de cada clínica sin convertirse en agregador regulado, el camino más viable es que **cada clínica configure sus propias credenciales de comercio** (por ejemplo, llaves de Tilopay, PagueloFácil o Yappy Comercial) guardadas cifradas en `platform` o en la base de la clínica.
2. **Verificación de firma en webhooks de PagueloFácil:** confirmar el algoritmo y cabecera exacta en su portal técnico privado.
3. **Reembolsos automáticos por API en Yappy Comercial:** confirmar si existe endpoint REST para devolución automatizada ante pagos tardíos o cancelaciones.
4. **Tarifas exactas de adquirencia bancaria (BAC / otros bancos):** solo se obtienen mediante cotización formal de comercio electrónico.

## Qué debe implementar un adaptador local de `PaymentProvider`

Según `docs/design.md` (sección 7), el dominio de pagos depende de la interfaz `PaymentProvider`. Un adaptador para Tilopay, PagueloFácil o Yappy Comercial tendría que cubrir cuatro capacidades y respetar las invariantes de la plataforma:

1. **Conectar la cuenta de la clínica (`ConnectAccount`)**
   - Si el proveedor no soporta onboarding OAuth/Express (caso general en Panamá), el flujo en el panel debe recibir y validar las credenciales propias de la clínica (por ejemplo `Merchant ID`, `API Key`, `Secret`), cifrarlas con la DEK de la clínica (o KMS) y probar la conexión contra el endpoint del proveedor antes de marcarla activa.
   - Registrar en el dominio el identificador externo del comercio sin exponer secretos en logs ni respuestas de la API.

2. **Crear un cobro (`CreateCheckout`)**
   - Recibir `clinicId`, `appointmentId`, monto en centavos (`int64`), moneda (`USD`, equivalente 1:1 con PAB en Panamá), URL de retorno, `Idempotency-Key` y fecha límite del hold (15 minutos).
   - Generar una sesión o enlace de cobro alojado (redirección o componente embebido SAQ A) para que los datos de tarjeta nunca pasen por el servidor Go.
   - Si el proveedor no soporta expiración nativa en el enlace de pago (por ejemplo, si el usuario deja abierta la página más de 15 minutos), el adaptador debe enviar la referencia interna para que el dominio detecte pagos tardíos al recibir el webhook.

3. **Verificar y traducir webhooks (`ParseWebhook`)**
   - Validar la firma criptográfica o HMAC de la petición entrante usando el secreto del comercio correspondiente antes de leer el cuerpo.
   - Devolver un evento interno normalizado (`PaymentSucceeded`, `PaymentFailed`, `PaymentRefunded`) junto con el `providerEventId` único para que el dominio garantice idempotencia en `processed_events`.
   - En proveedores sin `providerEventId` explícito, construir una clave determinista a partir del identificador de transacción y el tipo de operación.

4. **Ejecutar reembolsos (`Refund`)**
   - Soportar reembolso total (cancelación a tiempo y reembolso automático por pago tardío cuando el hold venció y el horario ya se ocupó).
   - Propagar `Idempotency-Key` o verificar el estado previo de la transacción para evitar reembolsos duplicados ante reintentos del worker.
   - En proveedores donde el reembolso automático por API no esté disponible (posible limitación en Yappy Comercial), el adaptador debe devolver un error tipado (`ErrManualRefundRequired`) para que el sistema marque el cobro como pendiente de devolución manual en el panel de la clínica y notifique al personal.
