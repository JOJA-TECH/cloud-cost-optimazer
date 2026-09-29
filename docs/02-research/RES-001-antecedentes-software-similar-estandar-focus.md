# RES-001 — Antecedentes de software similar y estudio del estándar FOCUS

- **ID:** RES-001
- **Fecha:** 2026-09-28
- **Investigador:** Equipo del proyecto (registrado vía asistencia de IA; fuentes verificadas por acceso directo a cada URL)
- **Área:** Investigación / antecedentes / normalización de datos de costos
- **Estado:** Borrador (pendiente de revisión del equipo según `docs/MANUAL-DE-USO.md` §15)
- **Relacionado con:**
  - TASK-004 de MTG-001 ("Investigar antecedentes de software similar y el estándar FOCUS")
  - DEC-007 (capa de abstracción con modelo de datos común y conectores por proveedor)
  - DEC-020 y DEC-025 (modificaciones tras EXP-001 y prioridades del primer avance)
  - REQ-004 (recomendaciones sin aplicación automática) y REQ-005 (entrega beta académica)
  - EXP-001 / LAB-001 / DATA-001 (fuentes de datos Azure ya verificadas)
  - MTG-002 (el primer avance exige antecedentes)

## Pregunta de investigación

1. ¿Qué soluciones de software existen para el análisis y la optimización de costos de recursos en la nube, y qué enfoque de normalización de datos utilizan?
2. ¿Qué es el estándar FOCUS y en qué medida es aplicable como base de normalización de datos de costo/uso entre proveedores cloud para la capa de abstracción del proyecto?

## Contexto

El proyecto construye una plataforma para analizar y optimizar los costos de bases de datos en la nube (inicialmente Azure SQL Database) mediante monitoreo, analítica predictiva y recomendaciones que no se aplican automáticamente (REQ-004). En MTG-001 se decidió una capa de abstracción con modelo de datos común y conectores por proveedor (Azure/AWS/GCP preparados) (DEC-007).

EXP-001 y LAB-001 verificaron la disponibilidad de las fuentes de Azure, y el informe de factibilidad del laboratorio ya contempla el "modelado FOCUS del costo" (Cost Management con `resource_id`). Sin embargo, el estándar FOCUS no estaba documentado ni investigado formalmente en el repositorio, y TASK-004 permanecía pendiente. Este documento cierra ese vacío y aporta los antecedentes exigidos por el primer avance académico (DEC-025, REQ-005).

## Alcance y método

- **Tipo:** investigación documental (desk research) sobre fuentes primarias y secundarias.
- **Criterios de selección de antecedentes:** herramientas que aborden recolección/normalización de datos de costo y uso, análisis, recomendaciones o gobernanza de costos en la nube.
- **Verificación:** cada referencia fue accedida y confirmada el 2026-09-28; el registro de fuentes con tipo y estado de verificación está en `literature-review/RES-001-sources.md`.
- **Limitación:** no se ejecutaron pruebas comparativas (benchmarks) de las herramientas evaluadas; la evaluación es documental. Esto no impide usar el contenido como antecedente, pero delimita su alcance.

## Hallazgos

### Hallazgo 1 — Antecedentes comerciales (herramientas nativas y multicloud)

| Herramienta | Productor | Alcance | Enfoque de normalización | Ref. |
|---|---|---|---|---|
| Microsoft Cost Management + Azure Advisor | Microsoft | Nativa de Azure; costos, presupuestos, recomendaciones | Exportación de datos de costo/uso en esquema FOCUS | [8][9] |
| AWS Billing (Data Exports) + Cloud Intelligence Dashboards | AWS | Nativa de AWS; costos/uso y dashboards | Exportación FOCUS 1.0 (GA nov 2024) y 1.2 (GA nov 2025) | [10][11][12] |
| Google Cloud Billing (export a BigQuery) | Google | Nativa de GCP; export de facturación | Soporte de datasets FOCUS 1.0 | [3] |
| Apptio Cloudability (IBM) | IBM | Multicloud; TBM/FinOps, asignación y optimización | Ingesta de costos de Azure vía Cost Management Exports (ruta hoy orientada a FOCUS) | [18] |
| Kubecost | IBM | Costos de Kubernetes/clústeres | Modelo propio basado en OpenCost | [14][15][19] |

