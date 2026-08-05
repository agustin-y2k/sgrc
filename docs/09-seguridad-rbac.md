# Seguridad y Control de Acceso — SGRC

## 1. Autenticación
- Passwords con hash `argon2id` (resistente a ataques GPU).
- JWT firmados **`HS256`** (secreto simétrico) — un solo proceso firma y verifica, así que un secreto simétrico cumple la función sin la gestión de un par de claves asimétricas (ver `06-arquitectura.md` §7).
- Access token: 1h (`JWT_ACCESS_TTL`). **No hay refresh token**: cuando el access expira se vuelve a iniciar sesión. Para una jornada escolar, renovar la sesión una vez al día es aceptable, y evita el segundo token con su propio almacenamiento, su rotación y su revocación.
- Login en un solo paso, con email y contraseña.
- **Ingreso con cuenta de Google (opcional).** Habilitado solo si el despliegue configura `GOOGLE_CLIENT_ID`; sin eso, los endpoints responden 503 y el frontend no dibuja el botón. Ver §1.1.
- **Una baja tiene efecto inmediato.** El token sigue siendo la prueba de identidad, pero no alcanza por sí solo: cada request autenticado consulta el estado de la cuenta antes de dejar pasar. Si el usuario ya no existe, no está `APROBADA`, o cambió de rol, el request se rechaza aunque el token siga sin expirar.

  Antes esto era al revés y estaba documentado como una decisión consciente: el JWT era stateless y una cuenta dada de baja conservaba acceso de escritura hasta una hora. La ventana era real —se verificó con un token emitido antes de la baja escribiendo en la base después— y RF-02.8/02.9 tratan la baja como efectiva de inmediato, así que la decisión se revirtió.

  El costo es una consulta por PK por request autenticado, irrelevante a esta escala. Ante un error de base **falla cerrado** (503, no "pasá igual"), y el rol que vale es el de la base, no el del token: no hay forma de conservar permisos viejos guardándose un token.

- **El secreto y la verificación de cuenta viajan juntos.** El middleware se construye con los dos a la vez y `RegisterRoutes` de cada paquete recibe ese valor, no el secreto pelado. Es deliberado: pasarlos por separado permitiría montar una ruta que valide la firma y se saltee el estado de la cuenta, que es exactamente el agujero que esto cierra.

- **Qué se calla y qué se dice, y por qué no es lo mismo.** Antes de verificar la credencial, el sistema es deliberadamente opaco: un email inexistente y uno real con la contraseña equivocada devuelven el mismo error y consumen el mismo tiempo (ver el punto siguiente). **Después** de verificarla, deja de haber motivo para esconder nada: quien presentó la contraseña correcta —o un ID token de Google firmado— ya probó que la cuenta es suya, así que se le dice exactamente por qué no puede entrar: pendiente de aprobación, rechazada o dada de baja. Los tres son 403; lo que cambia es la explicación, no el veredicto.

  Antes los tres devolvían "cuenta no habilitada". Quien se acababa de registrar y quien había sido rechazado leían lo mismo, y ninguno de los dos sabía si tenía que esperar, insistir o hablar con alguien.

- **El login tarda lo mismo exista o no la cuenta.** Con un email inexistente se devolvía sin hashear nada, así que medir el tiempo de respuesta alcanzaba para enumerar quién tiene cuenta en la escuela — el mensaje de error era el mismo, pero el reloj no. Ahora ese camino corre un `argon2id` contra un hash de descarte que no le pertenece a nadie. El hash se calcula una sola vez por proceso: recalcularlo en cada intento habría igualado los tiempos, pero convertiría un endpoint sin autenticar en una forma de gastar 64 MB por request.

## 1.1 Ingreso con cuenta de Google

Se usa el **flujo de ID token**: el navegador obtiene de Google un JWT firmado, lo manda a `POST /api/auth/google`, y el backend lo verifica y emite el token de siempre. No hay redirects, no hay estado de sesión OAuth, y **no interviene ningún client secret** — el client ID es público. A partir de la respuesta, la sesión es idéntica a la de un login con contraseña: todo el RBAC de §3 sigue funcionando sin cambios porque el token que circula es el nuestro.

