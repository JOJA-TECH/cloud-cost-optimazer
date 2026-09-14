# LAB-001 — Laboratorio: Verificación de factibilidad de las APIs de Azure

- **Título:** Verificación de factibilidad de las APIs de Azure para el optimizador de costos de Azure SQL Database
- **Fecha:** Captura 2026-09-06 (ventana de ~3.7 h); informe 2026-09-13
- **Responsable:** Jesús (datos/ML y laboratorios)
- **Participantes:** Jesús
- **Estado:** Cerrado

## Objetivo

Comprobar si las APIs y fuentes de Azure entregan los datos necesarios para que la propuesta de plataforma de análisis y optimización de costos de Azure SQL Database sea técnicamente viable.

## Pregunta

¿Las APIs de Azure (ARM, Cost Management, Retail Prices, Monitor Metrics, Log Analytics, Query Store/DMVs, Advisor y estado de tabla SQL) entregan los datos necesarios para construir la plataforma propuesta?

## Hipótesis

Las fuentes de Azure consultadas entregan los datos necesarios para validar experimentalmente la propuesta, sin necesidad de implementar el sistema final (motor de recomendaciones, forecast, baselines, autoescalamiento).

## Contexto

- [MTG-001](../01-governance/meetings/MTG-001.md): definición del proyecto y tareas; TASK-006 asigna a Jesús la investigación de precios reales y métricas de Azure SQL.
- [DEC-012](../01-governance/DECISION-LOG.md): el usuario provee su propia API key del proveedor cloud.
- [DEC-013](../01-governance/DECISION-LOG.md): laboratorios con Terraform.
- Este laboratorio **solo verifica disponibilidad de datos**: no implementa el sistema final ni modifica recursos productivos; no se almacenan secretos.

## Entorno

### Hardware

No relevante: las consultas son llamadas REST y consultas SQL de administración.

### Sistema operativo

Linux (entorno local de desarrollo).

### Software / versiones

- Python 3.14+
- `uv` (gestor de entornos)
- ODBC Driver 18 (conexión a Azure SQL)
- Terraform (infraestructura desplegada en el laboratorio)
- APIs REST de Azure consultadas directamente

## Materiales

- Suscripción `e44b10e0-cdae-4525-b479-b9ad9454f0d1`, región `australiaeast`
- Grupo de recursos `rg-proyecto-grado-free`
- Servidor SQL `sql-cuddly-moray.database.windows.net` (v12.0)
- Escenario `db-1`: base de datos `proyectogrado` (SKU `GP_S_Gen5_2`, serverless, nivel gratuito `useFreeLimit=true`)
- Escenario `db-2`: base de datos con costo (esquema pagado), para el KPI económico
- Workspace Log Analytics `la-proyectogrado-free`
- Tabla de telemetría `dbo.carga_telemetria`
- Scripts `run_all.py` y `feasibility.py` (directorio `python/`)

## Dataset

- DATA-001 (capturas de respuestas de las APIs de Azure)
- Versión: 1.0

## Variables

### Independientes

- Fuente/API consultada (8 fuentes).

### Dependientes

- Éxito de la consulta
- Cantidad de registros devueltos
- Campos y tipos disponibles
- Unidades
- Cobertura temporal
- Datos faltantes (nulos/vacíos)

### Controladas

- Suscripción, región, servidor, base de datos consultados
- Ventana de captura (2026-09-05/06)

## Procedimiento

1. Desplegar infraestructura con Terraform (`rg-proyecto-grado-free`, SQL serverless en nivel gratuito, workspace Log Analytics y tabla de telemetría).
2. Configurar `python/.env` con las credenciales de acceso (sin versionar secretos).
3. Ejecutar la captura de todas las fuentes:

   ```bash
   cd python
   uv run python run_all.py --duration 5m
   ```

4. Verificar que cada fuente genere `respuesta.json` (original), `estructura.json` (esquema) y `resumen.json` (fecha UTC, estado, parámetros sin secretos, HTTP, errores y advertencias) en `data/responses/<fuente>/`.
5. Generar los artefactos de factibilidad:

   ```bash
   cd python
   uv run python feasibility.py --out data/feasibility
   ```

