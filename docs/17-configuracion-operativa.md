# Configuración operativa

En **Administración → Configuración del sistema**, cualquier cuenta `ADMIN` puede
ajustar los plazos del trabajo diario sin editar archivos ni reiniciar el backend.
Los valores usan la zona horaria de la instalación (`APP_TIMEZONE`).

| Ajuste | Valor inicial | Límites | Efecto |
|---|---|---|---|
| Espera para retirar | 40 minutos | 1–1440 | Desde el inicio de la clase, permite liberar equipos sin retiro registrado cuando hay mostrador atendido. |
| Antelación del recordatorio | 60 minutos | 1–1440 | Prepara el recordatorio antes de la reserva; el correo respeta las preferencias del docente. |
| Espera después del cierre | 60 minutos | 0–720 | Con jornada declarada, retrasa el aviso por equipos que siguen afuera. Cero permite avisar desde el cierre. |
| Hora del aviso sin jornada | 23 | 0–23 | Alternativa cuando la institución no declaró horarios. Esta hora ya es la del aviso. |
| Hora del aviso de licencias | 7 | 0–23 | El barrido de licencias puede avisar desde esa hora local. |

Los valores iniciales de retiro, hora alternativa y hora de licencias se toman de
`RETIRO_GRACIA_MINUTOS`, `CIERRE_JORNADA` y `LICENCIAS_HORA_AVISO` sólo al crear la
configuración. Después prevalecen los guardados desde la interfaz, también al
reiniciar. Los recordatorios y la espera posterior al cierre comienzan en 60 min.

El guardado es completo, valida todos los campos y exige la versión consultada.
Si otro administrador guardó primero, se responde conflicto y se ofrece recargar
la configuración vigente. La escritura y su auditoría se confirman juntas; si falla
la auditoría, no se aplica el cambio. La auditoría presenta cada plazo anterior y
nuevo con nombres legibles.

## Cómo se aplica `NO_RETIRADA`

La liberación automática cuenta el plazo desde el inicio de la clase. Se ejecuta
sólo mientras la franja sigue vigente, con mostrador atendido, sin una entrega
vinculada previa y sin préstamo abierto de la PC. Si la clase dura menos que el
plazo, no se libera por este mecanismo. Una entrega devuelta también cuenta como
retiro registrado y protege esa reserva.

El estado puede surgir además de una entrega parcial declarada por el administrador:
lo que decidió no llevarse queda libre en ese momento. Los equipos impedidos por
una reserva intermedia al intentar adelantar el retiro conservan su reserva.

Los cambios de plazo se aplican a las siguientes comprobaciones y a las reservas
pendientes. Reducirlo puede liberar reservas que ya superaron el nuevo plazo;
aumentarlo no recupera las liberadas. No se reenvían avisos ya registrados.
El barrido de reservas corre cada cinco minutos, por lo que la acción puede ocurrir
en la pasada siguiente. Las revisiones de licencias usan el intervalo técnico de
la instalación (`LICENCIAS_INTERVALO_MINUTOS`, 60 por defecto).

## Otros tiempos del sistema

- La jornada escolar y los horarios de atención ya se editan en sus pantallas.
- Duración, fecha de vencimiento y días de aviso se editan en cada licencia.
- La devolución prevista se toma del fin de la reserva; en entregas sin reserva
  puede anotarse una hora opcional. Una demora no equivale a una devolución.
- El fin de una reserva, el rango de una recurrencia y el año lectivo pertenecen
  a la planificación académica, no a un contador global.
- La duración de sesión normal y con «Recordarme» depende de `JWT_ACCESS_TTL` y
  `JWT_REMEMBER_TTL` de la instalación (24 h y 30 días por defecto). El código de
  recuperación dura 15 minutos. Son límites de seguridad; esta pantalla gestiona
  los plazos operativos. Los intervalos de tareas, reintentos, límites de peticiones
  y tiempos de conexión pertenecen a la infraestructura.

## Reportes de actividad

**Reservas y entregas efectivas**, en Reportes, separa la planificación de los
movimientos reales. Una entrega explícitamente vinculada a una `NO_RETIRADA` aún
vigente cuenta esa reserva como atendida sin reabrir su estado. Varias entregas
contra la misma reserva cuentan una reserva atendida y varios movimientos.

Un préstamo sin reserva al mismo docente sigue siendo independiente. Se conservan
sus datos y, si tiene cuenta asociada, se agrupa por esa cuenta. Los nombres iguales
sin identidad de cuenta no se fusionan. Los préstamos antiguos sin procedencia
recuperable aparecen como «origen no determinado»; no se infiere una reserva por
nombre, fecha, destino o equipo.

Los nuevos cierres conservan el resumen completo y los pendientes en ese momento.
Los históricos sellados anteriores no se reescriben ni se reconstruyen desde fuentes
que pudieron borrarse. El reporte indica cuando ese detalle todavía no existía.
