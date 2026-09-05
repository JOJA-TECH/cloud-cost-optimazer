# Primera Reunión – Proyecto de Grado (Ing. I)

**Fecha:** 1 de septiembre de 2026

## Tema del proyecto

El proyecto consiste en el diseño y desarrollo de una plataforma inteligente para el análisis y optimización de costos de bases de datos en la nube (específicamente Azure SQL Database), mediante monitoreo, analítica predictiva y recomendaciones de capacidad.

- **Título de trabajo inicial:** "Diseño y desarrollo de una plataforma inteligente para el análisis de medición de costo de base de datos en la nube mediante monitoreo, analítica predictiva y recomendaciones de capacidad" (pendiente de precisar que es específicamente para Azure SQL Database).
- Se consideró también un título alternativo enfocado en "análisis y optimización de costes mediante monitoreo, analítica predictiva y recomendaciones de capacidad", pero se descartó por ser demasiado genérico (enfoque "workload"). Queda pendiente definir el nombre final.

## Puntos generales acordados

- El grupo revisó el documento con el contexto y recomendaciones generadas con ayuda de IA para orientar el proyecto.
- Se identificó como crítico definir bien la **pregunta de investigación** y la **hipótesis de trabajo**, ya que de eso depende justificar el proyecto ante el profesor; sin esto claro, corren el riesgo de que les devuelvan la primera entrega.
- **Consenso:** tomar como base, de momento, la pregunta de investigación y la hipótesis de trabajo propuestas por el análisis con IA, y validarlas con el profesor Aisner antes de avanzar más.
- Se acordó llevar un documento de **gobernanza/trazabilidad** del proyecto: un registro de todas las decisiones tomadas, con estado, fecha, decisores y documentos relacionados (requerimientos, experimentos, reuniones). Esto serviría como respaldo ante preguntas del profesor sobre por qué se tomó cada decisión.
- Se acordó usar herramientas de IA para generar los informes y avances a partir de la documentación y datos ya recolectados en el repositorio, en lugar de redactar todo manualmente.

## Plazos y entregas

- Primer avance: **14 de septiembre** (aprox. 2 semanas desde la reunión). Debe incluir: comprensión del problema e investigación inicial, planteamiento del problema, justificación, objetivo general y específicos, usuarios beneficiados, alcance preliminar y primeros antecedentes.
- Avance 4: **26 de octubre** – debe existir al menos un flujo principal funcionando de extremo a extremo (interpretado como MVP: login, registro de usuario, guardado en base de datos, y el flujo completo asociado).
- Entrega final: aproximadamente a **4 meses** (finales de noviembre). Debe incluir producto completo, documento del proyecto, código fuente y evidencias, y presentación preparada para la feria.
- Se acordó priorizar el trabajo fuerte en las últimas semanas antes de la fecha final, sin dejar todo para el final; se irán priorizando módulos según la entrega.

## Documentación y trabajo pendiente para el primer avance

- Completar información general: título del proyecto, resumen ejecutivo, antecedentes, planteamiento del problema, justificación.
- Definir alcance del proyecto y requerimientos preliminares.
- Definir usuarios, clientes y partes interesadas.
- Investigar y añadir referencias bibliográficas (antecedentes de software similar).
- Investigar el estándar **FOCUS** (especificación abierta para normalizar datos de costos entre proveedores cloud), ya que planean intentar usarlo por ser ampliamente adoptado.
- Definir KPIs medibles para demostrar que el sistema optimiza costos y que las recomendaciones son comprensibles; queda pendiente validar estos KPIs con alguien con experiencia en el tema (no se ha definido con quién).
- Investigar precios reales de Azure y métricas de Azure SQL.

## Arquitectura general propuesta

- **Frontend:** aplicación web (no chat, solo dashboard, reportes y recomendaciones). Se evaluaron **Next.js** y **Flutter**; queda pendiente decidir cuál usar (a definir con Anderson).
- **Backend:** un API Gateway escrito en **Go**, elegido por facilidad de despliegue (se compila a un binario único, fácil de dockerizar). El backend concentrará: autenticación, gestión de base de datos, orquestación y recomendaciones.
- **Módulo de Machine Learning:** separado del backend (dos "monolitos" independientes que se comunican entre sí), para que la ingesta de datos y el módulo de ML no se bloqueen mutuamente ni compitan por recursos.
- Arquitectura de microservicios simple: solo dos servicios (backend en Go y módulo de ML).
- Se define una **capa de abstracción** en el backend con un modelo de datos común (métricas, costo, configuración, recursos) y conectores/providers específicos por proveedor cloud. Aunque inicialmente solo se trabajará con Azure, se dejará el código preparado (mencionando explícitamente Azure/AWS/GCP) para justificar una arquitectura pensada a futuro.

