# Análisis de la base de datos `cco_data`

## 1. Resumen ejecutivo

`cco_data` es la base de datos analítica del sistema **Cloud Cost Optimizer**. Su responsabilidad es adquirir y conservar datos de recursos cloud, convertir telemetría en variables analíticas, entrenar y evaluar modelos, simular configuraciones y producir recomendaciones de optimización.

La base está organizada como un pipeline de datos:

```text
Catálogos + recursos
        ↓
Telemetría cruda y costos
        ↓
Features periódicas
        ↓
Datasets y modelos ML
        ↓
Forecasts
        ↓
Escenarios y candidatos
        ↓
Simulaciones, riesgo y evidencia
        ↓
Recomendaciones
```

La base no representa usuarios, organizaciones, membresías, permisos de producto ni interacciones de usuario. Esas responsabilidades pertenecen a `cco_app`. La integración entre ambas bases utiliza UUIDs públicos estables y vistas del schema `contract`, sin claves foráneas PostgreSQL entre bases.

El diseño prioriza cinco propiedades:

1. **Trazabilidad:** una recomendación puede relacionarse con su escenario, simulación, evidencia, riesgo, forecast, modelo, dataset y telemetría.
2. **Reproducibilidad:** datasets, corridas de entrenamiento, versiones de modelos, parámetros y hashes quedan registrados.
3. **Separación de responsabilidades:** los datos crudos, los hechos canónicos, las features, los modelos y las decisiones viven en schemas distintos.
4. **Auditoría:** las configuraciones, cargos, ejecuciones del collector y recomendaciones se conservan con sus referencias temporales.
5. **Contrato externo seguro:** la aplicación consume UUIDs y vistas públicas, no las claves identity internas.

---

## 2. Requisitos técnicos de ejecución

El DDL está diseñado para **PostgreSQL 14 o superior** y requiere la extensión `pgcrypto`, usada para generar UUIDs con `gen_random_uuid()`.

La base debe ejecutarse como una base independiente llamada `cco_data`:

```bash
createdb cco_data
psql -d cco_data -v ON_ERROR_STOP=1 -f cco_data_schema.sql
```

El script crea schemas, tablas, vistas, funciones, restricciones, índices y datos iniciales de catálogos. La creación de roles, la asignación de permisos y la administración de particiones deben realizarse con un usuario administrador o migrator.

---

## 3. Organización por schemas

| Schema | Responsabilidad principal | Naturaleza de los datos |
|---|---|---|
| `catalog` | Proveedores, regiones, métricas, workloads y SKUs | Referencia y dominios controlados |
| `core` | Recursos, configuración, costos y workloads | Modelo canónico |
| `raw` | Telemetría, actividad y respuestas de proveedores | Crudo, append-only y de alto volumen |
| `feat` | Definiciones y snapshots de features | Datos derivados para analítica y ML |
| `ml` | Experimentos, datasets, entrenamientos, modelos y forecasts | Lineage y ciclo de vida de modelos |
| `opt` | Escenarios, simulaciones, riesgo y recomendaciones | Decisiones analíticas |
| `contract` | Vistas y versión del contrato externo | Superficie read-only para `cco_app` |

La separación evita mezclar datos operativos de aplicación con datos de observabilidad y machine learning. También permite aplicar permisos distintos a collectors, servicios ML, motor de optimización, backend y archivado.

---

## 4. Schema `catalog`: dominios y espacio de búsqueda

### 4.1 Proveedores y regiones

`catalog.cloud_provider` contiene los proveedores cloud habilitados. Actualmente el catálogo inicial incluye Azure, pero el campo `provider_code` permite ampliar el modelo a otros proveedores.

`catalog.region` identifica regiones por proveedor mediante una clave primaria compuesta:

```text
(provider_code, region_code)
```

Esta relación es importante porque impide asociar accidentalmente una región con un proveedor incorrecto. Las tablas de recursos, precios y costos utilizan la misma combinación para validar la pertenencia de la región.

