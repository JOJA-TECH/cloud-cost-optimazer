# Manual de documentación y convenciones del proyecto

## 1. Propósito

Este manual establece cómo el equipo registra, organiza, relaciona y mantiene la documentación del proyecto.

La documentación no es un producto separado del desarrollo: es el mecanismo que permite reconstruir qué se hizo, por qué se hizo, con qué evidencia y qué resultado produjo.

---

# 2. Principios obligatorios

## 2.1 Trazabilidad

Toda decisión, experimento, requisito o resultado importante debe poder conectarse con otros artefactos.

Ejemplo:

`Problema → Objetivo → REQ → ADR/DEC → Implementación → TEST/EXP → Resultado`

## 2.2 Reproducibilidad

Un laboratorio o experimento debe contener suficiente información para que otra persona pueda repetirlo.

## 2.3 Evidencia antes que opinión

No registrar "funciona mejor" sin indicar cómo se midió.

Preferir:

> "El modelo B obtuvo MAE=0.143 frente a 0.167 del modelo A en DATA-003 v1.2."

## 2.4 Historia inmutable

No se debe editar la historia para ocultar decisiones anteriores. Si una decisión cambia, se registra una nueva decisión y se referencia la anterior.

## 2.5 Documentar al momento

No esperar al final del semestre para reconstruir reuniones o experimentos desde memoria.

## 2.6 Una fuente de verdad

El documento académico resume el trabajo. Los registros técnicos son la evidencia primaria.

---

# 3. Cuándo documentar

| Evento | Documento |
|---|---|
| Se realiza una reunión | MTG |
| Se toma una decisión general | DEC |
| Se toma una decisión arquitectónica | ADR |
| Aparece un problema | ISS |
| Se modifica algo importante | CHG |
| Aparece un riesgo | RSK |
| Se investiga una pregunta | RES |
| Se crea/modifica un requisito | REQ |
| Se ejecuta un laboratorio | LAB |
| Se ejecuta un experimento controlado | EXP |
| Se mide rendimiento | BEN |
| Se crea/modifica dataset | DATA |
| Se crea/evalúa modelo IA | MOD |
| Se registra configuración de IA reproducible | PROMPT |
| Se ejecuta una prueba | TEST |
| Se obtiene un resultado importante | RESU |
| Se publica una versión | REL |

---

# 4. Convención de identificadores

Formato:

`TIPO-NNN`

Ejemplos:

- REQ-001
- ADR-001
- DEC-001
- MTG-001
- ISS-001
- CHG-001
- RSK-001
- RES-001
- LAB-001
- EXP-001
- BEN-001
- DATA-001
- MOD-001
- TEST-001
- REL-001
- RESU-001

El número es secuencial y nunca se reutiliza.

No usar:

- `nuevo_experimento.md`
- `decision_final.md`
- `test_final_v2.md`

Preferir:

- `EXP-014-model-comparison.md`
- `ADR-006-database-selection.md`
- `TEST-021-cost-api.md`

---

# 5. Estados

Usar únicamente estados conocidos.

## Documentos

`Borrador → En revisión → Aprobado → Obsoleto`

## Decisiones

`Propuesta → Aceptada / Rechazada → Reemplazada`

## Requisitos

`Propuesto → Aprobado → Implementado → Verificado → Obsoleto`

## Experimentos

`Planificado → Ejecutado → Analizado → Cerrado`

## Issues

`Abierto → En progreso → Resuelto → Cerrado`

---

# 6. Convención de nombres de archivos

Formato recomendado:

`ID-titulo-en-kebab-case.md`

Ejemplos:

`MTG-014-revision-arquitectura.md`

`ADR-005-seleccion-base-datos.md`

`EXP-012-comparacion-modelos.md`

`DATA-003-cost-history-v1.md`

No incluir fechas en el nombre si el ID ya identifica el documento, salvo que sea útil para una serie temporal.

---

# 7. Reglas para reuniones

Antes:
1. Crear objetivo.
2. Definir agenda.

Durante:
1. Registrar decisiones.
2. Registrar responsables.
3. Registrar bloqueos.

Después:
1. Guardar acta.
2. Crear DEC o ADR cuando corresponda.
3. Crear tareas/issues.
4. Enlazar documentos.

Nunca confiar únicamente en el chat de WhatsApp/Discord/Teams como registro oficial.

---

# 8. Reglas para decisiones

Una decisión menor puede quedar en el Decision Log.

Una decisión con impacto arquitectónico, tecnológico, de datos, seguridad, metodología o costo debe tener ADR.

Preguntas mínimas:

1. ¿Cuál era el problema?
2. ¿Qué alternativas existían?
3. ¿Qué criterios usamos?
4. ¿Qué elegimos?
5. ¿Por qué?
6. ¿Qué consecuencias tiene?
7. ¿Qué evidencia respalda la decisión?

---

# 9. Reglas para experimentos

Antes de ejecutar:
- Definir pregunta.
- Definir hipótesis.
- Definir variables.
- Definir dataset.
- Definir métricas.
- Definir entorno.

Después:
- Registrar resultados reales.
- No reemplazar resultados negativos.
- Registrar fallos.
- Registrar configuración.
- Guardar artefactos.
- Concluir con base en evidencia.

Un resultado negativo es un resultado válido.

---

# 10. Reglas para datasets

Nunca reemplazar silenciosamente un dataset.