**Qué se verifica en cada ID token** (`internal/auth/infrastructure/google_idtoken.go`):

| Chequeo | Por qué |
|---|---|
| Firma RS256 contra las claves públicas vigentes de Google | Es lo que hace que el token no se pueda inventar |
| Algoritmo fijado a `RS256` por lista blanca | Sin eso, un token con `alg: none` —o un HMAC firmado con la propia clave pública, que es pública— sería aceptado. Es la familia de bugs clásica de JWT |
| `aud` igual a **nuestro** client ID | Google le firma ID tokens a cualquiera con una app registrada. Sin este chequeo, alguien podría presentar un token legítimo emitido para su propia aplicación y entrar como quien quisiera. Es el chequeo más importante de todos |
| `iss` de Google y `exp` vigente | Un token sin `exp` no caduca nunca: se rechaza |
| `email_verified` en `true` | Sin esa garantía, cualquiera que pueda escribir una dirección ajena en su perfil de Google entraría a la cuenta de esa persona |
| Dominio del email en `GOOGLE_DOMINIOS_PERMITIDOS`, si está configurado | No es un control de acceso (la aprobación del Admin lo es): evita que el Admin tenga que revisar solicitudes de cualquier persona de internet |

Las claves públicas se cachean respetando el `max-age` que declara Google, acotado entre 5 minutos y 12 horas: un cache demasiado largo dejaría todos los logins fallando después de una rotación de claves, y uno demasiado corto convertiría cada login en un pedido a Google. Un token con un `kid` desconocido provoca **un solo** refresco de claves y después se rechaza, para que un token basura no sea un amplificador de tráfico contra Google.

**Una falla de red al buscar las claves no se reporta como token inválido.** Devuelve 500, no 401: culpar al usuario por un problema nuestro deja el incidente sin rastro de la causa.

**Vincular por email es seguro solo porque antes se exigió `email_verified`.** Un docente que ya tenía cuenta con contraseña y entra con Google queda vinculado a su misma cuenta y **conserva la contraseña** — las dos formas de ingreso conviven (`migrations/008_login_con_google.sql`). Una cuenta en `BAJA` no se vincula: RF-02.9 la hace terminal y no se reactiva por la puerta de atrás.

**Una cuenta creada con Google queda `PENDIENTE` igual que cualquier otra.** Tener una cuenta de Google válida prueba quién sos, no que la escuela te conozca.

**El login con contraseña no revela que una cuenta entra con Google.** Una cuenta sin `password_hash` recibe exactamente el mismo error y consume exactamente el mismo tiempo que un email inexistente (mismo argumento que el párrafo anterior de §1). Decir "esta cuenta entra con Google" sería más amable, pero convertiría el endpoint en un oráculo de qué direcciones tienen cuenta en la escuela y con qué la abrieron. La pantalla de login tiene el botón de Google al lado, que es donde esa persona encuentra la salida.

**Es lo único que carga código de un tercero.** La biblioteca de Google (`accounts.google.com/gsi/client`) no se puede empaquetar en el bundle: la URL es parte del contrato, porque el script se comunica con esa misma página, y el botón se dibuja en un iframe de ese origen. La CSP del HTML lo habilita explícitamente en `script-src`, `frame-src` y `connect-src` (ver §4) — si eso se quita, el botón deja de aparecer sin más síntoma que su ausencia.

**`POST /api/auth/cambiar-password` sí lo dice explícitamente** (409): quien llega ahí ya está autenticado y es dueño de la cuenta, así que no hay nada que revelarle sobre sí mismo. Un Admin puede darle una contraseña con `reset-password` sin romper el vínculo con Google — es la forma de devolverle el acceso a alguien que perdió su cuenta de Google.

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
| Promover un docente a Admin | ✅ | ❌ |
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
| Rate limiting | `/api/auth/login`: 30/min por IP **y** 10/min por cuenta. `/api/auth/registro`: 5/min por IP. `/api/auth/google`: 30/min por IP; `/api/auth/google/registro`: 5/min por IP |
| IP real del cliente | `CF-Connecting-IP`, aceptado solo desde `TRUSTED_PROXIES` |
| Password temporal | La API responde 403 mientras `debe_cambiar_password` siga en `true` |
| Headers | `HSTS`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, `CSP` restrictiva. En **dos** lugares: el binario Go los pone en `/api` y nginx en el HTML y los assets (ver abajo) |
| CORS | Solo dominio del frontend, sin wildcard |
| Validación | Estricta en cada handler; nunca se confía en el frontend |
| Secrets | `.env` fuera de git + Docker secrets. Secreto JWT nunca en el repo |
| Permisos DB | Un usuario Postgres de aplicación con GRANT sobre `sgrc_db`, sin permisos de `SUPERUSER` |