### 4.2 Catálogo de métricas

`catalog.metric_catalog` es el contrato formal de ingesta. Cada métrica tiene:

- código canónico;
- nombre visible;
- unidad;
- fuente;
- granularidad esperada;
- indicador de si participa en el feature store;
- descripción.

Entre las métricas definidas se encuentran CPU, almacenamiento, sesiones, workers, deadlocks, I/O, log write, memoria, CPU de instancia y duración de queries.

El collector no debería aceptar una métrica no registrada aquí. Esto evita que el pipeline dependa de nombres implícitos o inconsistentes entre APIs.

### 4.3 Clases de workload

`catalog.workload_class` clasifica las pruebas experimentales: `low_stable`, `high_stable`, `periodic`, `bursty`, `increasing`, `cpu_intensive`, `io_intensive` y `mixed`.

La clase de workload convierte una corrida de carga en un experimento identificable. Permite distinguir, por ejemplo, una reducción válida para una carga estable de una reducción peligrosa para un workload bursty.

### 4.4 Catálogo de SKUs

`catalog.sql_sku` representa el espacio finito de configuraciones que el optimizador puede evaluar. Incluye:

- nombre de SKU;
- nombre ARM;
- tier;
- familia;
- modelo provisioned o serverless;
- capacidad en vCores;
- capacidad mínima para serverless;
- tamaño máximo;
- pertenencia al alcance de Release 1.0.

Este catálogo limita la generación de candidatos a configuraciones conocidas y auditables. El optimizador no debería generar una configuración que no exista en este catálogo.

---

## 5. Schema `core`: hechos canónicos

### 5.1 `core.resource`

Es el maestro de recursos monitoreados. Actualmente está modelado para bases de datos Azure SQL y conserva:

- `resource_id`: clave interna identity usada por las FK locales;
- `resource_uid`: UUID estable de integración;
- proveedor, suscripción y resource group;
- servidor y base de datos;
- `azure_resource_id`: ruta ARM completa y única;
- región;
- tipo o características del recurso;
- estado de ciclo de vida;
- fechas de registro y desactivación.

La ruta ARM es la identidad natural del recurso dentro de Azure. El UUID es la identidad pública que puede utilizar `cco_app`.

El estado de ciclo de vida puede ser `active`, `paused`, `deleted` o `archived`. La separación entre la clave interna y el UUID público permite reorganizar internamente las claves sin romper referencias externas.

### 5.2 `core.resource_config_snapshot`

Conserva snapshots históricos de la configuración observada en ARM. Cada snapshot puede registrar:

- SKU;
- tier y familia;
- capacidad y límites;
- modelo de cómputo;
- tamaño máximo;
- configuración de auto-pause;
- estado operativo;
- free limit;
- payload original del proveedor;
- momento de observación.

El modelo es temporal: no se actualiza una única fila para representar el estado actual, sino que se añade un snapshot. Esto permite reconstruir la capacidad vigente cuando se generó una feature o se ejecutó un workload.

### 5.3 `core.price_catalog`

Es el catálogo de precios de lista. Representa el precio utilizado para simular escenarios, no necesariamente el cargo final facturado.

Registra:

- proveedor y región;
- meter y producto;
- SKU;
- tipo de precio;
- unidad de medida;
- precio unitario y moneda;
- mínimo de tier;
- vigencia;
- payload y momento de extracción.

La vigencia por fecha permite calcular simulaciones con el precio correspondiente al periodo analizado.

### 5.4 `core.cost_daily`

Es el hecho canónico de costo real diario, inspirado en un modelo FOCUS-lite. Puede representar cargos a nivel de suscripción, resource group o recurso.

Incluye:

- recurso cuando existe mapeo;
- alcance del cargo;
- servicio y región;
- fecha de uso;
- identificador del cargo de origen;
- meter;
- costo antes de impuestos;
- cantidad y unidad consumida;
- precio de lista;
- modelo de pricing;
- categoría del cargo;
- fuente y fecha de colección.