Cada modificación significativa genera una nueva versión.

Ejemplo:

`DATA-003 v1.0 → v1.1 → v2.0`

Registrar:
- origen;
- fecha;
- licencia;
- tamaño;
- variables;
- transformaciones;
- problemas;
- hash cuando sea posible;
- experimentos que lo utilizaron.

No guardar secretos, credenciales, tokens ni datos personales innecesarios en el repositorio.

---

# 11. Reglas para IA

Para cada modelo importante:
- registrar versión;
- datos utilizados;
- configuración;
- métricas;
- limitaciones;
- hardware;
- costo/recursos cuando aplique.

Para experimentos con prompts o configuración:
- registrar modelo;
- versión;
- configuración;
- entrada de prueba;
- salida relevante;
- criterio de evaluación.

No presentar una salida generada por IA como evidencia de investigación sin validación independiente.

---

# 12. Reglas para pruebas

Toda prueba importante debe apuntar a un requisito.

Ejemplo:

`REQ-014 → TEST-028`

El reporte debe conservar:
- entorno;
- pasos;
- resultado esperado;
- resultado obtenido;
- evidencia;
- incidencias.

---

# 13. Regla de trazabilidad

Cuando creen un documento, pregúntense:

> "¿De dónde salió esto?"

Y:

> "¿Qué produjo esto?"

Ejemplo:

`RES-004 → ADR-003 → CHG-011 → TEST-019`

Si no existe relación, puede estar faltando documentación.

---

# 14. Git y documentación

Se recomienda que código y documentación evolucionen juntos.

Commit recomendado:

`docs(ADR): documenta selección de PostgreSQL`

`docs(EXP): registra comparación de modelos`

`docs(MTG): agrega reunión 014`

`docs(REQ): actualiza requisitos de rendimiento`

Evitar commits como:

`cosas`
`cambios`
`final`
`final_final`
`ahora_si_final`

---

# 15. Pull Requests / revisiones

Toda documentación importante debería ser revisada por al menos otra persona cuando sea posible.

Checklist:

- [ ] Tiene ID.
- [ ] Tiene fecha.
- [ ] Tiene responsable.
- [ ] Tiene estado.
- [ ] Tiene evidencia.
- [ ] Tiene enlaces a documentos relacionados.
- [ ] No contiene secretos.
- [ ] Es reproducible cuando corresponde.
- [ ] La conclusión coincide con los datos.
- [ ] No elimina historia previa.

---

# 16. Regla para ti como responsable de documentación

Tu función no debería ser escribir todo lo que hacen los demás.

Tu función es mantener el **sistema de trazabilidad**.

Cada integrante debe producir el registro primario de su trabajo.

Tú debes:
- revisar;
- normalizar;
- asignar IDs;
- detectar huecos;
- mantener índices;
- verificar trazabilidad;
- consolidar la documentación académica.

---

# 17. Flujo semanal recomendado

### Inicio de semana

Revisar:
- requisitos;
- tareas;
- riesgos;
- experimentos planificados.

### Durante la semana

Registrar:
- reuniones;
- decisiones;
- issues;
- experimentos;
- cambios.

### Fin de semana/sprint

Revisar:
- resultados;
- pruebas;
- documentación pendiente;
- riesgos;
- trazabilidad.

### Cierre de sprint

Crear retrospectiva.

---

# 18. Definition of Done documental

Una tarea importante no está completamente terminada hasta que:

- [ ] El código/artefacto existe.
- [ ] Se registró el cambio.
- [ ] Las pruebas están documentadas.
- [ ] Los resultados están registrados.
- [ ] Los requisitos relacionados están actualizados.
- [ ] Los ADR afectados están actualizados.
- [ ] Los riesgos fueron reevaluados.
- [ ] La matriz de trazabilidad fue actualizada.

---

# 19. Jerarquía de evidencia

Priorizar:

1. Datos/resultados medidos.
2. Logs y artefactos reproducibles.
3. Pruebas automatizadas.
4. Documentación técnica.
5. Fuentes académicas/oficiales.
6. Actas y decisiones.
7. Opiniones del equipo.

Una opinión puede iniciar una investigación, pero no debería cerrar una discusión técnica importante.

---

# 20. Regla especial para el proyecto académico

La documentación debe mantener coherencia con la formulación de Proyecto de Ingeniería I:

`Problema → Evidencia → Objetivos → Solución → Metodología → Validación → Resultados`

Los documentos técnicos funcionan como evidencia detrás de esa cadena.

No duplicar innecesariamente el contenido: enlazarlo.

---

# 21. Mantenimiento

Responsable general de documentación:
**[Nombre / rol]**

Responsables de documentación primaria:

| Área | Responsable |
|---|---|
| Gobernanza | |
| Backend / arquitectura | |
| IA / datos | |
| Frontend / UX | |
| QA / validación | |

La responsabilidad de documentación no elimina la responsabilidad individual de registrar el trabajo realizado.

---

# 22. Regla final

Si dentro de seis meses alguien pregunta:

> "¿Por qué hicieron esto?"

el repositorio debería poder responder.

Si pregunta:

> "¿Cómo demostraron que funciona?"

debería existir una prueba, experimento o evidencia.

Si pregunta:

> "¿Qué pasó antes de esto?"

debería poder reconstruirse mediante los IDs y Git.

Ese es el estándar de documentación que debe perseguir el equipo.
