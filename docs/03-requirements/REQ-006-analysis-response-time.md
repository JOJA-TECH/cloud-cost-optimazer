# REQ-006 — Tiempo preliminar de respuesta del análisis

- **Tipo:** No funcional
- **Prioridad:** Media
- **Fuente:** Stakeholder / Reunión MTG-002
- **Estado:** Propuesto; pendiente de precisión
- **Fecha:** 2026-09-12

## Descripción

El flujo de análisis y recomendación deberá generar resultados en menos de 30 segundos bajo condiciones controladas de laboratorio. El equipo deberá definir posteriormente el punto de inicio, el punto de finalización, el tamaño de la consulta, la cantidad de recursos y el percentil utilizado.

## Justificación

Un tiempo de respuesta corto es necesario para que la plataforma sea útil durante la consulta interactiva del usuario.

## Criterio de aceptación

El requisito se considera cumplido cuando el tiempo total del flujo se mide con un protocolo documentado y el indicador definido se mantiene por debajo de 30 segundos en las condiciones de prueba acordadas.

## Método de validación

Benchmark.

## Evidencia esperada

Protocolo de medición, número de ejecuciones, tiempos observados, percentil reportado, entorno y logs.

## Dependencias

La definición del flujo completo de análisis y recomendación.

## Riesgos

La meta puede ser ambigua si no se define el percentil, el volumen de datos, el estado de caché y el alcance exacto de la operación.

## Trazabilidad

- Problema: Necesidad de respuesta rápida durante el análisis.
- Objetivo: Ofrecer una herramienta interactiva.
- Componente: Backend de análisis y frontend.
- TEST-006: Pendiente.
- RESULT-006: Pendiente.
