# LAB-001 — Integración de fuentes de datos de Azure SQL

## Identificación

- **ID:** LAB-001
- **Título:** Integración inicial y verificación de fuentes para Cloud Database Cost Optimizer
- **Fecha:** 2026-09-06
- **Responsable:** Equipo del proyecto
- **Participantes:** Equipo del proyecto
- **Estado:** Ejecutado; pendiente de repetición con histórico extendido

## Objetivo

Verificar que el laboratorio puede consultar configuración, costos, precios, métricas, logs, telemetría de consultas y datos de carga de Azure SQL.

## Pregunta

¿Las fuentes disponibles permiten construir un dataset inicial para analizar costo, utilización, comportamiento del workload y configuración de Azure SQL?

## Hipótesis

Las fuentes disponibles permiten construir la base técnica de Release 1.0, aunque pueden existir limitaciones de cobertura, costo facturado, granularidad y permisos.

## Contexto

El laboratorio corresponde a la primera prueba de integración del proyecto de grado. Su resultado debe orientar el diseño de la recolección histórica y del experimento comparativo. No pretende demostrar todavía una reducción de costos.

## Entorno

### Hardware

No registrado en el paquete de respuestas. El entorno de ejecución del cliente fue identificado en algunas sesiones como `vm-extractor`.

### Sistema operativo

No registrado de forma suficiente para reproducibilidad completa.

### Software / versiones

Se utilizaron APIs y consultas de Azure SQL, Azure Monitor, Log Analytics, Cost Management, Retail Prices, ARM/SQL, Query Store, Database watcher y Advisor. Las versiones exactas de cliente no quedaron registradas y deben incorporarse en la siguiente ejecución.

## Materiales

- Recurso Azure SQL de laboratorio.
- Suscripción y grupo de recursos de laboratorio.
- Consultas de integración.
- Paquete de respuestas JSON.
- Tabla sintética `dbo.carga_telemetria`.

## Dataset

- **DATA-001**
- **Versión:** 1.0-inicial

## Variables

### Independientes

La fuente consultada, el tipo de métrica, el periodo de consulta, la granularidad, la configuración observada y la familia de precio fueron las variables de análisis de esta fase.

### Dependientes

Se observó si la consulta devolvía datos, la cantidad de registros, los campos disponibles, la cobertura temporal, los valores de costo, las métricas de utilización y la posibilidad de relacionar las fuentes.

### Controladas

Se mantuvieron el recurso de laboratorio, la región, el grupo de recursos y el periodo de ejecución registrados en las respuestas.

## Procedimiento

1. Registrar la configuración del recurso y del servidor.
2. Consultar costos agregados.
3. Consultar precios regionales.
4. Consultar métricas y logs.
5. Consultar Query Store y Database watcher.
6. Verificar la tabla y la carga persistida.
7. Calcular cobertura y revisar limitaciones.
8. Registrar la decisión de continuar con cambios en el plan experimental.

## Configuración

La base analizada se encontraba en General Purpose, Gen5 y serverless, con 2 vCores configurados, mínimo de 0,5 vCore, pausa automática de 60 minutos y nivel gratuito activo. El servidor tenía acceso público habilitado y TLS mínimo 1.2.

## Resultados

La conectividad de las fuentes fue satisfactoria. Azure Monitor devolvió cinco métricas en una ventana de 15 minutos. Log Analytics devolvió 20 filas en una ventana aproximada de siete minutos. Cost Management devolvió 11 filas entre el 1 y el 5 de septiembre, incluyendo una fila de SQL Database con costo USD 0. La tabla sintética contenía 239.006 filas.

### Detalle de resultados por fuente

