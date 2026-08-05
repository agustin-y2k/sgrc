# Seguridad y Control de Acceso — SGRC

## 1. Autenticación
- Passwords con hash `argon2id` (resistente a ataques GPU).
- JWT firmados **`HS256`** (secreto simétrico) — un solo proceso firma y verifica, así que un secreto simétrico cumple la función sin la gestión de un par de claves asimétricas (ver `06-arquitectura.md` §7).
- Access token: 1h (`JWT_ACCESS_TTL`). **No hay refresh token**: cuando el access expira se vuelve a iniciar sesión. Para una jornada escolar, renovar la sesión una vez al día es aceptable, y evita el segundo token con su propio almacenamiento, su rotación y su revocación.
- Login en un solo paso, con email y contraseña.
- **Una baja tiene efecto inmediato.** El token sigue siendo la prueba de identidad, pero no alcanza por sí solo: cada request autenticado consulta el estado de la cuenta antes de dejar pasar. Si el usuario ya no existe, no está `APROBADA`, o cambió de rol, el request se rechaza aunque el token siga sin expirar.

  Antes esto era al revés y estaba documentado como una decisión consciente: el JWT era stateless y una cuenta dada de baja conservaba acceso de escritura hasta una hora. La ventana era real —se verificó con un token emitido antes de la baja escribiendo en la base después— y RF-02.8/02.9 tratan la baja como efectiva de inmediato, así que la decisión se revirtió.

  El costo es una consulta por PK por request autenticado, irrelevante a esta escala. Ante un error de base **falla cerrado** (503, no "pasá igual"), y el rol que vale es el de la base, no el del token: no hay forma de conservar permisos viejos guardándose un token.

- **El secreto y la verificación de cuenta viajan juntos.** El middleware se construye con los dos a la vez y `RegisterRoutes` de cada paquete recibe ese valor, no el secreto pelado. Es deliberado: pasarlos por separado permitiría montar una ruta que valide la firma y se saltee el estado de la cuenta, que es exactamente el agujero que esto cierra.

- **El login tarda lo mismo exista o no la cuenta.** Con un email inexistente se devolvía sin hashear nada, así que medir el tiempo de respuesta alcanzaba para enumerar quién tiene cuenta en la escuela — el mensaje de error era el mismo, pero el reloj no. Ahora ese camino corre un `argon2id` contra un hash de descarte que no le pertenece a nadie. El hash se calcula una sola vez por proceso: recalcularlo en cada intento habría igualado los tiempos, pero convertiría un endpoint sin autenticar en una forma de gastar 64 MB por request.

## 2. Estructura del JWT

```json
{ "sub": "userId", "rol": "ADMIN|DOCENTE", "nombre": "...", "apellido": "...", "dcp": true, "exp": ... }
```

`dcp` (`debe_cambiar_password`) viaja en el token para poder exigir el cambio
sin consultar la base en cada request. Solo aparece cuando es `true`. Los
demás campos son para mostrar el nombre en la interfaz sin pedir el perfil;
**ninguno se usa para autorizar**: el rol que decide es el que sale de la
base al verificar la cuenta (§1).

## 3. Matriz RBAC

| Acción | Admin | Docente |
|---|:---:|:---:|
| Crear/aprobar otros Admins | ✅ | ❌ |
| Crear/editar carros | ✅ | ❌ |
| Registrar/editar PCs, dar de baja una PC | ✅ | ❌ |
| Ver inventario (carros/PCs, incl. software instalado y freezado) | ✅ | ✅ |
| Cambiar estado de PC | ✅ | ❌ |
| Registrar incidencia | ✅ | ✅ solo reportar |
| Ver el historial de incidencias de una PC | ✅ | ✅ |
| Cambiar estado de incidencia / marcar envío a DGE | ✅ | ❌ |
| Aprobar cuentas de docentes | ✅ | ❌ |
| Resetear contraseña de un usuario | ✅ | ❌ |
| Dar de baja a un docente (permanente) | ✅ | ❌ |
| Eliminar definitivamente una cuenta en BAJA | ✅ | ❌ |
| Remover docente de una materia puntual | ✅ | ❌ |
| Gestionar ciclos, cursos, materias (crear, editar, eliminar sin reservas) | ✅ | ❌ |
| Archivar y clonar ciclo lectivo | ✅ | ❌ |
| Asignar docentes a materias | ✅ | ❌ |
| Ver calendario de PC | ✅ | ✅ |
| Reservar para cualquier materia | ✅ | ❌ |
| Reservar para materia asignada | ✅ | ✅ solo asignadas |
| Cancelar reserva propia (una PC o el grupo completo) | ✅ | ✅ |
| Cancelar reserva ajena (con motivo) | ✅ | ❌ |
| Ver una reserva puntual (`GET /grupos/{id}`) | ✅ | ✅ solo propias |
| Bloquear PCs para evaluación | ✅ | ❌ |
| Ver reportes (activos e históricos) | ✅ | ❌ |
| Ver notificaciones propias | ✅ | ✅ |
| Configurar mi horario de disponibilidad | ✅ | ❌ |
| Ver disponibilidad de Admins | ✅ | ✅ |