Otras soluciones citadas habitualmente en el mercado (Flexera One, CloudHealth/Broadcom, CloudZero, Finout) quedan **pendientes de verificación de fuente primaria** y no se usan como antecedente en esta fase (ver `literature-review/RES-001-sources.md`).

### Hallazgo 2 — Antecedentes open source

| Proyecto | Comunidad | Alcance | Relevancia para el proyecto | Ref. |
|---|---|---|---|---|
| OpenCost | CNCF (Sandbox dic 2022; incubación oct 2024) | Modelo de costos abierto para Kubernetes: monitoreo de costos en tiempo real y asignación por servicio/namespace/etiqueta | Precedente de un modelo de costos abierto por recurso; su alcance es contenedores, no bases de datos PaaS | [13][14][15] |
| Cloud Custodian | CNCF (incubación 2022) | Motor de reglas de gobernanza como código (seguridad, costo, gobernanza) con políticas YAML que ejecutan acciones | Contraste directo con REQ-004: automatiza acciones; el proyecto decide recomendaciones sin cambios automáticos | [16][17] |
| FOCUS Validator | FinOps Foundation | Aplicación Python que valida datasets contra la especificación FOCUS | Herramienta candidata para validar la ingesta de costos del proyecto (futuro TEST) | [7] |

### Hallazgo 3 — Qué es el estándar FOCUS

FOCUS (FinOps Open Cost and Usage Specification) es una especificación abierta, iniciada bajo la FinOps Foundation con soporte de la Linux Foundation, que define un esquema común (columnas, tipos, semántica y requisitos normativos) para los datos de facturación de costo y uso en la nube [1][6].

- **Versiones:** previsualizaciones 0.5 y 1.0 Preview; **1.0** ratificada en junio de 2024 [2]; **1.1** ratificada el 7 de noviembre de 2024 [3]; **1.2** (2025) amplía el alcance hacia datos SaaS/PaaS [4]; se anunció **1.3** (dic 2025) con compromisos de contrato y asignación de costos compartidos [5]. Publicaciones incrementales aproximadamente dos veces al año [3].
- **Objetivo práctico:** fusionar datos de facturación de varios proveedores **sin aplicar esquemas de normalización propietarios**; con 1.0 fue la primera vez que los practitioners pudieron hacerlo con los cuatro proveedores principales [3].
- **Adopción por proveedores:** AWS, Google Cloud, Microsoft Azure y OCI liberaron soporte de datasets FOCUS (1.0); AWS y Microsoft publican informes de conformidad que indican qué columnas les faltan respecto de la última versión [3].
- **Columnas clave v1.1/v1.2** (según el esquema publicado por Microsoft para su export FOCUS [9]): `BilledCost`, `EffectiveCost`, `ListCost`, `ListUnitPrice`, `ContractedCost`, `ContractedUnitPrice`, `ChargeCategory`, `ChargeFrequency`, `ChargePeriodStart/End`, `BillingPeriodStart/End`, `ResourceId`, `ResourceName`, `ResourceType`, `ServiceCategory`, `ServiceSubcategory`, `SkuId`, `SkuMeter`, `SkuPriceDetails`, `CommitmentDiscountCategory/Id/Quantity/Status/Type/Unit`, `PricingCategory`, `PricingQuantity`, `PricingUnit`, `ProviderName`, `PublisherName`, `RegionId/Name`, `InvoiceId`, `SubAccountId/Name`, `ConsumedQuantity/Unit`.

### Hallazgo 4 — Aplicación de FOCUS al proyecto

El mapeo entre las necesidades del proyecto y las columnas FOCUS es directo:

| Necesidad del proyecto | Columnas FOCUS que la cubren |
|---|---|
| Costo por base de datos / recurso (Azure SQL) | `ResourceId`, `ResourceName`, `ResourceType` |
| Series temporales de costo para el módulo ML (forecast, anomalías) | `ChargePeriodStart/End`, `ChargeCategory`, `BilledCost`, `EffectiveCost` |
| Análisis de costos amortizados vs. facturados (reservas/compromisos) | `EffectiveCost` vs `BilledCost`; `CommitmentDiscount*` |
| Análisis de reservas de Azure SQL (capacity reservations) | `CapacityReservationId/Status` (v1.1) |
| Diferenciación de servicios (bases de datos vs. cómputo vs. storage) | `ServiceCategory`, `ServiceSubcategory` (v1.1) |
| Modelo de precios (on-demand, reserva, spot) | `PricingCategory`, `PricingQuantity`, `PricingUnit` |
| Conectores AWS/GCP de la capa de abstracción (DEC-007) | Export FOCUS disponible en los cuatro proveedores → menos ETL propietario por conector |

