# REQ-004 — Recomendaciones sin aplicación automática

- **Tipo:** Restricción funcional y de seguridad
- **Prioridad:** Alta
- **Fuente:** Stakeholder / Reunión MTG-002
- **Estado:** Propuesto
- **Fecha:** 2026-09-12

## Descripción

El sistema deberá presentar sugerencias de optimización y comparación de costos, pero no deberá aplicar automáticamente cambios sobre los recursos del usuario. La decisión de aceptar o rechazar una sugerencia será responsabilidad del usuario autorizado.

## Justificación

La solución es una herramienta de apoyo a la decisión. La aplicación automática podría afectar disponibilidad, rendimiento, costos o continuidad del servicio.

## Criterio de aceptación

El requisito se considera cumplido cuando una recomendación puede visualizarse y registrarse sin que el sistema invoque operaciones de modificación de recursos. Cualquier futura acción de cambio deberá permanecer fuera del alcance inicial y requerir un flujo de autorización explícita.

## Método de validación

Inspección y prueba de seguridad.

## Evidencia esperada

Prueba que muestre la generación de una recomendación sin cambios en Azure, logs de operaciones y revisión del código de integración.

## Dependencias

REQ-003 y ADR-002.

## Riesgos

La futura incorporación de acciones automáticas puede ampliar el alcance y requerir controles de aprobación, auditoría y reversión.

## Trazabilidad

- Problema: Riesgo de modificar recursos sin decisión del usuario.
- Objetivo: Apoyar decisiones seguras de optimización.
- Componente: Motor de recomendaciones y frontend.
- TEST-004: Pendiente.
- RESULT-004: Pendiente.
