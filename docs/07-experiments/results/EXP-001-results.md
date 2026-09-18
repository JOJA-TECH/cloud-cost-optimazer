# EXP-001 — Resultados

## Resumen

La prueba de integración fue técnicamente satisfactoria. Las fuentes principales devolvieron respuestas utilizables para construir el pipeline inicial. La evidencia no es suficiente para validar ahorro, forecasting ni recomendaciones de capacidad.

## Evidencia cuantitativa

| Área | Resultado |
|---|---|
| Configuración | Recurso online en Australia East, General Purpose Gen5 serverless, 2 vCores configurados |
| Costos | 11 filas entre el 1 y el 5 de septiembre; SQL Database con USD 0 |
| Precios | 343 elementos de SQL Database en Australia East |
| Métricas | 5 series con 15 puntos por serie a intervalos de un minuto |
| Logs | 20 filas a intervalos de un minuto |
| Query Store | 10 textos, 10 consultas, 20 estadísticas de ejecución y 20 esperas |
| Database watcher | 50 observaciones de recursos y 6 sesiones |
| Carga persistida | 239.006 filas |
| Advisor | 9 recomendaciones, sin rightsizing comparable de Azure SQL |

## Resultados técnicos

La integración de configuración, métricas, precios, costos, logs, Query Store, Database watcher, Advisor y tabla de carga funciona. La carga persistida demuestra que la base puede recibir datos y que es posible consultar el estado de la tabla.

## Resultados metodológicos

La ventana de observación es demasiado corta para obtener un histórico representativo. El costo SQL igual a cero impide usar la facturación observada como única referencia económica. El catálogo de precios necesita filtrado semántico. La carga no contiene todavía todas las etiquetas necesarias para una comparación experimental controlada.

## Interpretación

El resultado apoya la decisión de continuar con Azure SQL como dominio de Release 1.0. También obliga a modificar el plan experimental. La siguiente fase debe centrarse en recolección histórica, definición de escenarios, medición de latencia de cliente, throughput, SLA y costo contrafactual.

## Acciones derivadas

| Acción | Prioridad | Estado |
|---|---|---|
| Definir una ventana histórica mínima | Crítica | Pendiente |
| Implementar contrato normalizado de precios | Crítica | Pendiente |
| Etiquetar cada workload y configuración | Crítica | Pendiente |
| Separar costo facturado y costo contrafactual | Crítica | Pendiente |
| Incorporar latencia, throughput, errores y SLA | Crítica | Pendiente |
| Restringir el acceso público del laboratorio | Alta | Pendiente |
| Registrar versiones y parámetros completos de ejecución | Alta | Pendiente |
| Repetir el laboratorio con histórico extendido | Alta | Pendiente |

## Veredicto

**Factibilidad de integración: aceptada.**

**Validación de optimización: pendiente.**

**Benchmark comparativo: no ejecutado.**
