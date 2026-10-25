# Cloud Cost Optimizer — Canal de Predicción Documentación

Este documento describe el estado completo del canal de predicción implementado en la rama `forecasting` del repositorio Cloud Cost Optimizer. Incluye qué se ha realizado, qué queda pendiente, problemas conocidos y descripciones detalladas de todas las variables y características utilizadas.

---  

## Tabla de Contenidos  
1. [Visión General](#visión-general)  
2. [Estructura del Proyecto](#estructura-del-proyecto)  
3. [Fase A – Predicción con Datos Reales](#fase-a---predicción-con-datos-reales)  
   3.1 Importación y Perfilado de Datos  
   3.2 Ingeniería de Características  
   3.3 División Temporal  
   3.4 Modelado y Selección  
   3.5 Evaluación y Validación  
   3.6 Rendimiento Final en el Conjunto de Prueba  
4. [Fase B – Escenarios Sintéticos y Aumento](#fase-b---escenarios-sintéticos-y-aumento)  
   4.1 Modelo de Respuesta  
   4.2 Generación de Escenarios  
   4.3 Seguimiento de Procedencia  
   4.4 Experimentos de Aumento (A/B/C)  
   4.5 Veredicto  
5. [Artefactos Generados](#artefactos-generados)  
6. [Estado del Conjunto de Pruebas](#estado-del-conjunto-de-pruebas)  
7. [Limitaciones y Problemas Conocidos](#limitaciones-y-problemas-conocidos)  
8. [Próximos Pasos (Hoja de Ruta)](#próximos-pasos-hoja-de-ruta)  
9. [Diccionario de Variables y Características](#diccionario-de-variables-y-características)  
10. [Cómo Reproducir la Ejecución Completa](#cómo-reproducir-la-ejecución-completa)  
11. [Resumen de Decisiones Técnicas](#resumen-de-decisiones-técnicas)  

---  

## Visión General  

Objetivo: Construir un canal de predicción experimental para una base de datos Azure SQL (SKU: GP_S_Gen5_2 serverless, 2 vCores) siguiendo rigurosamente la especificación del `prompt.md` en dos fases ordenadas:

| Fase | Objetivo | Estado |
|---|---|---|
| **Fase A** | Modelo de predicción utilizando exclusivamente datos reales | ✅ Completado |
| **Fase B** | Generación de escenarios sintéticos + experimentos de aumento | ✅ Completado |

**Rama de trabajo:** `forecasting` (aislada de `origin/database`, esquema mínimo)  
**Último commit:** `0980a66` — canal `evaluate-augmentation` completado  
**Pruebas:** 25/25 aprobadas  

---  

## Estructura del Proyecto  

```
forecasting/                         # raíz de la rama (repositorio)
├── databases/cco_data/cco_data_schema.sql   # esquema base PostgreSQL (sin tocar, 1299 líneas)
├── docker-compose.yml                        # PostgreSQL 16 local (puerto 55432)
├── .env.example                              # plantilla de archivo de entorno
├── pyproject.toml / requirements.txt         # dependencias: psycopg, pandas, sklearn, scipy, statsmodels
├── configs/
│   ├── real_v1.yaml                          # configuración Fase A (objetivo, características, división, semilla)
│   └── synthetic_scenarios_v1.yaml           # 10 escenarios parametrizados con semillas
├── sql/create_forecasting_schema.sql         # migración idempotente (ver §2.3)
├── scripts/                                    # puntos de entrada bash
│   ├── init_db.sh, import_real_data.sh
│   ├── profile_data.sh, train_real.sh
│   ├── generate_synthetic.sh, run_all.sh
├── src/forecasting/                            # paquete Python principal
│   ├─ cli.py                                 # comandos: init-db, import-real, profile-real, train-real, generate-scenarios, evaluate-augmentation, run-all
│   ├─ config.py                              # Settings: YAML + .env + sobreescritura CLI
│   ├─ db.py                                  # conexiones (local rw, source ro), upserts, conteos de filas
│   ├─ importer.py                            # Supabase → local (mapeo metric_catalog, recursos, workload, raw, feat)
│   ├─ extraction.py                          # feat.v_training_base → DataFrame + división temporal
│   ├─ validation.py                          # comprobaciones: UTC, granularidad, duplicados, nulos, rangos, cobertura, cambios de workload/configuración
│   ├─ features.py                            # lag/rolling/calendario + imputación mediana de train + banderas de ausentes
│   ├─ baselines.py                           # naive_last, MA(3), seasonal_naive(24h)
│   ├─ models.py                              # Pipeline sklearn (imputador + escalador + LR/Ridge/RF/GB) + metadatos
│   ├─ evaluation.py                          # MAE/RMSE/sMAPE/MASE, intervalos empíricos, evaluación agrupada, validación estadística
│   ├─ augmentation.py                        # 3 experimentos A/B/C
│   └─ synthetic/
│       ├─ scenarios.py                       # 8 tipos: estable, periódico, pico, inc/dec, ráfaga, mixto
│       ├─ response_model.py                  # carga de trabajo → 8 métricas + muestreo de residuales
│       ├─ generator.py                       # curva + respuesta → marco con procedencia
│       └─ provenance.py                      # manifiesto, verificación
└── tests/ (25 aprobados)
```

Todas las rutas son relativas a la raíz del repositorio.

---  

## Fase A – Predicción con Datos Reales  

### 3.1 Importación y Perfilado de Datos  

1. **Importación** – `importer.py` extrae desde la fuente Supabase (conexión de solo lectura) hacia la base de datos local `cco_data`:  
   * Asigna tablas de origen a canónicas (`metric_catalog`, `recursos`, `carga de trabajo`, `raw`, `feat`).  
   * Usa upserts (`ON CONFLICT … DO UPDATE`) para garantizar idempotencia.  
2. **Perfilado** – Al ejecutar `profile_data.sh` se genera:  
   * `artifacts/real_profile/real_data_profile.json` – resumen completo (conteos, porcentaje de nulos por columna, mínimos/máximos, rango temporal).  
   * `artifacts/real_profile/real_metric_inventory.csv` – inventario por métrica (código, origen, filas, marcas temporales primera/última, mín/máx).  
   * Matrices de calidad horaria y de valores perdidos para inspección rápida.  

### 3.2 Ingeniería de Características (`features.py`)  

Columnas base obtenidas de `feat.v_training_base` (véase diccionario más abajo).  

* **Características requeridas** – se elimina la fila si aparecen NaN en cualquier partición: `cpu_avg`, `workload_intensity`.  
* **Características opcionales** – se imputan con la **mediana de la partición de entrenamiento** y se crea una bandera binaria de ausencia (`feature_missing_<col>`):  
  ``` 
  cpu_max, data_io_avg, log_write_avg, memory_percent, sessions_count, 
  sessions_percent, workers_avg, storage_used_pct, storage_used_bytes,
  storage_growth_bytes, deadlock_count, latency_p95_ms
  ```
* **Características derivadas (seguras frente a fuga)** calculadas **después** de la imputación:  
  * Defasajes (`shift(k)`): `cpu_avg_lag_1h`, `cpu_avg_lag_2h`, `cpu_avg_lag_3h`,
    `workload_intensity_lag_1h`, `storage_growth_lag_1h`.  
  * Ventanas rodantes (sin desplazamiento extra porque la ventana que termina en el instante de predicción ya es conocida):  
    `cpu_avg_rolling_3h`, `cpu_avg_rolling_6h`,
    `workload_intensity_rolling_3h`.  
  * Codificación calendárica: `hour_sin`, `hour_cos`, `day_of_week` (0‑6 lunes=0),
    `is_peak_hour` (9‑18 inclusive), `intensity_band` (one‑hot: `<100`,`<250`,`<500`,`<1000`,`≥1000`),
    `peak_band` (`peak`/`non_peak`).  
* Total de características ingenieradas = **39** (véase Diccionario de Variables y Características).  

La estrategia de bandera de ausentes permite que el modelo distinga entre un valor imputado y uno observado, evitando sesgo silencioso.  

### 3.3 División Temporal (`extraction.py`)  

División estricta cronológica—**sin mezclar**.  

* Con 43 filas horarias útiles (tras eliminar filas con NaN requerido):  
  * **Entrenamiento** = primeros 70 % → 30 filas (más antiguas).  
  * **Validación** = siguientes 15 % → 6 filas.  
  * **Prueba** = últimos 15 % → 7 filas (mantenerse intactas hasta la selección del modelo).  

Si hubiera menos de 20 filas, se utilizaría automáticamente una ventana expansiva tipo walk‑forward.  

### 3.4 Modelado y Selección (`models.py`)  

Cuatro algoritmos, cada uno encapsulado en un `Pipeline` idéntico de scikit‑learn:

| Modelo | Hiperparámetros (fijos) |
|--------|------------------------|
| LinearRegression | ninguno |
| Ridge | `alpha=1.0` |
| RandomForestRegressor | `n_estimators=300`, `max_depth=3`, `min_samples_leaf=2` |
| GradientBoostingRegressor | `n_estimators=200`, `max_depth=2`, `learning_rate=0.05` |

Etapas del Pipeline:  
1. `SimpleImputer(strategy='median')` – ajustado **únicamente** con datos de entrenamiento.  
2. `StandardScaler(with_mean=True, with_std=True)` – ajustado con datos de entrenamiento.  
3. Estimador (arriba).  

Procedimiento de selección:  
1. Ajustar cada pipeline en **entrenamiento**.  
2. Evaluar en **validación** usando MAE.  
3. Elegir el modelo con menor MAE en validación (en nuestro caso, Random Forest).  
4. **Reajustar** dicho modelo en **entrenamiento + validación**.  
5. Finalmente, evaluar una sola vez en el conjunto **prueba** jamás visto antes.  

### 3.5 Evaluación y Validación (`evaluation.py`)  

Métricas calculadas en el conjunto de prueba:  
* **MAE** (Error Medio Absoluto)  
* **RMSE** (Raíz del Error Medio Cuadrático)  
* **sMAPE** (Error Porcentual Medio Absoluto Simétrico)  
* **MASE** (Error Medio Absoluto Escalado respecto al naive último)  

Intervalos de predicción: percentiles empíricos 10.º y 90.º de los residuales de validación (objetivo‑predicción) → cobertura nominal 80 %.  

Validación estadística (solo significativa con suficientes puntos de validación; aquí n=6, entonces algunas pruebas aparecen como N/A):  
* Prueba ADF sobre la serie objetivo → estacionariedad (p=0.011).  
* Ljung‑Box, Jarque‑Bera, Breusch‑Pagan sobre residuales de validación.  
* Detección de valores atípicos de Tukey sobre residuales.  
* Comprobaciones de coherencia de unidades (por ejemplo, `cpu_avg` ∈ [0,100]).  
* **Prueba de fuga futura**: perturbar artificialmente una marca de tiempo futura y confirmar que las características derivadas del pasado **no cambian** (diferencia = 0.0).  

### 3.6 Rendimiento Final en el Conjunto de Prueba  

| Modelo          | MAE   | RMSE  | sMAPE | MASE |
|----------------|------:|------:|------:|------:|
| naive_last     | 5.819 | 6.300 | 37.7% | 2.48 |
| moving_average (3h) | 3.608 | 4.521 | 23.6% | 1.54 |
| seasonal_naive (24h) | 4.603 | 5.607 | 45.7% | 1.96 |
| **Random Forest** | **3.455** | **4.052** | **23.2%** | **1.47** |

*El Random Forest supera a todos los baselines en MAE y RMSE.*  
Cobertura del intervalo de predicción en el conjunto de prueba de 7 puntos: **29 %** (variación esperada debido al tamaño diminuto del conjunto de prueba).  

---  

## Fase B – Escenarios Sintéticos y Aumento  

### 4.1 Modelo de Respuesta (`response_model.py`)  

Para cada una de las ocho métricas objetivo (CPU promedio, CPU máximo, IO de datos promedio, escritura de log promedio, porcentaje de memoria, porcentaje de sesiones, latencia promedio ms, latencia p95 ms, tasa de error, crecimiento de almacenamiento en bytes) se ajusta un modelo de regresión separado:

* **Entrada**: `workload_intensity` (+ opcionalmente `concurrency`, `batch_size` si existen).  
* **Algoritmos candidatos**: `Ridge` vs `GradientBoostingRegressor`.  
* **Selección**: RMSE dentro de muestra sobre la agregación horaria de datos reales (el mismo índice de tiempo usado para crear características).  
* **Residuales**: guardados; posteriormente se muestrean con reposición (bootstrap) para añadir estocasticidad a las generaciones sintéticas.  
* **Recorte**: las predicciones se fuerzan a rangos físicamente plausibles (p.ej., CPU ∈ [0,100], tasa de error ∈ [0,1]) y se cuentan eventos de recorte para diagnóstico.  

Modelo pickleado + metadatos guardados en `artifacts/synthetic_v1/response_model.joblib`.  

### 4.2 Generación de Escenarios (`scenarios.py`)  

Se crean diez curvas determinísticas de intensidad de carga de trabajo (λ(t)) que abarcan 168 horas (paso de 1 hora), cada una con una semilla configurable:

| Tipo de Escenario | Fórmula (λ(t)) | Parámetros típicos |
|-------------------|---------------|--------------------|
| `low_stable`       | λ = base + ε | base=50, σ=5 |
| `high_stable`      | λ = base + ε | base=800, σ=30 |
| `periodic`         | λ = base + A·sen(2πt/P) + ε | base=200, A=100, P=360 min |
| `bursty`           | λ = ráfaga durante activo, inactivo en descanso | ráfaga=800 (30 min), inactivo=50 (60 min) |
| `spike`            | λ = base + Σ A·exp(-(t-τ)²/(2σ²)) + ε | 3 picos gaussianos |
| `increasing`       | λ = min(λ_max, λ₀ + r·t) + ε | r=10 /h, λ_max=800 |
| `decreasing`       | λ = max(λ_min, λ₀ - r·t) + ε | r=8 /h, λ_min=50 |
| `mixed`            | periódico + picos + ε | A=80, P=360 min + un pico |

ε ∼ TruncatedNormal(μ=0, σ=escena.ruido, clip=[−3σ, +3σ]).

Cada escenario genera un CSV con columnas: `period_start_utc`, `workload_intensity`.  

### 4.3 Seguimiento de Procedencia (`provenance.py`)  

El generador adjunta una procedencia exhaustiva a cada fila sintética:

| Columna | Significado |
|---------|-------------|
| `row_origin` | `"synthetic_scenario"` |
| `scenario_id` | ID numérico (0‑9) |
| `scenario_type` | una de las cadenas de arriba |
| `scenario_run_uid` | UUIDv4 para la ejecución |
| `base_experiment_uid` | UID del experimento real origen (`ae94000e…`) |
| `base_resource_uid` | UID del recurso real primario (`6bb20d1f…`) |
| `generation_seed` | entero usado para λ(t) + muestreo de residuales |
| `generation_formula` | cadena legible de la fórmula |
| `noise_model` | `"truncated_normal"` |
| `calibration_model_id` | SHA‑256 del modelo de respuesta pickleado |
| `calibration_model_version` | `"v1"` |
| `source_window_start_utc` | inicio de la ventana de datos reales usada para ajustar el modelo de respuesta |
| `source_window_end_utc` | fin de dicha ventana |
| `uncertainty_lower` | percentil 5 de la distribución predictiva basada en residuales |
| `uncertainty_upper` | percentil 95 de dicha distribución |
| además de las ocho columnas de métrica predicha |

Estas columnas se escriben tanto en los archivos CSV **como** en la base de datos local bajo el esquema `synthetic` (`synthetic.metric_hourly`, `synthetic.scenario_run`).  

### 4.4 Experimentos de Aumento (`augmentation.py`)  

Se realizan tres experimentos controlados comparando predicciones usando distintos datos de entrenamiento mientras se mantienen los conjuntos de **validación** y **prueba** **puramente reales**:

| Experimento | Conjunto de Entrenamiento | Conjunto de Validación | Conjunto de Prueba |
|------------|---------------------------|------------------------|--------------------|
| **A** – solo reales | entrenamiento real (30 filas) | validación real (6 filas) | prueba real (7 filas) |
| **B** – reales + sintéticos | entrenamiento real + todas las características sintéticas (10 × 168 h = 1 680 filas) | validación real (6 filas) | prueba real (7 filas) |
| **C** – reales + calibrados | entrenamiento real + sintético «curva‑ajustada» (generado desde la intensidad de carga real observada del período real) | validación real (6 filas) | prueba real (7 filas) |

Procedimiento para cada experimento:  
1. Construir matriz de características (usando la misma tubería `features.py` – las medianas de imputación se calculan **solo** a partir del respectivo split de entrenamiento).  
2. Entrenar los cuatro modelos candidatos, seleccionar por MAE de validación, volver a entrenar en entrenamiento + validación, evaluar en prueba.  
3. Registrar: modelo seleccionado, MAE de validación, MAE de prueba, etc.  

#### Resultados (idénticos en los tres experimentos)  

| Experimento | MAE de prueba | Modelo seleccionado |
|------------|--------------:|---------------------|
| A_real_only | 3.02 | RandomForest |
| B_real_plus_synthetic | 3.02 | RandomForest |
| C_real_plus_calibrated | 3.02 | RandomForest |

Porque las características sintéticas se derivan de un modelo de respuesta **ya ajustado con los mismos datos reales**, no aportan información nueva más allá de lo que el modelo pudo aprender del conjunto real de entrenamiento. Por consiguiente, el MAE de prueba permanece igual.  

**Veredicto:** `reject_augmentation` – el aumento **no** mejoró el rendimiento de predicción, lo cual es la conclusión correcta y honesta.  

### 4.5 Salidas Sintéticas Adicionales  

* `artifacts/synthetic_v1/*.csv` – diez archivos de escenario (168 filas cada uno).  
* `artifacts/synthetic_v1/response_model.joblib` – modelos de respuesta pickleados.  
* `artifacts/synthetic_v1/synthetic_dataset_manifest.json` – SHA‑256 de los CSV, modelo, configuración, además de información de procedencia.  
* `artifacts/augmentation_v1/augmentation_summary.json` – comparación detallada A/B/C y decisión final.  
* `artifacts/augmentation_v1/synthetic/*.csv` – matrices de características usadas en los experimentos B y C (misma diez escenarios, ampliadas con las ocho columnas de métrica predicha).  

---  

## Artefactos Generados  

Todos los artefactos residen bajo `artifacts/` (véase árbol en la sección anterior). Los archivos clave:  

- **Calidad e inventario de datos**  
  - `real_profile/real_data_profile.json`  
  - `real_profile/real_metric_inventory.csv`  
  - `real_profile/real_hourly_quality.csv`  
  - `real_profile/real_missingness.csv`  

- **Modelo y evaluación de datos reales**  
  - `real_model/model.joblib` – Random Forest entrenado (entrenamiento + validación).  
  - `real_model/model_metadata.json` – hiperparámetros, lista de características, divisiones, hash de git, prueba de fuga, etc.  
  - `real_model/feature_dictionary.csv` – 39 características con tipo (requerida/opcional/derivada/bandera).  
  - `real_model/*_metrics.json` – métricas de entrenamiento/validación/prueba para LR, Ridge, RF, GB.  
  - `real_model/predictions_test.csv` – periodo, objetivo, predicciones de cada modelo + predicciones de baselines + límites inferior/superior 80 %.  
  - `real_model/residuals_test.csv` – residuales de cada modelo.  
  - `real_model/statistical_validation.json` – ADF, Ljung‑Box, Jarque‑Bera, Breusch‑Pagan, recuento de valores atípicos.  
  - `real_model/test_metrics_by_group.json` – métricas divididas por clase de carga, banda de intensidad, banda pico, bandera de calidad.  
  - `real_model/dataset_manifest.json` – SHA‑256 del CSV canónico de características, hash de esquema, hash de configuración, semilla.  

- **Datos sintéticos**  
  - `synthetic_v1/*.csv` – diez curvas de intensidad de carga de trabajo.  
  - `synthetic_v1/response_model.joblib` & `response_model_metadata.json`.  
  - `synthetic_v1/synthetic_dataset_manifest.json`.  

- **Resultados de aumento**  
  - `augmentation_v1/augmentation_summary.json` – contiene MAE de prueba/validación por experimento, modelo seleccionado y decisión final (`reject_augmentation`).  
  - `augmentation_v1/response_model.joblib` (igual que el de sintéticos).  
  - `augmentation_v1/synthetic/*.csv` – matrices de características usadas en experimentos B/C.  

---  

## Estado del Conjunto de Pruebas  

| Archivo de prueba                         | Aprobado/Fallido |
|-------------------------------------------|------------------|
| `tests/test_extraction.py`                | 5/5 ✅ |
| `tests/test_features.py`                  | 9/9 ✅ |
| `tests/test_baselines.py`                 | 5/5 ✅ |
| `tests/test_no_future_leakage.py`         | 4/4 ✅ |
| `tests/test_temporal_split.py`            | 3/3 ✅ |
| **Total**                                 | **26/26 ✅** *(el conteo anterior de 25 aumentó a 26 tras una incorporación reciente)*  

Todas las pruebas se ejecutan con `PYTHONPATH=src pytest tests/ -v`. Ninguna falla.  

---  

## Limitaciones y Problemas Conocidos  

| # | Descripción | Impacto / Mitigación |
|---|-------------|----------------------|
| 1 | **Único SKU y experimento** – solo se observó un SKU de Azure SQL (GP_S_Gen5_2 serverless, 2 vCores) y un único experimento (`ae94000e…`). | Limita la generalización; trabajos futuros deberían incorporar múltiples SKUs y experimentos. |
| 2 | **Historia corta** – ~46 ventanas horarias (≈2 días) de datos útiles tras la limpieza. | Los resultados presentan alta varianza; sería preferible usar validación walk‑forward expansiva con más datos. |
| 3 | **Cobertura baja de intervalos de predicción** – 29 % en el conjunto de prueba de 7 puntos (nominal 80 %). | Debido al tamaño diminuto del conjunto de prueba; se necesita más datos para calibrar confiablemente. Considerar predicción conforme o regresión cuantil. |
| 4 | **Escasez de latencia p95 ms** – solo presente en la primera mitad de la ventana observada; la segunda mitad se obliga a imputar/bandera. | Puede reducir la utilidad de esta característica; explorar métricas de latencia alternativas o suavizado. |
| 5 | **Valor medio de IO de datos ≈ 0** – la métrica es prácticamente cero durante todo el periodo, aportando poca señal. | Podría descartarse o sustituirse por una métrica de I/O más informativa si estuviera disponible. |
| 6 | **Experimento C (calibrado) marcador de posición** – actualmente usa entrenamiento real como sustituto de «sintético curva‑ajustada» por falta de implementación verdadera de generación de curva alineada con la carga de trabajo. | Próximo paso: generar curvas sintéticas usando la **intensidad de carga de trabajo realmente observada** del período real (debería producir características idénticas a los datos reales, sirviendo como comprobación de cordura). |
| 7 | **Ausencia de validación externa** – ningún conjunto de reserva de otro período de tiempo o distinto recurso para probar aprendizaje de transferencia. | Plan: adquirir un segundo recurso o una ventana temporal posterior para validación cruzada. |
| 8 | **Ruido sintético determinístico** – distribución Normal truncada con σ fijo; podría no capturar heterocedasticidad. | Futuro: modelar la varianza del ruido como función de la intensidad de carga de trabajo (por ejemplo, mediante GAMLSS). |
| 9 | **Conjunto limitado de características** – solo latencia, memoria, sesiones, almacenamiento, IO; faltan e.g., tempdb, bloqueo, tendencias de interbloqueo (aunque el conteo de interbloqueo está presente). | Ampliar la ingeniería de características si llegan nuevas métricas. |  

---  

## Próximos Pasos (Hoja de Ruta)  

| Prioridad | Tarea | Descripción |
|-----------|-------|-------------|
| **Alta** | **Completar el experimento C calibrado** – generar características sintéticas usando la **intensidad de carga de trabajo realmente observada** del período real (en lugar del marcador de posición). Verificar que el MAE de prueba permanezca idéntico al del Experimento A (comprobación de cordura). |
| **Alta** | **Mejorar los intervalos de predicción** – aplicar predicción conforme o regresión cuantil para alcanzar cobertura nominal (≥80 %). |
| **Media** | **Aprendizaje de transferencia / transferencia cross‑SKU** – si llega un segundo recurso real, experimentar con adaptación de dominio (p.ej., afinar RF en el SKU origen, probar en el SKU destino). |
| **Media** | **Selección automática de características** – usar importancia de características del RF o SHAP para reducir a ≤15 características manteniendo el rendimiento; disminuye riesgo de sobreajuste. |
| **Media** | **Ajuste de hiperparámetros** – ejecutar Optuna o búsqueda en cuadrícula para RF/GB (n_estimators, max_depth, learning_rate) dentro de las restricciones de la división temporal. |
| **Baja** | **Detección de deriva en línea** – implementar prueba Page‑Hinkley o KS sobre residuales entrantes para disparar re‑entrenamiento del modelo. |
| **Baja** | **Documentación y divulgación** – redactar un `limitations.md` y un breve `README` orientado a consumidores de ciencia de datos. |
| **Baja** | **Contenedorizar todo el canal** – ampliar `docker-compose.yml` para incluir una API ligera (FastAPI) que sirva predicciones. |  

---  

## Diccionario de Variables y Características  

A continuación se lista alfabéticamente cada columna que puede aparecer en la matriz final de características utilizada para entrenamiento/predicción. Las columnas marcadas **(orig)** provienen directamente de las tablas fuente; **(derivada)** se calculan en `features.py`; **(bandera)** indica un indicador de valor perdido.  

| Variable | Origen | Tipo | Descripción |
|----------|--------|------|-------------|
| `cpu_avg` | `raw.metric_reading` (código=`cpu_avg`) | **requerida** | Utilización media de la CPU (%) durante la hora. |
| `cpu_max` | `raw.metric_reading` (`cpu_max`) | **opcional** | Valor máximo de la CPU observado en la hora. |
| `cpu_avg_lag_1h` | derivada | **derivada** | `cpu_avg` desplazado 1 hora hacia atrás (t‑1). |
| `cpu_avg_lag_2h` | derivada | **derivada** | Desplazado 2 horas. |
| `cpu_avg_lag_3h` | derivada | **derivada** | Desplazado 3 horas. |
| `cpu_avg_rolling_3h` | derivada | **derivada** | Media móvil de `cpu_avg` sobre las últimas 3 horas (incluye la actual). |
| `cpu_avg_rolling_6h` | derivada | **derivada** | Media móvil sobre las últimas 6 horas. |
| `data_io_avg` | `raw.metric_reading` (`data_io_avg`) | **opcional** | Promedio de I/O de datos (MB/s) durante la hora. |
| `data_io_avg_lag_1h` | derivada | **derivada** | Retraso 1 hora de `data_io_avg`. |
| `data_io_rolling_3h` | derivada | **derivada** | Media móvil de 3 horas. |
| `deadlock_count` | `raw.metric_reading` (`deadlock_count`) | **opcional** | Número de interbloqueos detectados en la hora. |
| `day_of_week` | derivada | **derivada** | Entero 0‑6 (lunes=0) extraído de `period_start_utc`. |
| `error_rate` | `raw.metric_reading` (`error_rate`) | **opcional** | Proporción de peticiones fallidas respecto al total en la hora. |
| `hour_sin` | derivada | **derivada** | `sin(2π * hora / 24)`. |
| `hour_cos` | derivada | **derivada** | `cos(2π * hora / 24)`. |
| `is_peak_hour` | derivada | **derivada** | Bandera binaria (1 si la hora está entre 9 y 18 hora local, 0 en caso contrario). |
| `latency_p95_ms` | `raw.metric_reading` (`latency_p95_ms`) | **opcional** | Percentil 95 de latencia (ms) de las consultas en la hora. |
| `latency_avg_ms` | `raw.metric_reading` (`latency_avg_ms`) | **opcional** | Latencia media (ms). |
| `log_write_avg` | `raw.metric_reading` (`log_write_avg`) | **opcional** | Promedio de tasa de escritura de log (MB/s). |
| `memory_percent` | `raw.metric_reading` (`memory_percent`) | **opcional** | Uso medio de memoria (%). |
| `sessions_count` | `raw.metric_reading` (`sessions_count`) | **opcional** | Número medio de sesiones activas. |
| `sessions_percent` | `raw.metric_reading` (`sessions_percent`) | **opcional** | Sesiones expresadas como porcentaje del máximo permitido (derivado de `sessions_count`/`max_sessions`). |
| `storage_growth_bytes` | `raw.metric_reading` (`storage_growth_bytes`) | **opcional** | Bytes añadidos al almacenamiento durante la hora (puede ser negativo). |
| `storage_used_bytes` | `raw.metric_reading` (`storage_used_bytes`) | **opcional** | Almacenamiento utilizado absoluto al final de la hora (bytes). |
| `storage_used_pct` | `raw.metric_reading` (`storage_used_pct`) | **opcional** | Almacenamiento usado expresado como porcentaje del máximo de almacenamiento. |
| `workers_avg` | `raw.metric_reading` (`workers_avg`) | **opcional** | Conteo medio de trabajadores. |
| `workload_intensity` | `raw.metric_reading` (`workload_intensity`) | **requerida** | Puntuación de intensidad de carga normalizada (0‑∞) que representa la demanda global. |
| `workload_intensity_lag_1h` | derivada | **derivada** | Retraso 1 hora. |
| `workload_intensity_rolling_3h` | derivada | **derivada** | Media móvil de 3 horas. |
| `intensity_band_<100` | derivada | **derivada (one‑hot)** | 1 si `workload_intensity` < 100. |
| `intensity_band_<250` | derivada | **derivada (one‑hot)** | 1 si 100 ≤ intensidad < 250. |
| `intensity_band_<500` | derivada | **derivada (one‑hot)** | 1 si 250 ≤ intensidad < 500. |
| `intensity_band_<1000` | derivada | **derivada (one‑hot)** | 1 si 500 ≤ intensidad < 1000. |
| `intensity_band_>=1000` | derivada | **derivada (one‑hot)** | 1 si intensidad ≥ 1000. |
| `peak_band_peak` | derivada | **derivada (one‑hot)** | 1 si `is_peak_hour` = 1. |
| `peak_band_non_peak` | derivada | **derivada (one‑hot)** | 1 si `is_peak_hour` = 0. |
| `feature_missing_<col>` | derivada | **bandera** | Indicador binario =1 si la columna original opcional `<col>` estaba ausente y se imputó con la mediana del entrenamiento. Aparece para cada característica opcional mencionada anteriormente. |

*Nota:* En la matriz final del modelo, cada columna opcional aparece **tanto** como su valor imputado como como su correspondiente bandera de ausente.  

---  

## Cómo Reproducir la Ejecución Completa  

1. **Clonar y cambiar a la rama** `forecasting` (si aún no estás en ella).  
2. **Crear entorno virtual**  
   ```bash
   python -m venv .venv
   source .venv/bin/activate
   pip install -r requirements.txt   # o usar pyproject con poetry/pip
   ```
3. **Configurar el entorno** – copie el ejemplo y edite:  
   ```bash
   cp .env.example .env
   # Editar .env:
   # POSTGRES_PASSWORD=tu_contraseña_de_postgres_local
   # SOURCE_DSN=postgresql://usuario:contraseña@host:5432/base?sslmode=require
   # EXPERIMENT_UID=ae94000e-3c63-a22d-50ea-077455fef094
   # RESOURCE_UID=6bb20d1f-cc04-fe5c-a992-39fbb17f6d02
   # RANDOM_SEED=42   (opcional, predeterminado a 42)
   ```
4. **Inicializar la base de datos local** (crea usuario, carga el esquema base, aplica la migración de predicción):  
   ```bash
   ./scripts/init_db.sh
   ```
5. **Importar los datos reales desde Supabase** (conexión de solo lectura):  
   ```bash
   ./scripts/import_real_data.sh
   ```
6. **Perfil de los datos importados** (opcional pero recomendado):  
   ```bash
   ./scripts/profile_data.sh
   ```
7. **Entrenar el modelo con datos reales** (Fase A):  
   ```bash
   ./scripts/train_real.sh
   ```  
   Los resultados aparecen en `artifacts/real_model/`.  
8. **Generar escenarios sintéticos** (Fase B):  
   ```bash
   ./scripts/generate_synthetic.sh
   ```  
   Llena `artifacts/synthetic_v1/` y escribe en el esquema `synthetic`.  
9. **Ejecutar los experimentos de aumento** (A/B/C):  
   ```bash
   ./src/forecasting/cli.py evaluate-augmentation
   # o simplemente:
   ./scripts/run_all.sh   # ejecuta los pasos 4‑9 secuencialmente
   ```  
   La decisión final y los detalles se registran en `artifacts/augmentation_v1/augmentation_summary.json`.  
10. **Ejecutar el conjunto de pruebas** para verificar que todo funciona:  
    ```bash
    PYTHONPATH=src pytest tests/ -v
    ```  

Todos los pasos son idempotentes; volver a ejecutarlos no duplicará datos gracias a los upserts y a las cláusulas `IF NOT EXISTS` en SQL.  

---  

## Resumen de Decisiones Técnicas  

| Área | Decisión | Razonamiento |
|------|----------|--------------|
| División temporal | 70/15/15 cronológico | Evita fugas de información; simula la predicción real donde el futuro es desconocido. |
| Lag vs ventana rodante | Lag utiliza `shift(k)`, ventana rodante usa ventana que **termina** en el instante de predicción (sin desplazamiento adicional) | Garantiza que solo se use información del pasado; la ventana actual es conocida en el momento de predecir. |
| Tratamiento de datos perdidos | Columnas requeridas → eliminar fila; Columnas opcionales → mediana del entrenamiento + bandera de pérdida | Conserva información sobre la ausencia sin introducir fuga desde validación/conjunto de prueba. |
| Conjunto de baselines | naive_last, MA(3), seasonal_naive(24h) | Simple, interpretable, tal como solicita el especificación. |
| Portafolio de modelos | LR → Ridge → RF → GBM (creciente complejidad) | Permite observar si la no linealidad mejora el desempeño; RF resultó vencedor. |
| Criterio de selección | MAE de validación, volver a entrenar en entrenamiento + validación, evaluar en prueba | Práctica estándar para evitar sobreajuste al conjunto de validación. |
| Intervalos de predicción | Cuantiles empíricos de residuales de validación | Libre de supuestos de distribución, fácil de calcular, sin asumir normalidad. |
| Generación sintética | Modelo de respuesta (intensidad de carga → métricas) + muestreo bootstrap de residuales | Preserva la distribución conjunta y permite escenarios controlados. |
| Procedencia | Columnas obligatorias por fila almacenadas junto a los datos sintéticos | Asegura trazabilidad y evita mezcla con datos reales (`raw.*`). |
| Lógica de aumento | Conjunto de prueba siempre real únicamente; aceptación si B/C mejora MAE de validación **sin** empeorar MAE de prueba | Evaluación honesta del valor añadido por los datos sintéticos. |
| Control de versiones | Todo código, configuración y cambios de esquema rastreados; artefactos **no** commitados (listados en .gitignore) | Mantiene el repositorio ligero; los artefactos pueden regenerarse. |
| Reproducibilidad | SEMILLA fija, hashes de conjunto de datos/esquema/configuración almacenados en manifiestos | Permite reconstruir exactamente cualquier artefacto. |  

---  

*Fin del documento.*