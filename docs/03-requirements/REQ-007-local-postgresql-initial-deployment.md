# REQ-007 — Despliegue local inicial con PostgreSQL

- **Tipo:** Restricción técnica
- **Prioridad:** Alta
- **Fuente:** Stakeholder / Reunión MTG-002
- **Estado:** Propuesto
- **Fecha:** 2026-09-12

## Descripción

La primera versión de las bases de datos deberá ejecutarse localmente utilizando PostgreSQL. Cada integrante podrá ejecutarla en su propio computador o servidor de desarrollo, siempre que utilice una configuración documentada.

## Justificación

El despliegue local permite avanzar en el diseño y validación de los esquemas antes de definir una infraestructura compartida o administrada.

## Criterio de aceptación

El requisito se considera cumplido cuando el equipo puede levantar las dos bases, ejecutar las migraciones y realizar las operaciones mínimas de lectura y escritura siguiendo instrucciones reproducibles.

## Método de validación

Demostración e inspección.

## Evidencia esperada

Archivo de configuración de ejemplo, migraciones, instrucciones de instalación y prueba de conexión.

## Dependencias

ADR-002 y REQ-002.

## Riesgos

Diferencias entre los entornos locales, versiones incompatibles y ausencia de un servidor compartido.

## Trazabilidad

- Problema: Necesidad de validar la persistencia sin desplegar infraestructura cloud adicional.
- Objetivo: Construir la base técnica inicial.
- Componente: Persistencia local.
- TEST-007: Pendiente.
- RESULT-007: Pendiente.