El `source_charge_id` permite reintentos idempotentes y evita duplicar el mismo cargo. La unicidad parcial solo se aplica cuando el proveedor entrega ese identificador, lo que permite conservar cargos distintos del mismo servicio y fecha.

### 5.5 Workloads experimentales

`core.workload_run` representa una sesión controlada de carga sobre un recurso. Registra su clase, configuración, inicio, fin, duración y parámetros del generador.

`core.workload_cycle` descompone la corrida en ciclos y conserva:

- número del ciclo;
- intensidad objetivo;
- operaciones;
- progreso;
- inserts, updates, deletes y selects.

Estas tablas son importantes porque permiten comparar la intensidad aplicada con la telemetría observada. Así se puede construir ground truth experimental en lugar de depender únicamente de métricas agregadas del proveedor.

---

## 6. Schema `raw`: ingesta y procedencia

El schema `raw` es el espejo de los datos recibidos desde los proveedores. Debe operarse como append-only: el collector inserta y consulta, pero no corrige históricamente mediante `UPDATE` o `DELETE`.

### 6.1 Auditoría de ingesta

`raw.collector_run` registra cada ejecución del collector:

- fuente consultada;
- recurso o scope;
- inicio y fin;
- estado `ok`, `partial` o `error`;
- cantidad de filas ingeridas;
- mensaje de error;
- parámetros de solicitud;
- payload de respuesta opcional.

Esto proporciona trazabilidad de cada lectura al proveedor y permite investigar huecos, errores o reintentos.

### 6.2 Métricas agregadas

`raw.metric_reading` conserva lecturas de Azure Monitor con:

- recurso;
- métrica;
- timestamp;
- intervalo;
- promedio;
- mínimo;
- máximo;
- total;
- conteo;
- fecha de colección.

La tabla está particionada por rango mensual sobre `timestamp_utc`, con una partición `DEFAULT`. El índice BRIN sobre la fecha es adecuado para series temporales grandes y físicamente ordenadas.

Esta tabla es válida para promedios, máximos, tendencias y otros cálculos agregados. No contiene la distribución completa de observaciones y, por sí sola, no permite obtener un p95 o p99 válido de las muestras originales.

### 6.3 Muestras individuales y percentiles

`raw.metric_sample` almacena observaciones individuales para estadísticas de distribución. Contiene:

- identificador de muestra;
- recurso y métrica;
- instante de observación;
- valor;
- fuente;
- identificador de muestra de origen cuando exista;
- fecha de colección.

Las fuentes permitidas son `azure_monitor_sample`, `database_watcher_sample`, `query_store_execution` y `workload_observation`.

Esta tabla es la fuente correcta para calcular percentiles exactos. El pipeline debe conservar la procedencia y evitar confundir una secuencia de promedios de un minuto con las observaciones originales.

### 6.4 `raw.db_resource_stats`

Conserva estadísticas internas de Azure SQL, como:

- CPU promedio;
- data I/O promedio;
- log write promedio;
- memoria promedio;
- CPU de instancia;
- límites de DTU y CPU.

Está particionada mensualmente. Es especialmente útil para identificar cuellos de I/O que no se ven en la CPU agregada.

### 6.5 Query Store y sesiones

Las tablas de Query Store conservan consultas, estadísticas de runtime y esperas. Permiten investigar calidad y latencia a nivel de query, no solo a nivel de recurso.

`raw.db_session_snapshot` representa instantáneas de sesiones activas con login, host, programa, estado y última petición. Sirve para caracterizar concurrencia y contexto operativo.

### 6.6 Activity Log

`raw.activity_log` conserva operaciones del plano de control, incluyendo operación, resultado, firma, IP, correlación, duración y propiedades.

