# Data Dictionary — DATA-001

| Campo / grupo | Tipo | Descripción | Unidad | Permitidos / rango | Nullable | Fuente |
|---|---|---|---|---|---|---|
| Identificador de recurso | Texto | Identifica servidor o base analizada | No aplica | Ruta de recurso Azure | Sí según fuente | Configuración y métricas |
| Región | Texto | Región del recurso o del precio | No aplica | Nombre de región Azure | No en configuración | ARM y precios |
| Modalidad | Texto | Modelo de operación del recurso | No aplica | Provisioned, serverless u otra modalidad registrada | Sí | Configuración y precios |
| Tier | Texto | Nivel de servicio | No aplica | General Purpose, Business Critical, etc. | Sí | Configuración y precios |
| Capacidad | Numérico | Capacidad configurada o límite | vCore | Mayor o igual que 0 | Sí | Configuración |
| Fecha/hora | Fecha-hora | Instante de uso, métrica o registro | UTC | ISO 8601 recomendado | No en series temporales | Todas las fuentes temporales |
| Promedio | Numérico | Valor promedio de una métrica agregada | Porcentaje, cantidad o unidad de fuente | Mayor o igual que 0 cuando aplica | Sí | Monitor y logs |
| Mínimo | Numérico | Valor mínimo agregado | Unidad de fuente | Depende de la métrica | Sí | Monitor y logs |
| Máximo | Numérico | Valor máximo agregado | Unidad de fuente | Mayor o igual que mínimo cuando ambos existen | Sí | Monitor y logs |
| Nombre de métrica | Texto | Métrica observada | No aplica | Catálogo devuelto por Azure | No en registros de métrica | Monitor y logs |
| Unidad | Texto | Unidad de la métrica o precio | No aplica | Percent, Count, 1 Hour, 1 GB/Month, etc. | Sí | Monitor y precios |
| Costo antes de impuestos | Numérico | Costo reportado por Cost Management | Moneda de la fila | Mayor o igual que 0 | No en filas de costo | Cost Management |
| Moneda | Texto | Moneda del costo o precio | No aplica | Código monetario | No en costos | Cost Management y precios |
| Fecha de uso | Fecha | Día de uso facturado | Día calendario | YYYYMMDD en respuesta original | No en costos | Cost Management |
| Precio minorista | Numérico | Precio de referencia del catálogo | Unidad del precio | Mayor o igual que 0 | No en elementos de precio | Retail Prices |
| Tipo de precio | Texto | Modalidad comercial del precio | No aplica | Consumption o Reservation | No en precios | Retail Prices |
| Fecha de vigencia | Fecha-hora | Inicio de vigencia del precio | UTC | ISO 8601 | Sí | Retail Prices |
| Consulta / operación | Texto | Consulta o operación registrada | No aplica | Texto o identificador | Sí | Query Store y carga |
| Duración | Numérico | Duración de ejecución | Milisegundos según fuente | Mayor o igual que 0 | Sí | Query Store |
| Tiempo de CPU | Numérico | CPU consumida por ejecución | Microsegundos según fuente | Mayor o igual que 0 | Sí | Query Store |
| Lecturas lógicas | Numérico | Lecturas lógicas de la consulta | Cantidad | Mayor o igual que 0 | Sí | Query Store |
| Categoría de espera | Texto | Categoría de espera observada | No aplica | Texto de la fuente | Sí | Query Store |
| Evento | Texto | Nombre sintético del evento | No aplica | Texto | Sí | Tabla de carga |
| Carga | Entero | Intensidad o valor sintético asociado al evento | Unidad experimental pendiente | Entero de la carga | Sí | Tabla de carga |
| Descripción | Texto | Descripción sintética del registro | No aplica | Texto | Sí | Tabla de carga |

## Reglas de transformación

Las fechas deben normalizarse a UTC. Las respuestas tabulares deben transformarse desde arreglos de columnas y arreglos de filas a objetos con nombres de campo. Los precios deben separarse por región, modalidad, tier, familia, unidad, tipo de precio y fecha de vigencia. Las métricas deben conservar la granularidad y distinguir valor observado de valor agregado.

## Reglas de validación

Un registro temporal debe tener una fecha válida. Un valor mínimo no debe ser mayor que el máximo cuando ambos existan. Un precio no debe asociarse a una configuración si la región, modalidad, tier, unidad o tipo de precio no coinciden. Un costo facturado igual a cero debe conservarse, pero debe marcarse como potencialmente afectado por crédito o nivel gratuito. Ningún cálculo de p95 o p99 debe realizarse exclusivamente a partir de promedio, mínimo y máximo agregados.