### Infraestructura / almacenamiento
- Base de datos: **PostgreSQL**.
- Se evalúa usar **S3** y **CloudFront** (CDN/balanceador de carga) para resolver parte de la infraestructura de TI y generar tráfico demostrable para el profesor.
- Los laboratorios (generación de datos sintéticos) se manejarán con **Terraform**.
- Reportes/PDF: se acordó que los reportes **no se guardarán como archivo** en base de datos o storage; solo se guardará la información/resultado (respuesta del modelo generativo) en una tabla de la base de datos, y el PDF se generará en tiempo de ejecución cada vez que el usuario lo solicite (posiblemente vía plantilla HTML). Queda pendiente definir si se genera desde el backend o el frontend.
- Almacenamiento de archivos, modelos y reportes: no se usará un storage tipo bucket; se manejará a través de GitHub según el modelo propuesto.
- Cache: se considera Redis (o similar) solo para uso del lado del cliente (ej. sesiones), no como parte central del sistema.

### Autenticación
- Se descartó desarrollar autenticación propia "desde cero"; se evaluará usar un proveedor externo (Supabase u otro similar, o servicios de Azure/AWS).
- El usuario deberá proveer su propia **API key** del proveedor cloud (ej. Azure) para que la plataforma pueda monitorear sus recursos y generar recomendaciones. Esto se definió como parte del flujo de registro/configuración inicial.

## Módulo de Machine Learning (detalle)

- **Forecasting (pronóstico):** predice, por ejemplo, la probabilidad de aumento de tráfico en un rango de tiempo dado, usando datos históricos.
- Un modelo de lenguaje (LLM) se encargará de explicar en lenguaje natural los resultados del forecasting a partir de un prompt, en vez de tener explicaciones fijas predefinidas. Esto evita entrenar un modelo de lenguaje natural propio.
- Los reportes generados por el módulo de ML se guardarán en base de datos, con la idea de vectorizarlos.
- Se planteó guardar también la configuración/preferencias del usuario para permitir reentrenamiento personalizado en base a ellas.
- Reentrenamiento: el modelo se reentrenará periódicamente (por ejemplo, cada cierto tiempo o volumen de datos), con la condición de que solo se reentrene si hay una cantidad mínima de datos nuevos, para que sea viable y no consuma recursos innecesarios.
- Flujo de datos: se consumen datos desde APIs de Azure (costo estimado mensual, recursos consumidos, etc.) mediante el SDK de Azure para Go; se validan, transforman y pasan por un bus de eventos hacia la base de datos; luego el módulo de ML consume esas métricas.
- Componentes del motor de análisis (ML):
  - Forecasting de carga (predicción de carga futura).
  - Detección de anomalías (picos anómalos en los datos, que pueden indicar problemas de ingesta o de la base de datos).
  - Modelo de capacidad (reglas para decidir si se debe subir o bajar la capacidad de la base de datos según umbrales).
  - Optimizador de escenarios (cálculo de montos mínimos necesarios para mantener el sistema funcionando ante ciertos riesgos).
  - Cálculo de riesgo (probabilidad de que un cambio degrade el servicio monitoreado, en este caso Azure SQL).
  - Explicación generativa (LLM que lee la información y la explica al usuario).
  - Salidas del sistema: recomendación, ahorro estimado, riesgo asociado, explicación y evidencias (gráficos y datos).
- Métricas/datos de entrada mencionados: CPU/vCore, sesiones/conexiones, I/O, almacenamiento (tamaño y crecimiento), configuración actual, costos históricos y patrones de carga (picos, duración, frecuencia, percentiles).

## Chat / asistente conversacional

- Se discutió si incluir un chat tipo "copilot" para que el usuario pueda hacer preguntas sobre las recomendaciones.
- **Consenso:** si se incluye, se reutilizaría el mismo sistema/modelo que genera las explicaciones de las recomendaciones, en vez de construir un chat separado con memoria propia (se descartó el chat con memoria por no ser viable).
- Se planteó la opción de que el usuario pueda elegir su propio proveedor de modelo (local o externo) para no depender de un único proveedor pagado por el equipo. Queda como posible funcionalidad a definir (Jesús propuso esta alternativa; se dejó abierta para decidir más adelante).

## Funcionalidades descartadas o de baja prioridad