### Por qué se puede promover pero no degradar

Hay dos formas de que exista un Admin: que otro Admin lo cree directo (`POST /api/auth/admins`, RF-01.4) o que promueva a un docente ya aprobado (`POST /api/auth/usuarios/{id}/promover-a-admin`). Las dos las tiene que iniciar un Admin; ninguna es alcanzable desde afuera. El autorregistro —con contraseña o con Google— crea siempre rol DOCENTE, porque es un endpoint público sin autenticar y no puede ser una puerta por la que alguien se asigne un rol.

**Degradar un Admin a docente no existe, y es una omisión deliberada.** No es simétrico con promover: habría que decidir qué pasa con el guard del último Admin (RF-01.8), qué materias pasaría a dictar alguien que no tiene ninguna asignada, y si un Admin puede degradar a otro —o a sí mismo— y dejar el sistema en manos de nadie. Mientras nadie lo necesite, no existir es más seguro que existir a medias. Para sacarle los permisos a alguien hoy está la baja (RF-02.8), que sí está pensada de punta a punta.

Promover **solo agrega** Admins, así que no pasa por el guard del último Admin ni necesita transacción: no hay forma de que deje al sistema sin ninguno. Y no toca nada más de la cuenta —conserva materias, reservas y formas de ingreso— porque un docente que pasa a coordinar suele seguir dando clase: `ExisteYAprobado` de academic nunca miró el rol, y reservar tampoco lo pide.

El cambio **tiene efecto en el request siguiente, sin volver a iniciar sesión**, por lo mismo que una baja es inmediata (§1): el middleware lee el rol de la base en cada pedido y pisa el del token. La contracara es que un token viejo no conserva el rol viejo, ni para bien ni para mal.

### Por qué la CSP está en dos lugares y no en uno

Los headers de `internal/shared/middleware/security.go` los pone el binario Go, así que salen únicamente en las respuestas de `/api` — que son JSON, donde una CSP no restringe nada real. **El HTML de la SPA lo sirve nginx**, y durante un tiempo salió sin ninguna CSP: la política existía, pero justo en el único documento que un navegador puede ser engañado de ejecutar no se aplicaba.

Eso lo cubre `frontend/nginx-seguridad.conf`, incluido desde los dos `location` que sirven contenido propio. No se ponen en el bloque `server` por dos razones: nginx deja de heredar los `add_header` en cuanto un bloque define uno propio (y `/assets/` define su `Cache-Control`), y las rutas proxeadas a `/api` quedarían con dos headers `Content-Security-Policy` distintos en la misma respuesta.

Dos decisiones dentro de esa política merecen quedar escritas:

- **`script-src` autoriza el script inline de `index.html` por su hash SHA-256, no con `'unsafe-inline'`.** Ese script aplica el tema antes de pintar; sin él vuelve el fogonazo blanco al recargar en modo oscuro. Un `'unsafe-inline'` habría sido más cómodo pero deja pasar cualquier script que una inyección logre meter en el HTML, que es exactamente lo que la CSP está para impedir. El costo del hash es que se desincroniza en silencio: si alguien edita el script y no actualiza la CSP, el navegador lo bloquea y el único síntoma es el fogonazo, que nadie asocia con un header. Por eso hay un test (`frontend/csp.test.ts`) que recalcula el hash y falla con el valor nuevo listo para pegar.
- **`style-src` sí lleva `'unsafe-inline'`, y es deliberado.** La grilla del calendario posiciona cada bloque con `style={{top, height}}` y las barras de los reportes fijan su ancho igual; sin eso el calendario queda con todos los bloques apilados y sin alto. Una inyección de CSS es de otro orden de gravedad que una de script, que sí queda cerrada.

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
