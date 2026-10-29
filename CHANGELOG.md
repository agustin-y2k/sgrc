# Historial de cambios

Las versiones del sistema siguen la numeración mayor, menor y parche. Las
notas de cada versión describen los cambios respecto de la versión anterior.

## 1.22.0 — 2026-10-28

- Guías de administración y docencia con capturas renovadas sobre datos ficticios.
  Flechas vectoriales, recuadros y números con indicaciones breves en las imágenes;
  los objetivos se miden en los controles reales y se validan al regenerar los PDF.
  Incluyen clases breves simultáneas y la configuración de plazos y avisos;
  aclaran la liberación por equipo, las entregas registradas y los recordatorios
  cuando el mostrador no está atendido.

- Calendario semanal legible con clases simultáneas y breves: ancho por coincidencias,
  tarjetas resumidas y espacio suficiente sin tapar la clase siguiente. El detalle
  conserva curso, equipos y responsable; los horarios aclaran la duración real.
- Acciones de Nueva reserva con un área táctil mínima de 44 × 44 px, incluida la
  comprobación de series y los controles que aparecen al seleccionar equipos.
  Conserva el tamaño habitual con un puntero preciso en escritorio.

- Datos de integración de reservas y reportes coherentes con el año lectivo.
  Las materias que compiten en una fecha comparten ciclo, los escenarios de años
  distintos conservan su separación y las series máximas caben en un solo año.
  Relojes fijos y errores de preparación comprobados, sin alterar la validación
  anual de la aplicación ni de PostgreSQL.

- Túnel de Cloudflare opcional mediante el perfil `tunel`, con reinicio automático
  `unless-stopped`. `make run-prod-tunel` lo habilita; el arranque habitual y el
  despliegue con túnel externo no lo crean. Los comandos de parada, reinicio y
  consulta abarcan los perfiles del proyecto. Se unifica el perfil local y se
  conservan la reconexión del túnel compartido y el aislamiento de credenciales.

- Renumeración administrativa de computadoras de carro con motivo, control de
  duplicados y protección ante formularios desactualizados. Conserva ID, serie,
  reservas, préstamos y prioridades; las consultas muestran el rótulo actual.
- Calendario de reservas para docentes y administradores, con semana, mes y
  agenda, navegación por fechas y consulta de otras materias en modo de lectura.
  Agrupa equipos por clase e incluye reservas nocturnas, bloqueos y cancelaciones.

- Entregas vinculadas al equipo confirmado por el administrador: rechaza selecciones
  que cambiaron, conserva el equipo de reservas que ya tuvieron préstamos y permite
  retirar únicamente durante el día de la clase, antes de su fin exclusivo.
- Historial académico del préstamo conservado aunque se renombre una materia o se
  cierre el ciclo. Los datos anteriores se reconstruyen sólo cuando existe el vínculo.
- Revisión del impacto de editar un curso: permite conservar prioridades exactas o
  aceptar el nuevo alcance, con control de cambios concurrentes y reglas compartidas.
- Las prioridades operativas excluyen materias, cursos, espacios y ciclos archivados;
  el inventario distingue las marcas que sólo coinciden con datos archivados.
- Las reservas que ya tuvieron una entrega quedan protegidas de la toma por
  prioridad y de los desplazamientos secundarios del autorrelleno, incluso tras
  devolver el equipo. La protección corresponde a esa ocurrencia y se comprueba
  también ante entregas concurrentes; las fechas futuras sin entregas siguen
  participando de las prioridades.
- Entorno explícito por servicio en los Compose: el túnel recibe sólo su token;
  backend, PostgreSQL, Grafana y la carga de desarrollo reciben sus variables
  propias. Se evita propagar claves ajenas o variables nuevas por heredar un
  archivo completo. Incluye las plantillas locales y pruebas de configuración
  con valores ficticios, sin modificar las credenciales de la instalación.
- Levantamiento de bloqueos completo, transaccional e idempotente por operación.
  Conserva cancelaciones anteriores, permisos y motivos, y confirma la auditoría
  y los avisos junto con el lote. La interfaz refresca también ante fallos y
  respuestas perdidas. Las pruebas esperan la finalización real de la cancelación.
- Reportes y nuevos cierres conservan las reservas con retiro tardío efectivo.
  El resumen de actividad distingue reservas de préstamos con y sin reserva,
  devoluciones y pendientes. La procedencia antigua no demostrable se presenta
  explícitamente, sin asociar movimientos por coincidencia de docente o equipo.
- Configuración operativa para administradores: plazos de retiro, recordatorios
  y cierre, y horarios de avisos, persistentes y auditados sin reiniciar.
  Control de versión para impedir sobrescrituras entre administradores.
