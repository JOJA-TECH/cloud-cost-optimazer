# Traceability Matrix

| Problema/Evidencia | Objetivo | Requisito | Diseño/Componente | Experimento | Prueba | Resultado | Estado |
|---|---|---|---|---|---|---|---|
| | | REQ-XXX | | EXP-XXX | TEST-XXX | RESU-XXX | |
| ¿Las APIs de Azure entregan los datos necesarios? (MTG-001, TASK-006) | Verificar disponibilidad de datos de las APIs de Azure | REQ-003 | Conector por proveedor (DEC-007) | LAB-001 (datos: DATA-001) | RES-002 (catálogo de métricas y contratos JSON) | Veredicto: viable incrementalmente | Cerrado |
| Disponibilidad de fuentes para recomendación de capacidad (MTG-002, DEC-020) | Confirmar datos de configuración, costos, precios, métricas y telemetría | REQ-002, REQ-003, REQ-006 | Arquitectura de adquisición y persistencia (ADR-002) | EXP-001, LAB-001 (datos: DATA-001) | | Integración aceptada; validación de optimización pendiente | Ejecutado |
| Recomendaciones sin cambios automáticos (MTG-002, DEC-022) | El sistema solo sugiere, nunca aplica cambios en Azure | REQ-004 | Motor de Recomendaciones (ADR-013) | EXP-001 | TEST-004 (pendiente) | Resultado: informativo únicamente | Pendiente de verificación |
| Entrega beta al profesor (MTG-002, DEC-021) | Versión revisable antes de la entrega formal | REQ-005 | Documentación académica | MTG-002 | TEST-005 (no aplica) | Beta entregada | Pendiente |
| Tiempo de respuesta del análisis < 30s (MTG-002) | Protocolo de medición definido y cumplido | REQ-006 | Backend de análisis y frontend | | TEST-006 (pendiente) | Protocolo no definido | Pendiente de precisión |
| Despliegue local inicial con PostgreSQL (MTG-002) | Dos bases levantadas localmente con migraciones | REQ-007 | Persistencia (ADR-002) | | TEST-007 (pendiente) | Prueba de conexión pendiente | Pendiente |

## Regla

Todo requisito crítico debe terminar conectado con una evidencia de verificación.
