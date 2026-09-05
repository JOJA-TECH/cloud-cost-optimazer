# Decision Log

| ID | Fecha | Decisión | Responsable | Motivo | Evidencia | Estado |
|---|---|---|---|---|---|---|
| DEC-001 | 2026-09-01 | Usar la pregunta de investigación e hipótesis propuestas por IA como base | Equipo | Justificar el proyecto ante el profesor; crítico para la primera entrega | MTG-001 | En validación (prof. Aisner) |
| DEC-002 | 2026-09-01 | Mantener gobernanza/trazabilidad del proyecto con registro de decisiones | Equipo | Respaldo ante preguntas del profesor sobre decisiones | MTG-001 | Acordada |
| DEC-003 | 2026-09-01 | Usar IA para generar informes y avances desde la documentación del repositorio | Equipo | Agilizar redacción de entregables | MTG-001 | Acordada |
| DEC-004 | 2026-09-01 | Frontend web tipo dashboard (Next.js vs Flutter) | Anderson / Omar | Solo dashboard, reportes y recomendaciones | MTG-001 | Pendiente (elegir) |
| DEC-005 | 2026-09-01 | Backend como API Gateway en Go | Equipo | Facilidad de despliegue (binario único, dockerizable) | MTG-001 | Acordada |
| DEC-006 | 2026-09-01 | Módulo ML en servicio independiente; arquitectura de 2 microservicios | Jesús | Evitar bloqueos y competencia por recursos con el backend | MTG-001 | Acordada |
| DEC-007 | 2026-09-01 | Capa de abstracción con modelo de datos común y conectores por proveedor | Equipo | Preparar arquitectura para Azure/AWS/GCP | MTG-001 | Acordada |
| DEC-008 | 2026-09-01 | PostgreSQL como base de datos | Equipo | - | MTG-001 | Acordada |
| DEC-009 | 2026-09-01 | Reportes: guardar resultado en BD y generar PDF en tiempo de ejecución | Equipo | Evitar almacenar archivos | MTG-001 | Acordada (dónde: pendiente) |
| DEC-010 | 2026-09-01 | Sin storage tipo bucket; archivos/modelos vía GitHub | Equipo | - | MTG-001 | Acordada |
| DEC-011 | 2026-09-01 | Autenticación con proveedor externo (Supabase u otro) | Equipo | No desarrollar auth desde cero | MTG-001 | Acordada (proveedor a definir) |
| DEC-012 | 2026-09-01 | El usuario provee su propia API key del proveedor cloud | Equipo | Habilitar monitoreo de sus recursos | MTG-001 | Acordada |
| DEC-013 | 2026-09-01 | Laboratorios con Terraform; evaluar S3 y CloudFront | Jesús | Generar datos sintéticos e infraestructura demostrable | MTG-001 | Acordada |
| DEC-014 | 2026-09-01 | Chat/asistente reutiliza el motor de explicaciones; sin chat con memoria | Equipo | Evitar construir chat con memoria propia | MTG-001 | Acordada |
| DEC-015 | 2026-09-01 | Notificaciones descartadas para este alcance | Equipo | Complejidad; posible investigación futura | MTG-001 | Acordada |
| DEC-016 | 2026-09-01 | Scrum; PRs aprobados por al menos 2 revisores | Equipo | Revisión compartida y trazable | MTG-001 | Acordada |
| DEC-017 | 2026-09-01 | Reuniones semanales los domingos 10:00 a.m. | Equipo | Retrospectiva semanal | MTG-001 | Acordada |
| DEC-018 | 2026-09-01 | Prioridades MVP: backend (auth, gestión BD, orquestación, recomendaciones) y ML (forecasting, capacidad, riesgo) | Equipo | Enfocar esfuerzos del primer periodo | MTG-001 | Acordada |
| DEC-019 | 2026-09-01 | Reentrenamiento del modelo solo con cantidad mínima de datos nuevos | Jesús | Evitar consumo innecesario de recursos | MTG-001 | Acordada |
