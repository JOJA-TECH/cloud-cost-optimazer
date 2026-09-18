# Reunión 2 – Proyecto de Grado (Ing. I)

**Fecha:** 12 de septiembre de 2026

## Propósito de la reunión

Definir tareas concretas para el primer avance (entrega del 14 de septiembre) y repartir responsabilidades iniciales entre los integrantes, además de acordar cómo comunicarse con el profesor antes de la entrega.

## Consenso sobre la entrega al profesor

- Se acordó entregarle al profesor una **versión "beta"** de los documentos, con lo que ya tienen avanzado (llegando hasta el punto "usuarios, clientes y partes interesadas"), para que dé su opinión antes de la fecha de entrega formal.
- La idea es decirle al profesor que van por buen camino con el enfoque actual y preguntarle qué cambiaría o mejoraría, en vez de esperar hasta el último momento.
- Se intentará contactar al profesor un día antes de la entrega (mencionan el jueves) para ver si tiene tiempo de revisar avances con ellos.

## Cronograma / metodología

- Se ratificó el día de sprint/retrospectiva: **domingos a las 10:00 a.m.**
- Queda pendiente terminar de asignar tareas específicas a cada integrante y organizarlas formalmente.

## División de tareas para esta semana

### Jorge
- Encargado de diseñar el **esquema inicial de la base de datos**.
- Se aclaró que el proyecto requiere **dos bases de datos separadas**:
  1. Una para la comunicación entre el usuario y la aplicación (parte "usuario-servidor").
  2. Otra para el módulo de inteligencia artificial, usada para almacenar y recolectar datos (incluyendo la estructura de negocio que el usuario envía) con miras a futuros entrenamientos del modelo.
- Se acordó desplegar la base de datos de forma local por ahora, usando **PostgreSQL**; cada quien decide si la corre en su propio computador o en un servidor propio.
- Jorge tomó esto como su primera tarea de la semana.

### Jesús
- A cargo de la parte de inteligencia artificial y del servidor.
- Compartirá con el equipo un resumen/documento (mencionado como "overly") sobre cómo organizar las secciones del documento del proyecto.
- Debe enviarle a Anderson los datos (ejemplo de JSON con métricas) que sirven de referencia para diseñar la visualización del frontend.
- Completará la parte de requerimientos funcionales y no funcionales del documento.

### Anderson
- A cargo del **frontend**: diseño inicial de la interfaz de usuario y de una **landing page** tipo página de presentación/venta del proyecto (estilo profesional, mostrando qué ofrece el producto), con un botón de inicio de sesión que lleve a la aplicación.
- Se le pidió enfocarse en la **visualización de gráficos** como parte primordial del frontend, ya que aún no se ha definido con exactitud qué tipo de gráficos comparativos se usarán (se barajan opciones como gráfico de barras, de líneas y de torta).
- Debe basar el diseño de la vista de datos en las métricas ya identificadas por Jesús (ver sección de datos más abajo).
- Anderson pidió que las tareas se le asignen de forma explícita ("pónganme tarea, haz esto, haz lo otro").

## Datos y métricas que debe mostrar la aplicación (para orientar el frontend)

- En algún lugar de la interfaz se deben mostrar los **recursos actuales del usuario** (ej. distintos servidores que tiene).
- Al seleccionar un recurso, se debe mostrar una vista con las **estadísticas y métricas** que entrega la IA, junto con las sugerencias correspondientes.
- Métricas mencionadas: precio/costo actual, núcleos, lecturas, almacenamiento, red, y otros datos relacionados con el consumo del recurso.
- Se debe mostrar comparativamente el costo actual del usuario frente al costo estimado si aplicara las recomendaciones sugeridas por la IA (por ejemplo, cuánto se reduciría el costo de almacenamiento con cierta configuración).
- **Importante:** la aplicación solo sugiere; la decisión de aplicar o no el cambio recomendado es responsabilidad del usuario, el sistema no ejecuta cambios automáticamente.
- Esta información de referencia (ejemplos de datos/JSON y comparativa de proveedores) se encuentra en documentos compartidos por Omar, en un archivo relacionado con la investigación de proveedores.

## Documento del proyecto (avance 1)

- Se recordó que para esta entrega el profesor pide específicamente: **planteamiento del problema, justificación, objetivos, usuarios beneficiados, alcance original y primeros antecedentes**.
- Gran parte de esta información ya está redactada en el documento que compartió Omar; falta revisarla, pulirla y completar la parte de usuarios y partes interesadas.
- **Consenso:** priorizar y poner especial cuidado en estos puntos exigidos para la primera entrega, dejando que el resto de secciones del documento (arquitectura, tecnologías, metodología, solución, seguridad) se llenen más rápido después, usando lo ya definido como marco base para desarrollar los siguientes puntos del documento completo.

## Otros temas pendientes

- Definir con más detalle cómo se organizarán las secciones del documento completo (Jesús se comprometió a compartir una referencia/plantilla).
- Definir exactamente qué tipo(s) de gráfico(s) se usarán para comparar métricas de rendimiento.
- Anderson quedó de tener un primer diseño ("full pass") de la landing page para el día siguiente.
- Se mencionó una fecha de entrega cercana en la que el proyecto debe quedar funcionando al 100%, generando resultados en menos de 30 segundos (referido al flujo de análisis/recomendación), aunque no se detalló completamente esta meta.