Su objetivo es correlacionar cambios de control —por ejemplo, un escalado— con cambios posteriores en telemetría o costo. Está particionada por mes y puede conservar payloads JSONB de auditoría.

### 6.7 Azure Advisor

`raw.advisor_recommendation` conserva recomendaciones de Azure Advisor como baseline externo. No se considera ground truth universal. Puede compararse con las recomendaciones propias en `opt.advisor_comparison`.

---

## 7. Schema `feat`: feature store

### 7.1 Diccionario de features

`feat.feature_definition` documenta cada feature con:

- código;
- grupo;
- nombre visible;
- descripción;
- unidad;
- fórmula;
- tablas fuente;
- tipos de periodo permitidos.

Esto evita que el feature store sea una caja negra. Una persona puede conocer qué significa una columna y de dónde proviene.

Las features cubren:

- CPU promedio, p50, p95, p99, máximo y pendiente;
- I/O y log write;
- sesiones y workers;
- almacenamiento y crecimiento;
- temporalidad y hora pico;
- intensidad de workload;
- deadlocks;
- latencia p95.

### 7.2 `feat.percentile_statistic`

Esta tabla es el contrato estadístico de percentiles. Registra por recurso, ventana y métrica:

- percentil solicitado;
- valor;
- método;
- cantidad de observaciones;
- granularidad de la fuente;
- momento de cálculo.

Los métodos permitidos son:

| Método | Interpretación |
|---|---|
| `exact_sample` | Percentil calculado con observaciones individuales |
| `weighted_histogram` | Estimación a partir de un histograma con pesos conservados |
| `not_available` | No existe información suficiente |

Cuando solo existen `avg`, `min` y `max`, el resultado correcto es `method = 'not_available'`, `value = NULL` y `observation_count = 0`. No se debe inventar un p95 interpolando agregados.

La vista `contract.v_percentile_statistic` publica también este método y procedencia para que un consumidor externo pueda saber si un valor es exacto, estimado o no disponible.

### 7.3 `feat.feature_snapshot`

Es una tabla ancha con una fila por:

```text
resource_id + period_type + period_start_utc
```

Los periodos soportados son `15m`, `1h` y `1d`. Incluye:

- contexto de SKU y capacidad;
- CPU;
- I/O;
- concurrencia;
- almacenamiento;
- temporalidad;
- intensidad de workload;
- deadlocks;
- latencia.

Está particionada mensualmente. La estructura ancha favorece el entrenamiento y evita que cada feature tenga que reconstruirse mediante un modelo EAV.

Las columnas de percentiles pueden quedar en `NULL` cuando no existe evidencia estadística válida. La procedencia no debe perderse: el método y conteo quedan registrados en `feat.percentile_statistic`.

### 7.4 Vista de entrenamiento

`feat.v_training_base` une las features de la hora `h` con labels de la hora `h+1`. Esto evita utilizar el futuro dentro de las variables de entrada.

La vista expone objetivos como:

- CPU promedio de la siguiente hora;
- CPU p95 de la siguiente hora;
- intensidad futura;
- almacenamiento futuro.

Esta vista es una base preparada para exportación y construcción de datasets temporales.

---

## 8. Schema `ml`: reproducibilidad y forecasting

### 8.1 Experimentos

`ml.experiment` agrupa una investigación o línea de trabajo. Su tipo puede ser forecasting, right-sizing, risk o score. También conserva configuración y fechas.

### 8.2 Datasets versionados

`ml.training_dataset` registra la definición exacta de un dataset:

- nombre y versión;
- alcance de recursos;
- ventana temporal;
- tipo de periodo;
- lista explícita de features;
- columna objetivo;
- cantidad de filas;
- estrategia de división;
- hash del contenido exportado.

El hash y la versión permiten reconstruir qué datos originaron un modelo.

### 8.3 Corridas de entrenamiento

`ml.training_run` relaciona un dataset con:

- algoritmo;
- hiperparámetros;
- estado;
- inicio y fin;
- servicio que entrenó;
- URI del artefacto;
- notas.

Se pueden registrar baselines explicables como `naive_seasonal`, `arima`, `ets` y `rule_baseline`, además de algoritmos como random forest y XGBoost.

### 8.4 Registro de modelos

`ml.model_version` implementa el ciclo de vida del modelo:

```text
candidate → champion → archived/retired
```

Cada versión tiene nombre, versión, framework, parámetros, métricas y fecha de promoción. El modelo productivo esperado es el que se encuentra en estado `champion`.

### 8.5 Forecasts

`ml.forecast` conserva predicciones para:

- `cpu_avg`;
- `workload_intensity`;
- `storage_used_bytes`.

Cada forecast incluye valor predicho, límites inferior y superior, alpha, horizonte, unidad temporal y momento de generación.

El costo no se pronostica directamente en esta tabla. El diseño separa el forecasting de comportamiento y la simulación económica basada en `core.price_catalog`.

El UUID `forecast_uid` es el identificador de integración que se publica mediante `contract.v_forecast`.

### 8.6 Evaluación

`ml.forecast_evaluation` registra MAE, RMSE, MAPE, sMAPE, cobertura de intervalos y pinball loss por modelo, recurso, métrica, ventana y horizonte.

Esto permite evaluar tanto el error puntual como la calidad de los intervalos predictivos. La vista `ml.v_model_leaderboard` facilita comparar versiones y algoritmos.

---

## 9. Schema `opt`: optimización y decisiones

### 9.1 Escenarios

`opt.scenario` representa una solicitud de optimización. Contiene:

- recurso;
- UUID externo;
- horizonte de días;
- pesos del objetivo;
- restricciones JSONB;
- estado;
- creador y fechas.

Las restricciones pueden expresar límites de aumento de latencia p95, máximo de violaciones SLA u otras reglas del problema.

### 9.2 Configuraciones candidatas

`opt.candidate_configuration` contiene las alternativas que el optimizador evaluará. Cada candidato puede especificar SKU, modelo, capacidad, método generador, costo mensual estimado y base de precio.

La referencia a `catalog.sql_sku` garantiza que los candidatos pertenecen a un espacio permitido.

### 9.3 Simulaciones

`opt.scenario_simulation` evalúa cada candidato antes de recomendarlo. Conserva:

- versión del modelo usada;
- ventana de forecast;
- CPU, workload e indicadores de latencia previstos;
- costo mensual;
- ahorro estimado;
- factibilidad;
- restricciones violadas.

La simulación es el punto donde se combinan forecasts, precios, restricciones y riesgo de rendimiento.

### 9.4 Pruebas experimentales y ground truth

`opt.experiment_trial` representa la ejecución real de un workload sobre una configuración. Puede guardar costo medido, CPU, latencia, throughput, violaciones SLA y si el resultado se considera ground truth.

Esta tabla permite validar si una simulación fue acertada y medir fenómenos como el false optimization rate.

### 9.5 Recomendaciones

`opt.recommendation` es la salida final versionada. Incluye:

- configuración actual;
- configuración propuesta;
- ahorro estimado;
- confianza;
- estado;
- racional legible;
- generador;
- decisión humana;
- fecha de aplicación.

Los estados posibles son `proposed`, `under_review`, `accepted`, `rejected`, `applied` y `dismissed`.

La recomendación es inmutable conceptualmente: una nueva versión utiliza `supersedes_id` para indicar qué recomendación reemplaza. La unicidad de `(scenario_id, version)` impide duplicar versiones dentro del escenario.

### 9.6 Evidencia

`opt.recommendation_evidence` almacena evidencia cuantitativa que soporta una recomendación, por ejemplo CPU p95, p99, I/O, tendencia, horas pico o crecimiento de almacenamiento.

El LLM, si se usa, debe narrar esta evidencia y no generar datos, configuraciones ni métricas por su cuenta.