Adicionalmente:

- En Azure, la export FOCUS proviene de Cost Management [8][9], la misma fuente ya verificada en LAB-001/EXP-001 para `resource_id`, lo que valida el "modelado FOCUS del costo" previsto en el informe de factibilidad.
- El **FOCUS Validator** [7] puede usarse como control de calidad de la ingesta (candidato a futuro TEST apuntando al requisito de persistencia separada REQ-002 / ADR-002).
- FOCUS normaliza **facturación**, no cubre métricas de utilización en tiempo real (Azure Monitor, Query Store): el proyecto mantiene su propio modelo que une costo (FOCUS) con métricas, tal como anticipó LAB-001.

### Hallazgo 5 — Vacíos detectados y diferenciación del proyecto

1. Las herramientas comerciales son amplias pero genéricas: su profundidad de recomendación para bases de datos PaaS específicas (Azure SQL: DTU/vCore, Query Store, rightsizing) es limitada y sus algoritmos no están documentados.
2. OpenCost cubre Kubernetes/contenedores, no bases de datos PaaS.
3. Cloud Custodian automatiza acciones (gobernanza como código), en contraste con REQ-004 del proyecto.
4. Ninguna de las fuentes revisadas documenta un **modelo predictivo validado específicamente para optimización de capacidad en Azure SQL Database**, lo que respalda el diferenciador del proyecto y su hipótesis de investigación (DEC-001, pendiente de validación con el profesor).

## Comparaciones / contradicciones

- **FOCUS vs. métricas operativas:** FOCUS resuelve la normalización de facturación; no sustituye Azure Monitor/Query Store. El sistema necesita ambos planos de datos (costo normalizado + utilización) — coincide con la separación de persistencia de ADR-002.
- **Esquemas propietarios vs. FOCUS:** las CMP comerciales anteriores a FOCUS usan esquemas propios; FOCUS reduce el lock-in, pero existen brechas de conformidad por proveedor (los vendedores publican informes de conformidad con columnas faltantes) [3][9].
- **`EffectiveCost` vs `BilledCost`:** costo amortizado vs. facturado; el análisis interno debe elegir y documentar consistentemente cuál usa para cada KPI. La v1.1 añade metadatos para no reprocesar archivos históricos al migrar de versión [3].

## Conclusión

1. Existen antecedentes suficientes y verificables de soluciones similares: nativas por proveedor (Cost Management/Advisor, AWS Billing/Dashboards, GCP Billing), comerciales multicloud (Cloudability) y open source (OpenCost, Cloud Custodian, FOCUS Validator).
2. El estándar FOCUS es **aplicable y conveniente** como formato de normalización de costo/uso de la capa de ingesta del proyecto: lo soportan los cuatro proveedores principales, tiene versiones estables (1.0–1.2) y herramientas de validación existentes.
3. Queda documentado un vacío en soluciones comparables: optimización predictiva específica para bases de datos PaaS, que es el enfoque del proyecto.
4. La investigación se limita a evidencia documental; cualquier comparación cuantitativa futura con estas herramientas requeriría un experimento propio (EXP) con datasets del proyecto.

## Impacto sobre el proyecto

- **Avance 1 (REQ-005, DEC-025):** materializa la sección de antecedentes y aporta referencias verificables para el documento académico.
- **Arquitectura y modelo de datos:** la tabla de costos de la BD de recolección/IA debería alinearse con columnas FOCUS v1.1+ (insumo para TASK-007 de MTG-001 y para un futuro ADR del modelo de datos).
- **Conectores (DEC-007):** usar exports FOCUS por proveedor reduce el esfuerzo de ETL propietario de cada conector.
- **QA:** FOCUS Validator como candidato para un TEST de validación de ingesta.
- **Riesgos:** se mantiene la dependencia de fuentes externas ya registrada en EXP-001 (ver `docs/registry/EXPERIMENT-INDEX.md`).

