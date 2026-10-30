# ADR-007 — Diseño de seguridad del módulo Identidad y Acceso (IAM)

- **Estado:** Aceptada para la fase inicial
- **Fecha:** 2026-10-30
- **Decisores:** Equipo del proyecto
- **Relacionado con:** RNF-02, RNF-13, RNF-15, ADR-004, ADR-006

## Contexto

El módulo Identidad y Acceso es el primer módulo del backend Go y expone endpoints públicos de autenticación (`/auth/register`, `/auth/login`, `/auth/m2m/token`) y protegidos (`/api/v1/*`). Debe protegerse contra ataques comunes de fuerza bruta, inyección de headers, clickjacking, y robo de sesiones, sin añadir dependencias externas innecesarias.

MODULES.md establece que la trazabilidad transversal debe relacionar operaciones con actor, recurso, ejecución y evidencia. RNF-02 exige RBAC y cero secretos expuestos. RNF-13 exige `request_id` en logs.

## Problema

Definir las medidas de seguridad mínimas para el módulo IAM en Report 1.0:

1. ¿Qué algoritmo de hash de contraseñas usar?
2. ¿Qué algoritmo de firma JWT usar?
3. ¿Qué headers de seguridad HTTP agregar?
4. ¿Cómo prevenir ataques de fuerza bruta en login?
5. ¿Cómo rotar tokens de forma segura?

## Alternativas consideradas

### Hash de contraseñas

#### Alternativa A: bcrypt
**Ventajas:** Estándar ampliamente usado, implementado en `golang.org/x/crypto/bcrypt`.
**Desventajas:** Solo CPU-hard; vulnerable a ataques con GPU/ASIC que paralelizan bien.

#### Alternativa B: Argon2id (seleccionada)
**Ventajas:** Ganador del Password Hashing Competition 2015. Memory-hard + CPU-hard. Resistente a ataques con GPU/ASIC porque requiere mucha memoria. Parámetros ajustables (memoria, iteraciones, paralelismo).
**Desventajas:** Ligeramente más lento que bcrypt en configuraciones bajas.

#### Alternativa C: scrypt
**Ventajas:** Memory-hard, estándar IETF RFC 7914.
**Desventajas:** Menos adoption que Argon2id; parámetros más difíciles de tuning.

### Firma de JWT

#### Alternativa A: HS256 (actual)
**Ventajas:** Simple, una sola clave simétrica, rápido.
**Desventajas:** La misma clave firma y verifica; rotación de claves más difícil en múltiples instancias; si se filtra la clave, se pueden forjar tokens.

#### Alternativa B: RS256
**Ventajas:** Firma asimétrica (clave privada firma, clave pública verifica). Rotación de claves más sencilla. No se necesita compartir la clave privada con servicios que solo verifican.
**Desventajas:** Requiere gestión de pares de claves RSA; ligeramente más lento.

#### Alternativa C: EdDSA (Ed25519)
**Ventajas:** Más rápido que RSA, claves más pequeñas, seguridad superior, determinista.
**Desventajas:** Menos adoption en ecosistemas legacy; soporte en librerías JWT varía.

### Security headers HTTP

#### Alternativa A: Ninguno
**Desventajas:** Navegadores son vulnerables a MIME sniffing, clickjacking, referrer leakage.

#### Alternativa B: Headers básicos (seleccionada)
**Ventajas:** `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Content-Security-Policy`, `Strict-Transport-Security`. Costo cero, protección inmediata.

### Rate limiting y account lockout

#### Alternativa A: Ninguno
**Desventajas:** Ataques de fuerza bruta contra `/auth/login` ilimitados.

#### Alternativa B: Rate limiting por IP + account lockout por email (seleccionada)
**Ventajas:** Rate limiting detiene spam a nivel de red; account lockout detiene fuerza bruta contra cuentas específicas.

## Criterios de evaluación

| Criterio | Peso | Argon2id | bcrypt | scrypt |
|---|---:|---:|---:|---:|
| Resistencia a GPU/ASIC | 5 | 5 | 2 | 5 |
| Parámetros ajustables | 4 | 5 | 2 | 4 |
| Stdlib/stdlib-adjunto | 5 | 5 | 5 | 5 |
| Simplicidad de implementación | 4 | 4 | 5 | 4 |