### 9.7 Riesgo

`opt.risk_assessment` persiste los componentes del riesgo:

- pico de uso;
- variabilidad;
- tendencia;
- headroom;
- score;
- nivel;
- factores desglosados.

La columna `model_version` permite cambiar de una regla como `rule_v1` a un modelo posterior sin cambiar la estructura del resto de la base.

### 9.8 Database Efficiency Score

`opt.efficiency_score` registra un score compuesto por capacidad, costo y performance. Los pesos deben sumar aproximadamente uno y se identifican mediante `weights_version`.

El versionado de pesos permite análisis de sensibilidad y evita que cambios de ponderación hagan incomparables los resultados sin dejar registro.

### 9.9 Comparación con Azure Advisor

`opt.advisor_comparison` relaciona recomendaciones propias con recomendaciones de Azure Advisor y clasifica la relación como `agree`, `disagree`, `partial` o `not_comparable`.

Advisor sirve como referencia externa, no como verdad absoluta ni como sustituto del ground truth experimental.

### 9.10 Vista operativa

`opt.v_recommendation_latest` presenta la recomendación más reciente no rechazada ni descartada por recurso. Su objetivo es que el dashboard consuma una regla ya resuelta y no replique esa lógica en frontend.

---

## 10. Contrato externo con `cco_app`

El contrato activo se identifica como:

```text
cco_data.external v1
```

La tabla `contract.contract_version` conserva el productor, consumidor, estado, fecha y notas de la versión publicada.

### 10.1 Vistas públicas

| Vista | Finalidad |
|---|---|
| `contract.v_resource` | Publicar recursos y su UUID estable |
| `contract.v_forecast` | Publicar pronósticos |
| `contract.v_scenario` | Publicar escenarios |
| `contract.v_percentile_statistic` | Publicar percentiles con método y procedencia |
| `contract.v_recommendation` | Publicar recomendaciones y su estado |

Estas vistas no exponen las claves identity internas. `cco_app` debe utilizar:

- `resource_uid` como `data_resource_uid`;
- `recommendation_uid` para interacciones de usuario;
- `scenario_uid` y `forecast_uid` para referencias externas.

No se crean FKs PostgreSQL cross-database. La autorización por organización, proyecto y usuario se resuelve en `cco_app` y en el backend que traduce ese contexto a UUIDs de `cco_data`.

Los triggers de `contract` impiden modificar los UUID públicos de recursos, forecasts, escenarios y recomendaciones después de publicados.

---

## 11. Rendimiento y escalabilidad

### 11.1 Particionamiento

Las series de mayor volumen están particionadas por mes:

- `raw.metric_reading`;
- `raw.db_resource_stats`;
- `raw.activity_log`;
- `feat.feature_snapshot`.

La función `raw.ensure_monthly_partitions(date)` crea las particiones mensuales para estas tablas. La partición `DEFAULT` evita que la ingesta falle cuando todavía no existe una partición específica, pero debe vigilarse.

`raw.v_default_partition_health` permite detectar filas caídas en `DEFAULT`. El resultado esperado es cero.

### 11.2 Índices

El DDL contiene índices para:

- lectura temporal por recurso;
- costos por recurso y fecha;
- recursos por región y estado;
- forecasts por recurso y métrica;
- escenarios por recurso;
- candidatos y simulaciones;
- evidencia y riesgo por recomendación;
- percentiles por recurso, métrica y ventana.

Los índices BRIN están dirigidos a tablas temporales grandes. Los índices B-tree se usan para accesos selectivos y joins frecuentes.