## Decisiones derivadas

- **DEC-027** — Adoptar FOCUS como formato de normalización de datos de costo/uso en la capa de ingesta. Estado: Propuesta (a ratificar por el equipo en el siguiente sprint).

## Tareas derivadas

- Alinear el modelo de datos (TASK-007, MTG-001) con las columnas FOCUS v1.1 al diseñar la tabla de costos.
- Evaluar `finopsfoundation/focus_validator` cuando exista la primera ingesta de datos de costo (futuro TEST).

## Referencias

Fecha de acceso y verificación de todas las referencias: **2026-09-28** (detalle en `literature-review/RES-001-sources.md`).

1. FinOps Foundation — FOCUS™, FinOps Open Cost & Usage Specification: <https://focus.finops.org/>
2. FinOps Foundation — "FOCUS 1.0 is Now Available" (jun 2024): <https://www.finops.org/insights/focus-1-0-available/>
3. FinOps Foundation — "FOCUS 1.1 Now Available. Adoption Continues for Practitioners and Vendors." (nov 2024): <https://www.finops.org/insights/focus-1-1-available/>
4. FinOps Foundation — "Introducing FOCUS 1.2: SaaS/PaaS Support" (jun 2025): <https://www.finops.org/insights/focus-1-2-available/>
5. FinOps Foundation — "Introducing FOCUS 1.3" (dic 2025): <https://www.finops.org/insights/introducing-focus-1-3/>
6. Repositorio oficial de la especificación (GitHub): <https://github.com/FinOps-Open-Cost-and-Usage-Spec/FOCUS_Spec>
7. FOCUS Validator (GitHub, FinOps Foundation): <https://github.com/finopsfoundation/focus_validator>
8. Microsoft Learn — "What is FOCUS?": <https://learn.microsoft.com/en-us/cloud-computing/finops/focus/what-is-focus>
9. Microsoft Learn — "FOCUS cost and usage details file schema" (Cost Management): <https://learn.microsoft.com/en-us/azure/cost-management-billing/dataset-schema/cost-usage-details-focus>
10. AWS Blog — "Data Exports for FOCUS 1.0 is now in general availability" (nov 2024): <https://aws.amazon.com/blogs/aws-cloud-financial-management/data-exports-for-focus-1-0-is-now-generally-available/>
11. AWS Blog — "Data Exports for FOCUS 1.2 is now generally available" (nov 2025): <https://aws.amazon.com/blogs/aws-cloud-financial-management/data-exports-for-focus-1-2-is-now-generally-available/>
12. AWS Guidance — "FOCUS Dashboard" (Cloud Intelligence Dashboards): <https://docs.aws.amazon.com/guidance/latest/cloud-intelligence-dashboards/focus-dashboard.html>
13. CNCF Blog — "OpenCost: A new CNCF Sandbox Project for real-time Kubernetes cost monitoring" (dic 2022): <https://www.cncf.io/blog/2022/12/06/opencost-a-new-cncf-sandbox-project-for-real-time-kubernetes-cost-monitoring/>
14. OpenCost — "OpenCost Advances to CNCF Incubation" (oct 2024): <https://opencost.io/blog/cncf-incubation/>
15. Repositorio de OpenCost (GitHub): <https://github.com/opencost/opencost>
16. CNCF — Cloud Custodian (ficha de proyecto): <https://www.cncf.io/projects/cloud-custodian/>
17. CNCF Blog — "Cloud Custodian becomes a CNCF incubating project" (sep 2022): <https://www.cncf.io/blog/2022/09/14/cloud-custodian-becomes-a-cncf-incubating-project/>
18. IBM Docs — Cloudability: conexión a Azure EA vía Cost Management Exports: <https://www.ibm.com/docs/en/cloudability-commercial/cloudability-premium/saas?topic=azure-connecting-ea-cost-management-exports>
19. GitHub — Kubecost/OpenCost: anuncio de inclusión en la sumisión a CNCF (issue 1224): <https://github.com/kubecost/opencost/issues/1224>

> Nota sobre el formato bibliográfico: cuando el equipo adopte un gestor bibliográfico (p. ej. Zotero/BibTeX), estas referencias se migrarán al formato oficial; la lista anterior funciona como fuente primaria de la migración.
