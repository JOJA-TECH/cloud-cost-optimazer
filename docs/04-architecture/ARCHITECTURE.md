# Architecture — Documento de arquitectura

## 1. Contexto

El proyecto propone una plataforma para analizar recursos de bases de datos cloud, visualizar métricas y costos, y presentar sugerencias de optimización. La Reunión 2 definió una primera separación entre la persistencia operativa de la aplicación y la persistencia utilizada por el módulo de inteligencia artificial y la recolección de datos.

## 2. Objetivos arquitectónicos

- Separar datos operativos y datos de inteligencia artificial.
- Mantener el despliegue inicial local con PostgreSQL.
- Permitir que el frontend consulte recursos, métricas, estadísticas y sugerencias.
- Evitar cambios automáticos sobre los recursos del usuario.
- Facilitar la evolución posterior hacia servicios administrados o despliegue distribuido.

## 3. Restricciones

- La primera fase utiliza PostgreSQL local.
- Se mantienen dos bases de datos separadas.
- Las credenciales deben configurarse mediante variables de entorno o un mecanismo seguro.
- La aplicación solo presenta recomendaciones.
- La meta preliminar de respuesta menor a 30 segundos requiere un protocolo de medición antes de ser validada.

## 4. System Context

Los actores principales son el usuario final que consulta recursos y recomendaciones, el backend que procesa las solicitudes, las fuentes cloud que proporcionan métricas y costos, y el módulo de inteligencia artificial que analiza los datos recolectados.

Insertar `diagrams/context.svg` cuando el diagrama esté disponible.

## 5. Containers

La arquitectura inicial contempla:

1. Frontend web y landing page.
2. Backend de aplicación y autenticación.
3. Base PostgreSQL operativa para usuarios, sesiones, recursos y preferencias.
4. Servicio o módulo de recolección y análisis.
5. Base PostgreSQL de inteligencia artificial y recolección.
6. Conectores con fuentes de datos cloud.
7. Módulo de presentación de sugerencias.

Insertar `diagrams/containers.svg` cuando el diagrama esté disponible.

## 6. Components

Los componentes se organizarán inicialmente en gestión de usuarios, gestión de recursos, recolección de datos, normalización, análisis, generación de sugerencias y visualización. La aplicación no tendrá un componente de ejecución automática de cambios sobre infraestructura en la primera versión.

## 7. Deployment

El despliegue inicial será local. Cada integrante podrá ejecutar las bases PostgreSQL en su computador o servidor de desarrollo, utilizando una configuración documentada. La estrategia para un entorno compartido o administrado se definirá en una fase posterior.

Insertar `diagrams/deployment.svg` cuando el diagrama esté disponible.

## 8. Flujo de datos

1. El usuario inicia sesión y consulta sus recursos.
2. El backend solicita o recupera datos de configuración, costos, precios y métricas.
3. El módulo de recolección almacena los datos analíticos en la base de inteligencia artificial.
4. El backend consulta información operativa desde la base de aplicación.
5. El módulo de análisis produce estadísticas y sugerencias.
6. El frontend presenta métricas, costos, comparación y recomendación.
7. El usuario decide si considera la sugerencia. El sistema no aplica cambios automáticamente.

## 9. Seguridad

La primera versión debe evitar secretos en el código y restringir los permisos según la función. Las conexiones a PostgreSQL y a las fuentes cloud deben utilizar configuración externa. Los logs no deben incluir tokens, contraseñas ni cadenas de conexión completas.

## 10. Observabilidad

Se deben registrar estado de conexión, errores de consulta, tiempos de respuesta, fecha de actualización de datos y estado de las operaciones de análisis. La meta preliminar de respuesta menor a 30 segundos se registrará como requisito pendiente de protocolo.

## 11. Escalabilidad

La separación de bases permite evolucionar de forma independiente la persistencia operativa y la persistencia analítica. Antes de escalar, se deben definir sincronización, retención, consistencia, respaldos y mecanismos de intercambio de datos.

## 12. Decisiones arquitectónicas

- ADR-002 — Separación inicial de bases de datos PostgreSQL.

## 13. Limitaciones conocidas

- La infraestructura es local en la fase inicial.
- No se ha definido todavía el mecanismo de sincronización entre las bases.
- El flujo de análisis completo aún debe medirse con un protocolo reproducible.
- Los gráficos comparativos del frontend aún no están definidos.
- No se permite modificación automática de recursos.

## 14. Evolución prevista

La arquitectura podrá migrar posteriormente a bases administradas, un entorno compartido y servicios cloud. Esa evolución deberá revisarse mediante un nuevo ADR cuando se conozcan los requisitos de disponibilidad, seguridad, costo y operación.
