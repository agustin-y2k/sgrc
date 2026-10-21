# Historial de cambios

Las versiones del sistema siguen la numeración mayor, menor y parche. Las
notas de cada versión describen los cambios respecto de la versión anterior.

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
