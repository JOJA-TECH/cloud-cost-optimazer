# REQ-003 — Consulta de recursos, métricas y sugerencias

- **Tipo:** Funcional
- **Prioridad:** Alta
- **Fuente:** Stakeholder / Reunión MTG-002
- **Estado:** Propuesto
- **Fecha:** 2026-09-12

## Descripción

El sistema deberá permitir que el usuario visualice sus recursos registrados, seleccione un recurso y consulte sus estadísticas, métricas y sugerencias de análisis.

La vista deberá contemplar, cuando estén disponibles, costo actual, capacidad, lecturas, almacenamiento, red y otros indicadores relacionados con el consumo del recurso.

## Justificación

El usuario necesita relacionar un recurso específico con su comportamiento, costo y sugerencias. La selección de recursos es el punto de entrada para el análisis.

## Criterio de aceptación

El requisito se considera cumplido cuando el usuario puede visualizar una lista de recursos, seleccionar uno y obtener una vista con métricas, estadísticas y sugerencias asociadas.

## Método de validación

Prueba funcional y demostración.

## Evidencia esperada

Caso de prueba, captura o video de la vista, datos de prueba y registro de la consulta.

## Dependencias

Datos de configuración y métricas disponibles. REQ-002.

## Riesgos

Las métricas pueden no estar disponibles para todos los recursos o pueden tener ventanas temporales diferentes.

## Trazabilidad

- Problema: Falta de una vista consolidada para analizar recursos cloud.
- Objetivo: Facilitar el análisis de costos y rendimiento.
- Componente: Frontend y API de consulta.
- TEST-003: Pendiente.
- RESULT-003: Pendiente.
