# Decision Log

| ID | Fecha | Decisión | Responsable | Motivo | Evidencia | Estado |
|---|---|---|---|---|---|---|
| DEC-001 | 2026-09-01 | Usar la pregunta de investigación e hipótesis propuestas por IA como base | Equipo | Justificar el proyecto ante el profesor; crítico para la primera entrega | MTG-001 | En validación (prof. Aisner) |
| DEC-002 | 2026-09-01 | Mantener gobernanza/trazabilidad del proyecto con registro de decisiones | Equipo | Respaldo ante preguntas del profesor sobre decisiones | MTG-001 | Acordada |
| DEC-003 | 2026-09-01 | Usar IA para generar informes y avances desde la documentación del repositorio | Equipo | Agilizar redacción de entregables | MTG-001 | Acordada |
| DEC-004 | 2026-09-01 | Frontend web tipo dashboard (Next.js seleccionado; Flutter descartado) | Anderson / Omar | Solo dashboard, reportes y recomendaciones | MTG-001 | Acordada |
| DEC-005 | 2026-09-01 | Backend como API Gateway en Go | Equipo | Facilidad de despliegue (binario único, dockerizable) | MTG-001 | Acordada |
| DEC-006 | 2026-09-01 | Módulo ML en servicio independiente; arquitectura de 2 microservicios | Jesús | Evitar bloqueos y competencia por recursos con el backend | MTG-001 | Acordada |
| DEC-007 | 2026-09-01 | Capa de abstracción con modelo de datos común y conectores por proveedor | Equipo | Preparar arquitectura para Azure/AWS/GCP | MTG-001 | Acordada |
| DEC-008 | 2026-09-01 | PostgreSQL como base de datos | Equipo | - | MTG-001 | Acordada |
| DEC-009 | 2026-09-01 | Reportes: guardar resultado en BD y generar PDF en tiempo de ejecución | Equipo | Evitar almacenar archivos | MTG-001 | Acordada (dónde: app.report_requests / app.report_artifacts) |
| DEC-010 | 2026-09-01 | Sin storage tipo bucket; archivos/modelos vía GitHub | Equipo | - | MTG-001 | Acordada |
| DEC-011 | 2026-09-01 | Autenticación con proveedor externo (OIDC/Entra ID implementado) | Equipo | No desarrollar auth desde cero | MTG-001 | Acordada (proveedor: Entra ID) |
| DEC-012 | 2026-09-01 | El usuario provee su propia API key del proveedor cloud | Equipo | Habilitar monitoreo de sus recursos | MTG-001 | Acordada |
| DEC-013 | 2026-09-01 | Laboratorios con Terraform; evaluar S3 y CloudFront | Jesús | Generar datos sintéticos e infraestructura demostrable | MTG-001 | Acordada |
| DEC-014 | 2026-09-01 | Chat/asistente reutiliza el motor de explicaciones; sin chat con memoria | Equipo | Evitar construir chat con memoria propia | MTG-001 | Acordada |
| DEC-015 | 2026-09-01 | Notificaciones descartadas para este alcance | Equipo | Complejidad; posible investigación futura | MTG-001 | Acordada |
| DEC-016 | 2026-09-01 | Scrum; PRs aprobados por al menos 2 revisores | Equipo | Revisión compartida y trazable | MTG-001 | Acordada |
| DEC-017 | 2026-09-01 | Reuniones semanales los domingos 10:00 a.m. | Equipo | Retrospectiva semanal | MTG-001 | Acordada |
| DEC-018 | 2026-09-01 | Prioridades MVP: backend (auth, gestión BD, orquestación, recomendaciones) y ML (forecasting, capacidad, riesgo) | Equipo | Enfocar esfuerzos del primer periodo | MTG-001 | Acordada |
| DEC-019 | 2026-09-01 | Reentrenamiento del modelo solo con cantidad mínima de datos nuevos | Jesús | Evitar consumo innecesario de recursos | MTG-001 | Acordada |
| DEC-020 | 2026-09-12 | Continuar el proyecto con modificaciones en histórico, workloads, costos y métricas | Equipo del proyecto | Ajustar alcance tras EXP-001 | MTG-002 | Adoptada |
| DEC-021 | 2026-09-12 | Entregar una versión beta al profesor antes de la entrega formal | Equipo del proyecto | Retroalimentación temprana | MTG-002 | Adoptada |
| DEC-022 | 2026-09-12 | Separar la persistencia en una base de aplicación y otra de IA/recolección | Equipo del proyecto | Separar datos operativos y analíticos | MTG-002, ADR-002 | Adoptada |
| DEC-023 | 2026-09-12 | Usar PostgreSQL local en la fase inicial | Equipo del proyecto | Validar esquemas sin infraestructura compartida | MTG-002, REQ-007 | Adoptada |
| DEC-024 | 2026-09-12 | Mantener las recomendaciones sin aplicación automática | Equipo del proyecto | Evitar modificar recursos sin decisión del usuario | MTG-002, REQ-004 | Adoptada |
| DEC-025 | 2026-09-12 | Priorizar problema, justificación, objetivos, usuarios, alcance y antecedentes para el primer avance | Equipo del proyecto | Consolidar el primer avance académico | MTG-002, REQ-005 | Adoptada |
| DEC-026 | 2026-09-12 | Mantener el sprint o retrospectiva los domingos a las 10:00 a. m. | Equipo del proyecto | Ritmo de entrega semanal | MTG-002 | Adoptada |
| DEC-027 | 2026-09-28 | Adoptar FOCUS como formato de normalización de datos de costo/uso en la capa de ingesta | Equipo del proyecto | Estandarizar datos de costo | RES-001 | Propuesta |
| DEC-028 | 2026-10-29 | Autenticación y autorización dentro del monolito Go (sin gateway separado) | Equipo del proyecto | Unificar auth en un solo binario | ADR-004 | Aceptada |
| DEC-029 | 2026-10-29 | Pruebas unitarias junto al código e integración cross-package | Equipo del proyecto | Trazabilidad de pruebas | ADR-005 | Aceptada |
| DEC-030 | 2026-10-30 | Logging estructurado con log/slog | Equipo del proyecto | Trazabilidad transversal | ADR-006 | Aceptada |
| DEC-031 | 2026-10-30 | Argon2id, rate limiting, account lockout, security headers, refresh token rotation | Equipo del proyecto | Seguridad mínima para Report 1.0 | ADR-007 | Aceptada |
| DEC-032 | 2026-11-11 | Adoptar Valkey para estado compartido de rate limiting y lockout | Equipo del proyecto | Estado distribuido sin dependencia única | ADR-008 | Aceptada |
| DEC-033 | 2026-11-11 | Parámetros Argon2id fijos; migrar a formato PHC antes de evolucionar | Equipo del proyecto | Evitar invalidar hashes existentes | ADR-007 | Aceptada |
| DEC-034 | 2026-11-11 | Un solo handler global de logs a stdout | Equipo del proyecto | Simplicidad operativa en prototipo | ADR-006 | Aceptada |
| DEC-035 | 2026-11-11 | Acceso a proyecto = membresía de organización; sin ACL por proyecto | Equipo del proyecto | Simplificar modelo de autorización | ADR-004 | Aceptada |
| DEC-036 | 2026-11-15 | Auditoría persistente en cco_app.audit.events, separada de slog | Equipo del proyecto | Trazabilidad append-only autorizada | ADR-014 | Aceptada |
