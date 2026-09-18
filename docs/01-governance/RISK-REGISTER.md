# Risk Register

| ID | Riesgo | Prob. (1-5) | Impacto (1-5) | Score | Mitigación | Responsable | Estado |
|---|---|---:|---:|---:|---|---|---|
| RSK-002 | Histórico insuficiente para forecasting y percentiles confiables | 5 | 5 | 25 | Definir ventana mínima, recolectar datos continuamente y documentar cobertura | Equipo del proyecto | Abierto |
| RSK-003 | Costo facturado de SQL igual a cero por nivel gratuito o crédito | 5 | 5 | 25 | Separar costo facturado, precio de lista y costo contrafactual | Equipo del proyecto | Abierto |
| RSK-004 | Catálogo de precios mezcla modalidades, tiers, reservas y componentes | 4 | 5 | 20 | Implementar contrato de precios por región, modalidad, tier, unidad, tipo y vigencia | Equipo del proyecto | Abierto |
| RSK-005 | Carga sintética sin etiquetas suficientes para comparación experimental | 4 | 5 | 20 | Etiquetar workload, configuración, concurrencia, throughput, latencia, errores y SLA | Equipo del proyecto | Abierto |
| RSK-006 | Exposición pública del servidor de laboratorio | 3 | 5 | 15 | Restringir red, aplicar RBAC, revisar secretos y realizar hardening antes de RC | Equipo del proyecto | Abierto |
| RSK-007 | Dos bases locales pueden generar inconsistencias o problemas de sincronización | 3 | 4 | 12 | Definir contratos de datos, responsabilidades, migraciones y mecanismo explícito de intercambio | Equipo del proyecto | Abierto |
| RSK-008 | Meta de respuesta menor a 30 segundos sin protocolo definido | 4 | 4 | 16 | Definir inicio, fin, percentil, volumen, entorno y número de ejecuciones antes de validar | Equipo del proyecto | Abierto |
| RSK-009 | No recibir retroalimentación académica antes de la entrega formal | 3 | 4 | 12 | Entregar beta anticipada, registrar contacto y documentar observaciones o ausencia de respuesta | Equipo del proyecto | Abierto |

## Escala

- 1 = muy bajo
- 2 = bajo
- 3 = medio
- 4 = alto
- 5 = muy alto

**Score = Probabilidad × Impacto**