| Criterio | Peso | HS256 | RS256 | EdDSA |
|---|---:|---:|---:|---:|
| Simplicidad | 5 | 5 | 2 | 2 |
| Rotación de claves | 4 | 2 | 5 | 5 |
| Rendimiento | 5 | 5 | 3 | 4 |
| Seguridad criptográfica | 5 | 3 | 5 | 5 |

## Decisión

Se decide:

1. **Hash de contraseñas: Argon2id** con parámetros: memoria 64 MB, 3 iteraciones, 4 hilos, clave de 32 bytes, sal de 16 bytes. Implementado con `golang.org/x/crypto/argon2` y comparación en tiempo constante (`crypto/subtle.ConstantTimeCompare`).

2. **Firma JWT: HS256** por simplicidad en Report 1.0 (monolito single-instance). Se documenta como `shortcut` con camino de upgrade a RS256 o EdDSA cuando haya múltiples instancias o requisito de rotación de claves.

3. **Security headers:** `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Content-Security-Policy` restrictivo, `Strict-Transport-Security`. Aplicados a todas las respuestas HTTP.

4. **Rate limiting:** 60 requests por minuto por IP (ventana deslizante), aplicado globalmente.

5. **Account lockout:** Máximo 5 intentos fallidos de login por email, bloqueo por 15 minutos. Se reinicia en login exitoso.

6. **Refresh token con rotación:** Cada uso de refresh token revoca el anterior y emite uno nuevo. Almacenamiento en memoria (`sync.Map`).

## Justificación

Argon2id es el estándar actual para hash de contraseñas (ganador PHC 2015). HS256 es suficiente para un monolito single-instance pero se marca explícitamente como `shortcut` con camino a RS256/EdDSA. Los security headers y el rate limiting son de costo cero y protección inmediata contra ataques comunes.

## Consecuencias

### Positivas

- Contraseñas protegidas con hash memory-hard resistente a GPU/ASIC.
- Logs estructurados con `request_id`/`correlation_id` para trazabilidad (RNF-13).
- Rate limiting y account lockout reducen drásticamente el riesgo de fuerza bruta.
- Security headers protegen contra MIME sniffing, clickjacking y referrer leakage.
- Refresh token rotation limita el impacto de robo de tokens.

### Negativas

- Argon2id es más lento que bcrypt (acceptable para login esporádico).
- Rate limiting y account lockout están en memoria; no sobreviven un reinicio ni se distribuyen entre instancias.
- JWT HS256 con clave simétrica única requiere rotación manual de la variable de entorno.

### Riesgos

- Si `CCO_JWT_SIGNING_KEY` se filtra, se pueden forjar tokens hasta que se rote.
- Account lockout puede ser usado para denegación de servicio (bloquear cuentas intencionalmente). Mitigación: lockout temporal de 15 min, no permanente.

## Evidencia

- `backend/internal/iam/auth.go` — Argon2id hash, account lockout, `LoginLockout`
- `backend/internal/iam/handlers.go` — LoginHandler con lockout, RegisterHandler con Argon2id, TokenRefreshHandler con rotación
- `backend/internal/iam/middleware/security.go` — Rate limiting, CORS, SecurityHeaders
- `backend/cmd/cco-app/main.go` — Chain de middlewares

## Revisión

- **Fecha prevista de revisión:** Antes de producción, o cuando se desplieguen múltiples instancias de backend.
- **Condición que podría provocar una revisión:** Necesidad de RS256/EdDSA, rate limiting distribuido, o cumplimiento que exija HSM para claves de firma.

## Referencias

[1]: https://github.com/p-h-c/phc-winner-argon2 "Argon2 — Password Hashing Competition winner"
[2]: https://datatracker.ietf.org/doc/html/rfc9106 "Argon2 Memory-Hard Function for Password Hashing and Applications"
[3]: https://datatracker.ietf.org/doc/html/rfc7518 "JSON Web Signature (JWS) Algorithms"
[4]: https://owasp.org/www-community/attacks/Brute_force_attack "OWASP Brute Force Attack"