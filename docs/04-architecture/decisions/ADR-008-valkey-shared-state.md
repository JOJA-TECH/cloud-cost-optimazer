# ADR-008 — Adopción de Valkey para estado compartido (rate limiting y account lockout)

- **Estado:** Aceptada para la fase actual
- **Fecha:** 2026-11-11
- **Decisores:** Equipo del proyecto
- **Relacionado con:** ADR-007, ADR-006, RNF-02, RNF-15, DEC-031

## Contexto

El estado anti-abuso definido en ADR-007 (rate limiter por IP y lockout de cuentas) vivía en memoria del proceso: se perdía al reiniciar y no se compartía entre instancias. El propio ADR-007 documentaba esta limitación como negativa y fijaba "rate limiting distribuido" y "múltiples instancias" como condiciones de revisión.

## Problema

1. ¿Dónde reside el estado anti-abuso compartido (rate limiting y account lockout)?
2. ¿Qué cliente se usa para acceder a él?
3. ¿Qué ocurre cuando el almacén no está disponible (modo dev sin infraestructura, fallo en tiempo de ejecución)?

## Alternativas consideradas

### Alternativa A: Mantener el estado en memoria

**Ventajas**
- Sin dependencias ni infraestructura adicional.

**Desventajas**
- El estado se pierde al reiniciar y no se comparte entre instancias (limitación ya documentada en ADR-007).

### Alternativa B: go-redis

**Ventajas**
- Cliente maduro y ampliamente usado en el ecosistema Go.

**Desventajas**
- No es el cliente oficial de Valkey; añade una capa de compatibilidad innecesaria.

### Alternativa C: valkey-go, cliente oficial de Valkey (seleccionada)

**Ventajas**
- Cliente oficial del proyecto Valkey (fork de Redis bajo Linux Foundation, compatible con el protocolo RESP).
- Permite modo degradado sin infraestructura: cada componente conserva su fallback en memoria.

**Desventajas**
- Dependencia nueva.
- Cuando está configurado, el backend requiere salida de red hacia Valkey.

## Criterios de evaluación

| Criterio | Peso | A: memoria | B: go-redis | C: valkey-go |
|---|---:|---:|---:|---:|
| Estado compartido entre instancias | 5 | 1 | 5 | 5 |
| Sobrevive reinicios | 4 | 1 | 5 | 5 |
| Soporte oficial del proyecto Valkey | 3 | - | 2 | 5 |
| Costo en modo dev (sin infraestructura) | 5 | 5 | 4 | 5 |

## Decisión

Se decide **Alternativa C**: cliente oficial `github.com/valkey-io/valkey-go` v1.0.79. Se evaluó go-redis y se descartó en favor del cliente oficial de Valkey.

1. **Cliente compartido** en `backend/internal/platform/valkey`; `NewFromEnv()` lee `CCO_VALKEY_ADDR`, `CCO_VALKEY_PASSWORD` y `CCO_VALKEY_DB`.
2. **`CCO_VALKEY_ADDR` vacía** → cliente nil → cada componente usa su fallback en memoria (modo single-instance dev).
3. **`CCO_VALKEY_ADDR` configurada** → ping de 3 s al arranque; la aplicación no inicia si Valkey no responde.
4. **Rate limiter:** ventana fija por IP, clave `cco:rl:<ip>` con `INCR` + `EXPIRE NX`, 60 req/min.
5. **Account lockout:** `cco:lock:attempts:<email>` con `INCR`/`EXPIRE NX` 15 min y `cco:lock:locked:<email>` con `SET EX`.
6. **Política fail-open:** ante error de Valkey se permite la operación y se registra warning (disponibilidad sobre rigor; política de prototipo).

## Justificación

Valkey es el sucesor compatible de Redis mantenido bajo Linux Foundation; usar su cliente oficial evita depender de un cliente de terceros para un protocolo que Valkey define. El fallback en memoria preserva el modo dev sin infraestructura, y el ping al arranque convierte un Valkey mal configurado en un fallo temprano y visible en lugar de un fallo silencioso en runtime. El fail-open acepta degradar el rigor anti-abuso antes que la disponibilidad, coherente con la política de prototipo.

## Consecuencias

### Positivas

- El estado anti-abuso sobrevive reinicios y se comparte entre instancias.
- El modo dev sin Valkey sigue funcionando (fallback en memoria, cliente nil).
- Un Valkey configurado pero caído impide el arranque (fallo temprano) en lugar de degradar silenciosamente.
- Los errores de Valkey en runtime no bloquean operaciones (fail-open con warning).

### Negativas

- La directiva `go` del módulo subió de 1.22 a 1.25.0 (requerida por la dependencia).
- Dependencia nueva de test: `github.com/alicebob/miniredis/v2` v2.33.0.
- Cuando Valkey está configurado, el backend requiere salida de red hacia Valkey.

### Riesgos

- El fail-open degrada el rate limiting y el lockout durante una caída de Valkey: se permiten operaciones que normalmente se bloquearían. Mitigación: warning en logs; aceptado como política de prototipo, a revisar antes de producción.

## Evidencia

- `backend/internal/platform/valkey` — cliente compartido y `NewFromEnv()`.
- `backend/internal/iam/middleware/security.go` — rate limiter y account lockout (consumidores del cliente).

## Revisión

- **Fecha prevista de revisión:** Antes de producción, o cuando la política fail-open deje de ser aceptable.
- **Condición que podría provocar una revisión:** Requisito de fail-closed en endpoints de autenticación, alta disponibilidad de Valkey, o despliegue multi-instancia con requisitos de rigor anti-abuso.

## Referencias

[1]: https://github.com/valkey-io/valkey-go "valkey-go — Cliente oficial de Valkey para Go"
[2]: https://valkey.io "Valkey — Linux Foundation"
[3]: docs/04-architecture/decisions/ADR-007-security-design.md — Rate limiting y account lockout originales en memoria