6. Revisar el informe de factibilidad, el inventario de campos y la cobertura temporal.
7. Consolidar los resultados por fuente y emitir el veredicto de viabilidad.

## Configuración

Configuración anonimizada y sin secretos en `laboratory-001/db-2/data/feasibility/config_anonimizado.json` (suscripción, región, RG, servidor SQL, BD, resource ids y workspace; se excluyen `sql_password`, `telegram_token`, `telegram_chat_id` y `authorization`).

## Resultados

### Evaluación por fuente

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

### Veredicto

**La propuesta puede continuar técnicamente de forma incremental.** Todas las fuentes respondieron; las de configuración, precios, métricas y Query Store ya entregan campos suficientes para validar la propuesta. Conectar una API no equivale a validar la propuesta: costos, logs y Advisor requieren historial acumulado, permisos verificados y, para el KPI económico, una BD fuera del nivel gratuito durante un ciclo de facturación.

## Observaciones

- La BD gratuita (`SQL Database`) muestra **costo 0** durante la captura: el costo cero no equivale a ausencia de costo económico. El escenario `db-2` (BD con costo) es el previsto para medir el KPI económico en un ciclo de facturación.
- Advisor en el laboratorio no devolvió recomendaciones de categoría `Cost`, solo `HighAvailability` y `OperationalExcellence`.
- Las ventanas cortas (15 min de métricas, 20 filas de logs) son insuficientes para tendencias estacionales.

## Problemas

- No se registraron fallos de autenticación en la captura; sin embargo, si una API falla por `AuthorizationFailed/403`, se debe revisar el rol sobre el scope (Cost Management Reader, `Microsoft.Insights`, `Microsoft.Advisor`).
- Logs: requiere el workspace vinculado a la BD y retención configurada.
- Query Store: depende de que esté habilitado y de los permisos sobre la BD.

## Repetibilidad

¿Puede otra persona repetir el laboratorio con esta documentación?

Sí. Las instrucciones completas están en `laboratory-001/db-2/INSTRUCCIONES_REPETICION.md` (pre-requisitos, comandos, consultas por fuente y criterios de revisión).

## Artefactos

- Código: `python/run_all.py`, `python/feasibility.py`
- Resultados crudos: `laboratory-001/db-1/data/responses/`, `laboratory-001/db-2/data/responses/`
- Informe de factibilidad: `laboratory-001/db-2/informe_factibilidad.md`
- Datos de proveedores: `laboratory-001/db-1/DATOS_PROVEEDORES.md`
- Análisis: `laboratory-001/db-2/data/feasibility/` (inventario de campos, cobertura temporal, fuentes disponibles/faltantes, errores, hashes, configuración anonimizada)
- Instrucciones de repetición: `laboratory-001/db-2/INSTRUCCIONES_REPETICION.md`

## Conclusión

La plataforma de optimización de costos es técnicamente viable de forma incremental. Se mantiene: baseline por configuración (ARM), cotización de candidatos (Retail Prices), modelado FOCUS del costo (Cost Management con `resource_id`) y caracterización de carga (Métricas + Query Store). Se debe modificar: no construir aún motor de recomendación/forecast hasta acumular histórico y medir un KPI económico real con capacidad pagada.

## Limitaciones

- No implementa el sistema final ni evalúa el rendimiento de un motor de recomendaciones.
- Sin histórico acumulado: métricas, logs y Query Store cubren ventanas de minutos/horas.
- El costo de la BD gratuita es 0, por lo que no se pudo medir un KPI económico real sobre el recurso SQL.

## Documentos relacionados

- MTG-001 — `docs/01-governance/meetings/MTG-001.md` (decisiones DEC-001 a DEC-019, TASK-006)
- DATA-001 — `docs/06-data/DATA-001-captura-respuestas-azure.md`
- `laboratory-001/db-1/DATOS_PROVEEDORES.md`
- `laboratory-001/db-2/informe_factibilidad.md`
- `laboratory-001/db-2/INSTRUCCIONES_REPETICION.md`