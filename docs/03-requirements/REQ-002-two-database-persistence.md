# REQ-002 — Separación de persistencia operativa y de inteligencia artificial

- **Tipo:** Arquitectura / Restricción
- **Prioridad:** Alta
- **Fuente:** Stakeholder / Reunión MTG-002
- **Estado:** Propuesto
- **Fecha:** 2026-09-12

## Descripción

El sistema deberá utilizar inicialmente dos bases de datos PostgreSQL separadas. La primera almacenará la información de comunicación entre el usuario y la aplicación. La segunda almacenará datos recolectados para el módulo de inteligencia artificial y futuros entrenamientos.

## Justificación

La separación permite distinguir los datos operativos de los datos analíticos y de entrenamiento. También facilita la evolución independiente y la definición de permisos diferenciados.

## Criterio de aceptación

El requisito se considera cumplido cuando existen dos conexiones configurables, dos esquemas iniciales documentados y una prueba que confirme la escritura y lectura independiente en cada base.

## Método de validación

Inspección y demostración.

## Evidencia esperada

Esquemas, migraciones, archivo de configuración sin secretos, prueba de conexión y registro de una operación en cada base.

## Dependencias

ADR-002 y REQ-007.

## Riesgos

Duplicación, inconsistencia y sincronización no definida entre bases.

## Trazabilidad

- Problema: Separación de datos operativos y de inteligencia artificial.
- Objetivo: Construir una plataforma mantenible y trazable.
- Componente: Persistencia.
- TEST-002: Pendiente.
- RESULT-002: Pendiente.