## 4. Controles de infraestructura

| Control | Detalle |
|---|---|
| HTTPS | Cloudflare termina TLS; el túnel cifra hasta el servidor |
| Rate limiting | `/api/auth/login`: 30/min por IP **y** 10/min por cuenta. `/api/auth/registro`: 5/min por IP |
| IP real del cliente | `CF-Connecting-IP`, aceptado solo desde `TRUSTED_PROXIES` |
| Password temporal | La API responde 403 mientras `debe_cambiar_password` siga en `true` |
| Headers | `HSTS`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `CSP` restrictiva |
| CORS | Solo dominio del frontend, sin wildcard |
| Validación | Estricta en cada handler; nunca se confía en el frontend |
| Secrets | `.env` fuera de git + Docker secrets. Secreto JWT nunca en el repo |
| Permisos DB | Un usuario Postgres de aplicación con GRANT sobre `sgrc_db`, sin permisos de `SUPERUSER` |

### Por qué el login se limita también por cuenta

Limitar solo por IP falla en las dos direcciones. Los docentes que entran desde el wifi de la escuela salen todos por la misma IP NAT y se consumen la cuota entre ellos; y quien prueba contraseñas contra una cuenta puntual esquiva el límite cambiando de red. La cuenta atacada es lo único constante, así que se limita por las dos cosas a la vez.

Las ventanas son por minuto y no por segundo: `5 req/s` son 18.000 intentos por hora, que no frena a nadie.

### Por qué hace falta `TRUSTED_PROXIES`

El tráfico llega Cloudflare → `cloudflared` → `nginx` → `sgrc-app`, así que la IP del socket es siempre la de un contenedor. Sin configurar `ProxyHeader`, el rate limiting por IP degrada a un balde único para toda la institución y `audit_log.ip_origen` guarda la IP de nginx en cada fila — presente, pero inútil para saber quién hizo qué.

Se usa `CF-Connecting-IP` y no `X-Forwarded-For` porque Cloudflare la **sobrescribe** siempre con la IP real del cliente: no es falsificable mientras el único camino hasta la app sea el túnel. De ahí que el compose de producción no publique el `8080` al host — ese atajo desde la LAN de la escuela permitiría inventar el header. `TRUSTED_PROXIES` vacío degrada a usar la IP del socket, que es el default correcto: se pierde la IP real, no se gana una falsificable.

### Por qué el cambio de contraseña devuelve un token nuevo

`debe_cambiar_password` viaja dentro del JWT para poder exigirlo sin consultar la base en cada request, pero eso lo deja congelado en el token. `POST /api/auth/cambiar-password` responde con un token nuevo y el cliente tiene que reemplazar el anterior; si no, quien acaba de cambiar la contraseña quedaría bloqueado por su propio cambio exitoso hasta que el token expirara.

Las únicas dos rutas que aceptan un token con la contraseña temporal sin cambiar son `GET /api/auth/me` y `POST /api/auth/cambiar-password` — justamente las que hacen falta para salir de esa situación. Se marcan explícitamente con `JWTAuthPermitiendoPasswordVencida`; todo lo demás usa `JWTAuth`, que ya incluye la restricción, de modo que una ruta nueva queda protegida por omisión.

## 5. Tabla de auditoría

```sql
CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id UUID NOT NULL,
    accion VARCHAR(100) NOT NULL,
    entidad VARCHAR(50) NOT NULL,
    entidad_id UUID,
    detalle JSONB,
    ip_origen INET,
    creado_en TIMESTAMP NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_usuario ON audit_log(usuario_id, creado_en DESC);
```

Acciones auditadas: `CUENTA_APROBADA`, `CUENTA_RECHAZADA`, `CUENTA_BAJA`, `CUENTA_ELIMINADA_DEFINITIVAMENTE`, `ADMIN_CREADO`, `PASSWORD_RESETEADA`, `DOCENTE_REMOVIDO_DE_MATERIA`, `RESERVA_CANCELADA_POR_ADMIN`, `BLOQUEO_EVALUACION_CREADO`, `PC_ESTADO_CAMBIADO`, `PC_DADA_DE_BAJA`, `PC_MOVIDA_DE_CARRO`, `CURSO_ELIMINADO`, `MATERIA_ELIMINADA`, `CICLO_ARCHIVADO_RESERVAS_ELIMINADAS`, `CICLO_CLONADO`.

> `CICLO_ARCHIVADO_RESERVAS_ELIMINADAS` tiene su propio nombre (en vez de un `CICLO_ARCHIVADO` genérico) porque implica un borrado físico de datos — vale la pena que quede explícito en el log qué admin lo disparó y cuántas filas se eliminaron (`detalle` puede guardar el conteo).