| Fuente | Consulta exitosa | Registros | Datos necesarios | Cobertura | Recomendación |
|---|---|---|---|---|---|
| Configuración SQL (ARM) | Si | 4 | Completa | Si (snapshot puntual) | suficiente |
| Costos (Cost Management) | Si | 11 | Parcial | Parcial (mes en curso) | parcialmente suficiente |
| Precios (Retail Prices) | Si | 343 | Completa | Si (catálogo vigente) | suficiente |
| Métricas (Monitor Metrics) | Si | 5 (75 puntos) | Parcial | No (PT15M) | suficiente |
| Logs (Log Analytics) | Si | 20 | Parcial | No (ventana corta) | parcialmente suficiente |
| Query Store + Watcher (DMVs) | Si | 127 | Completa | Parcial (histórico en acumulación) | suficiente |
| Estado tabla telemetría | Si | 239 006 | Completa | Si | suficiente |
| Azure Advisor | Si | 9 | Parcial | No (sin categoría Cost) | complementaria |

Los escenarios de captura corresponden a `db-1` (base de datos gratuita, `useFreeLimit=true`) y `db-2` (base de datos con costo, prevista para el KPI económico); el detalle del contenido por proveedor está en `laboratory-001/db-1/DATOS_PROVEEDORES.md` y el análisis de factibilidad en `laboratory-001/db-2/informe_factibilidad.md`.

## Observaciones

Los datos permiten avanzar en la arquitectura de adquisición. El catálogo de precios requiere normalización porque mezcla modalidades y componentes. La muestra histórica no permite evaluar picos, estacionalidad ni forecasting. La carga persistida demuestra ingestión, pero todavía no está etiquetada como conjunto de workloads experimentales.

## Problemas

- Histórico insuficiente para la hipótesis académica.
- Costo facturado de SQL igual a cero por posible beneficio gratuito.
- Azure Advisor sin recomendación comparable de rightsizing SQL.
- Falta de registro completo del entorno cliente.
- Acceso público habilitado en el recurso de laboratorio.

## Repetibilidad

¿Puede otra persona repetir el laboratorio con esta documentación? **Parcialmente.** Puede repetir la lógica general y consultar fuentes equivalentes, pero debe agregarse la versión exacta de los clientes, parámetros de consulta, permisos, intervalos y comandos completos.

## Artefactos

- **Código:** `python/run_all.py` y `python/feasibility.py`; consultas de integración utilizadas por el equipo.
- **Notebook:** No aplica.
- **Resultados:** capturas en `laboratory-001/db-1/data/responses/` y `laboratory-001/db-2/data/responses/`; `responses.zip`, hash registrado en DATA-001.
- **Análisis:** `laboratory-001/db-2/informe_factibilidad.md` e `INSTRUCCIONES_REPETICION.md`; artefactos de `laboratory-001/db-2/data/feasibility/` (inventario de campos, cobertura temporal, fuentes disponibles/faltantes, errores, hashes y configuración anonimizada).
- **Gráficos:** No generados en esta fase.
- **Logs:** Respuestas de Azure Monitor y Log Analytics incluidas en el paquete.

## Conclusión

El laboratorio valida la factibilidad de integración, pero no valida todavía la optimización de capacidad. La siguiente ejecución debe ampliar la ventana histórica, definir workloads etiquetados y recolectar métricas de rendimiento de cliente.

## Limitaciones

Los resultados no deben interpretarse como un benchmark, una recomendación de cambio de capacidad o una estimación definitiva de ahorro. La validez externa está limitada a un recurso de laboratorio y a una ventana corta.

## Documentos relacionados

- `07-experiments/EXP-001-api-integration-data-feasibility.md`
- `06-data/DATA-001-azure-integration-responses.md`
- `07-experiments/results/EXP-001-results.md`
- `registry/EXPERIMENT-INDEX.md`

## Referencias

[1]: https://learn.microsoft.com/en-us/azure/azure-sql/database/monitoring-metrics-alerts "Microsoft Learn: Monitor Azure SQL Database with metrics and alerts"
[2]: https://learn.microsoft.com/en-us/azure/cost-management-billing/costs/understand-cost-mgt-data "Microsoft Learn: Understand Azure Cost Management data"