- Migración **035** para procedencia de préstamos, resumen de actividad de nuevos
  cierres y configuración institucional. Los históricos sellados no se recalculan.
- Retiro anticipado protegido ante otras reservas confirmadas y bloqueos de la
  misma PC: el mostrador explica el horario que impide adelantarlo y conserva
  la reserva posterior, también al completar un retiro parcial de otros equipos.
  La comprobación y la entrega se coordinan con las reservas concurrentes.
- Protección de la materia interna de cada espacio: se gestiona desde el espacio y
  conserva su identidad al renombrarlo y al copiar la estructura anual.
- Identidad por operación de bloqueo para conservar motivos, equipos y cancelaciones
  independientes, también al paginar. Los bloqueos anteriores se identifican por fila.
- Reservas puntuales y series limitadas al año de su ciclo lectivo, con validación
  en API, formularios y PostgreSQL. El fin exclusivo puede ser la medianoche que
  cierra el 31 de diciembre; no puede ocupar horas del año siguiente.
- Navegación desde reservas canceladas al listado con su filtro activo. Controles
  táctiles de al menos 44 × 44 px en el calendario móvil, incluidos filtros y detalles;
  las tarjetas cortas o simultáneas conservan separación y superficie alcanzable.
- Migraciones **030 a 034** y ampliación de las pruebas de integridad y navegador.

## 1.21.0 — 2026-10-21

### Reservas y gestión académica

- Prioridades por dispositivo con toma explícita de equipos ocupados y
  reposición de las reservas desplazadas en cascada. La previsualización de
  una serie revisa todas sus fechas antes de confirmar y solicita consentimiento
  si alguna toma equipos por prioridad.
- Preferencias vinculadas al curso y materia concretos, con selección
  relacionada de modalidad, año y división para evitar combinaciones inválidas.
- Cierre anual recuperable y coordinación de escrituras concurrentes sobre
  asignaciones, reservas, préstamos e inventario.
- La desvinculación de un docente conserva las reservas mientras existan otras
  cuentas aprobadas asignadas a esa materia y curso.
- Entregas, devoluciones, préstamos sin reserva, bloqueos y reservas recurrentes
  con validaciones de vigencia, fechas y conflictos.

### Seguridad y consistencia

- Revocación de sesiones y protección frente a cambios concurrentes de cuenta,
  contraseña, rol, perfil y estado. Recuperación de cuentas serializada.
- Protección de contraseñas de equipos y revisión de permisos y validación HTTP.
- Eventos durables y auditoría transaccional para evitar que un cambio confirmado
  pierda sus avisos o su registro de auditoría.
- Compilación con Go 1.26.10, incluido el parche de `crypto/x509` correspondiente
  a GO-2026-6809.

### Interfaz y documentación

- Navegación por rol y formularios con errores recuperables, estados de carga,
  borradores y mejoras de accesibilidad.
- Auditoría presentada con descripciones legibles; mejoras en inventario,
  licencias, soporte, notificaciones y reportes.
- Contrato OpenAPI y manuales técnicos actualizados. Guías de administración y
  docencia con 56 capturas de datos ficticios y generación reproducible de PDF.
- Retiro de la galería web y los generadores obsoletos de capturas.

### Instalación y mantenimiento

- Plantillas públicas de entorno y Docker Compose separadas de las
  configuraciones y datos privados de cada instalación.
- Inicialización de una instalación local independiente, claves aleatorias y
  cuentas de prueba; mejoras de respaldos y arranque seguro de Grafana.
- Ruta del módulo Go alineada con `github.com/agustin-y2k/sgrc`.
- CI para integración, releases y hotfixes: compilación, pruebas, lint,
  vulnerabilidades, integración con PostgreSQL e imágenes Docker. Los PR hacia
  `main` ejecutan los mismos seis controles.

### Actualización desde 1.20.0

- Una base del tag `v1.20.0` incorpora las migraciones **010 a 029**. La aplicación
  ejecuta al arrancar las que falten según el registro de Goose; no se deben
  reaplicar manualmente migraciones ya registradas.
- Conservar un respaldo de la base y las claves de cifrado correspondientes
  antes de actualizar. Comparar las variables privadas con `.env.example`,
  conservando los valores propios y sin reemplazarlos por ejemplos.
- Mantener las claves `CUENTAS_SECRET` y `OUTBOX_SECRET` que correspondan a los
  datos cifrados y a los eventos pendientes. El procedimiento se detalla en
  [Operación](docs/11-operacion.md).
- El retroceso debe contemplar la restauración compatible de la base; cambiar
  sólo la imagen no revierte las migraciones. Revisar el procedimiento de
  respaldo y restauración antes del despliegue.
- Para construir desde el código se utiliza Go 1.26.10 y Node 24; los Dockerfile
  incluyen las versiones correspondientes.