- **Notificaciones** (push, correo, etc.): se descartaron por complejidad, considerando que es una aplicación web (tendría más sentido en apps móviles o de escritorio). Se dejó como posible investigación futura, ya que muchas apps web sí permiten notificaciones.
- Simulaciones y reportes (como funcionalidades del backend) pueden quedar en un segundo plano dentro de los 4 meses, ya que dependen del módulo de ML.
- Detección de anomalías, optimización de escenarios y aplicaciones adicionales del ML también pueden quedar en segundo plano para el MVP.

## Auditoría y trazabilidad (para el usuario final del producto)

- Se propuso dar al usuario final la opción de ver el historial de todas las acciones/recomendaciones que ha recibido, para que pueda entender el rendimiento del servicio que está monitoreando.

## Roles y división de trabajo

- El equipo no tiene especializaciones fuertes definidas, por lo que se decidió repartir el trabajo de forma que todos tengan carga similar y aprendan de partes distintas del sistema.
- **Datos y Machine Learning:** a cargo de Jesús (incluye también los laboratorios con Terraform).
- **CI/CD y QA:** a cargo de Jorge.
- **Backend y Frontend:** a cargo de Omar y Anderson (Anderson también apoyará en backend); no se busca encapsular a nadie en un solo rol, sino que puedan rotar y aprender de otras partes.
- **Base de datos:** aún no se definió quién la liderará; Jorge se ofreció a encargarse con ayuda de Jesús. Se aclaró que antes de modelar la base de datos hay que definir primero el modelo de datos (cómo se van a recibir los datos).
- Omar asumirá temporalmente el rol de líder de proyecto para coordinar la organización general (documentación, repositorio, etc.).

## Flujo de trabajo técnico acordado

- **Metodología:** se intentará aplicar **Scrum**. Omar se encargará de crear los backlogs.
- **Repositorio:** GitHub, con posibilidad de agregar al profesor Aisner si él lo desea.
- **Control de versiones:** trabajo en ramas independientes; cada commit/pull request debe ser aprobado por al menos **2 integrantes** (excluyendo al autor del PR), para asegurar que todos revisen el trabajo de los demás y puedan responder preguntas del profesor.
- Reuniones semanales para reportar avances y definir tareas de la semana.

## Reuniones y comunicación

- Se acordó reunirse todos los **domingos a las 10:00 a.m.** como una especie de retrospectiva semanal.
- También se aprovecharán 2 horas libres antes de la clase con el profesor Monterrosa entre semana para hablar de temas puntuales.
- El repositorio contendrá también las grabaciones/transcripciones y reportes de las reuniones.
- Se descartó usar Slack u otras herramientas tipo Atlassian por considerarlas poco prácticas para este caso; no se definió una alternativa concreta más allá de las reuniones grabadas y el repositorio.

## MVP (avance del 26 de octubre)

- Interpretación acordada de MVP: al menos un flujo completo de extremo a extremo, por ejemplo, que el usuario pueda registrarse, iniciar sesión y que esa información se guarde correctamente en la base de datos.
- Módulos objetivo para los primeros 4 meses:
  - **Backend:** autenticación, gestión de base de datos, orquestación y recomendaciones (estas últimas puede ser una versión preliminar dependiente del ML). Simulaciones y reportes pueden quedar en segundo plano.
  - **ML:** forecasting, modelo de capacidad y cálculo de riesgo son prioritarios. Detección de anomalías, optimización de escenarios y aplicaciones adicionales pueden quedar en segundo plano.
  - Se apunta a tener al menos un 50-60% del backend avanzado, junto con una versión inicial del frontend (aunque no esté pulida) y los datasets generados desde los laboratorios en Terraform.

## Preguntas / temas pendientes por resolver con el profesor o el equipo

- Validar con el profesor la pregunta de investigación y la hipótesis de trabajo propuestas.
- Confirmar el horario y modalidad de las asesorías con el profesor.
- Decidir entre Next.js y Flutter para el frontend.
- Definir si el chat/asistente conversacional se implementará, y con qué proveedor de modelo (local vs. externo vs. configurable por el usuario).
- Definir si notificaciones se implementarán a futuro.
- Definir dónde se generará el PDF de los reportes (backend o frontend).
- Definir con quién validar los KPIs propuestos.
- Confirmar el nombre final del proyecto.
- Investigar antecedentes de software similar y el estándar FOCUS.
- Investigar precios reales de Azure y métricas de Azure SQL.
- Definir el modelo de datos antes de diseñar la base de datos.