Cada índice adicional tiene un costo de escritura. Las consultas críticas deben validarse con:

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT ...;
```

### 11.3 Idempotencia

El collector debe usar claves naturales y `INSERT ... ON CONFLICT`. El DDL define unicidad lógica para:

- cargos con `source_charge_id`;
- forecasts por modelo, recurso, métrica, horizonte y periodo;
- simulaciones por candidato, modelo y ventana.

Esto permite reintentos seguros sin multiplicar datos.

---

## 12. Seguridad y permisos

La separación recomendada de roles es:

| Rol | Responsabilidad |
|---|---|
| `cco_data_owner` | Propietario de objetos; no debe usarse desde la aplicación |
| `cco_data_migrator` | DDL, migraciones y particiones |
| `cco_data_collector` | Ingesta y lectura controlada |
| `cco_data_mlservice` | Lectura analítica y escritura de `feat`/`ml` |
| `cco_data_optimizer` | Lectura analítica y escritura limitada en `opt` |
| `cco_data_backend` | Lectura de vistas y operaciones permitidas |
| `cco_data_readonly` | Lectura de vistas públicas |
| `cco_data_archive` | Archivado y retención |

El collector debe tener `INSERT` y `SELECT` en `raw`, pero no `UPDATE`, `DELETE` ni `TRUNCATE`. El archivado y la eliminación de particiones deben quedar restringidos al rol de archivado o migración.

La aplicación no debe conectarse directamente desde el frontend a PostgreSQL. El acceso debe pasar por el backend.

No deben almacenarse en la base:

- secretos de cliente;
- access tokens;
- refresh tokens;
- API keys;
- contraseñas.

Los payloads JSONB de proveedores deben redactarse. Los cuerpos grandes deberían almacenarse en object storage, conservando en PostgreSQL URI, checksum, tipo y tamaño.

---

## 13. Retención y archivado

La política operativa inicial documentada es:

| Área | Retención sugerida |
|---|---|
| `raw` | 3–6 meses, según costo y necesidad de reprocesamiento |
| `core` | Histórico operativo |
| `feat` | Histórico para entrenamiento y auditoría |
| `ml` | Indefinida o según política de experimentos |
| `opt` | Indefinida para trazabilidad |

El archivado de `raw` debe seguir este orden:

1. confirmar que la partición esté cerrada;
2. exportarla a almacenamiento de objetos;
3. validar checksum y tamaño;
4. ejecutar `DETACH PARTITION`;
5. verificar que el archivo sea recuperable;
6. eliminar la tabla desprendida solo después de la verificación.

---

## 14. Flujo operativo recomendado

### Ingesta

1. Registrar una fila en `raw.collector_run`.
2. Consultar únicamente métricas declaradas en `catalog.metric_catalog`.
3. Insertar respuestas en las tablas `raw` correspondientes.
4. Registrar conteos, errores y payloads necesarios para auditoría.
5. Reintentar con `ON CONFLICT` sin modificar históricos.

### Preparación analítica

1. Asociar telemetría con `core.resource`.
2. Resolver el snapshot de configuración vigente para cada ventana.
3. Calcular features agregadas.
4. Calcular percentiles solo con muestras individuales o histogramas documentados.
5. Registrar el resultado en `feat.percentile_statistic`.
6. Construir `feat.feature_snapshot`.

### Machine learning

1. Crear un `ml.experiment`.
2. Versionar el dataset en `ml.training_dataset`.
3. Ejecutar y registrar `ml.training_run`.
4. Registrar el artefacto en `ml.model_version`.
5. Evaluar errores e intervalos en `ml.forecast_evaluation`.
6. Promover a `champion` solo después de comparar contra baselines.
7. Emitir forecasts con `forecast_uid`.

### Optimización

1. Crear un `opt.scenario`.
2. Generar candidatos desde `catalog.sql_sku`.
3. Simular cada configuración.
4. Evaluar riesgo y restricciones.
5. Incorporar evidencia estructurada.
6. Crear una recomendación versionada.
7. Exponerla a `cco_app` mediante `contract.v_recommendation`.
8. Registrar la decisión humana en la aplicación, no en el motor analítico salvo en los campos de decisión previstos.

---

## 15. Observaciones técnicas sobre el diseño actual

Estas observaciones describen el comportamiento y los puntos de atención del esquema vigente; no representan una comparación con otro diseño.

### 15.1 `raw.metric_sample` requiere una política de volumen

A diferencia de varias tablas temporales de `raw`, `raw.metric_sample` no está particionada. Si se ingieren muestras individuales a alta frecuencia, su crecimiento puede superar rápidamente al de las tablas agregadas.

Debe definirse si la ingesta de muestras será limitada, retenida por menos tiempo o particionada posteriormente por `observed_at_utc`.

### 15.2 Identificación cuando `source_sample_id` es NULL

La unicidad de `raw.metric_sample` incluye `source_sample_id`. En PostgreSQL, los valores `NULL` no se consideran iguales para una restricción UNIQUE. Por tanto, si una fuente no entrega identificador de muestra, la clave no evita por sí sola duplicados con la misma combinación de recurso, métrica, instante y fuente.

La idempotencia de esas fuentes debe resolverse mediante una clave generada por el collector, un hash de contenido o una regla específica de deduplicación.

### 15.3 Percentiles en `feat.feature_snapshot`

El snapshot ancho conserva las columnas `cpu_p95`, `cpu_p99`, `data_io_p95`, `log_write_p95`, `sessions_p95` y `latency_p95_ms`, mientras que la validez estadística vive en `feat.percentile_statistic`.

El pipeline debe mantener ambas capas sincronizadas y no poblar el snapshot con un valor cuyo método sea `not_available`.

### 15.4 Forecasts y percentiles de simulación

Los targets explícitos de `ml.forecast` son `cpu_avg`, `workload_intensity` y `storage_used_bytes`. `opt.scenario_simulation` también contiene campos como `predicted_cpu_p95`, `predicted_cpu_p99` y `predicted_latency_p95_ms`.

Para que esos campos sean reproducibles, el pipeline debe documentar si proceden de modelos específicos, intervalos de predicción, transformaciones de distribución o reglas de simulación. No deben interpretarse automáticamente como forecasts directos si no existe un modelo o método registrado para ellos.

### 15.5 Estado de recomendación y auditoría externa

La recomendación contiene campos de decisión humana y aplicación. La aplicación de cambios sobre Azure no forma parte del contrato externo definido: la recomendación es una salida analítica y el sistema debe mantener separada la decisión de la ejecución.

Si posteriormente se automatizara una aplicación, se necesitaría un contrato operativo adicional con autorización, idempotencia, auditoría y controles de reversión.

---

## 16. Conclusión

La versión actual de `cco_data` es una base analítica orientada a trazabilidad y reproducibilidad. Su principal fortaleza es que conserva el ciclo completo desde el dato observado hasta la recomendación, incluyendo configuración, costo, modelo, simulación, riesgo y evidencia.

La arquitectura también establece una frontera clara con `cco_app`: la aplicación mantiene identidad de producto y contexto de usuario, mientras `cco_data` mantiene hechos cloud y decisiones analíticas. El schema `contract` formaliza esa frontera mediante vistas read-only y UUIDs estables.

El tratamiento de percentiles es explícito: los agregados de intervalo sirven para estadísticas agregadas, pero los percentiles requieren muestras individuales o histogramas con pesos. Cuando esa evidencia no existe, el valor correcto es `not_available`, no una aproximación silenciosa.

Para operar la base de forma segura, los controles críticos son:

- crear y supervisar particiones mensuales;
- mantener `raw` append-only mediante permisos;
- ejecutar ingestas idempotentes;
- controlar el crecimiento de `raw.metric_sample`;
- verificar el método de cada percentil;
- proteger UUIDs públicos;
- evitar exponer claves identity internas;
- evaluar modelos con datasets y ventanas versionadas;
- conservar evidencia y riesgo de cada recomendación;
- validar consultas y retención con datos reales.
