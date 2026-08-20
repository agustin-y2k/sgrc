# Seguridad y Control de Acceso — SGRC

## 1. Autenticación
- Passwords con hash `argon2id` (resistente a ataques GPU).
- JWT firmados **`HS256`** (secreto simétrico) — un solo proceso firma y verifica, así que un secreto simétrico cumple la función sin la gestión de un par de claves asimétricas (ver `06-arquitectura.md` §7).
- Access token: 1h (`JWT_ACCESS_TTL`). **No hay refresh token**: cuando el access expira se vuelve a iniciar sesión. Para una jornada escolar, renovar la sesión una vez al día es aceptable, y evita el segundo token con su propio almacenamiento, su rotación y su revocación.
- Login en un solo paso, con email y contraseña.
- **Ingreso con cuenta de Google (opcional).** Habilitado solo si el despliegue configura `GOOGLE_CLIENT_ID`; sin eso, los endpoints responden 503 y el frontend no dibuja el botón. Ver §1.1.
- **Una baja tiene efecto inmediato.** El token sigue siendo la prueba de identidad, pero no alcanza por sí solo: cada request autenticado consulta el estado de la cuenta antes de dejar pasar. Si el usuario ya no existe, no está `APROBADA`, o cambió de rol, el request se rechaza aunque el token siga sin expirar.

  Un JWT estrictamente stateless sería más barato, pero le dejaría a una cuenta dada de baja hasta una hora de acceso **de escritura**, y RF-02.8/02.9 tratan la baja como efectiva de inmediato. La ventana no es teórica: un token emitido antes de la baja escribe en la base después, mientras no expire.

  El costo es una consulta por PK por request autenticado, irrelevante a esta escala. Ante un error de base **falla cerrado** (503, no "pasá igual"), y el rol que vale es el de la base, no el del token: no hay forma de conservar permisos viejos guardándose un token.

- **Cambiar la contraseña cierra las sesiones abiertas** (RF-01.11). El punto anterior verifica el *estado* de la cuenta, que no cambia al cambiar una contraseña; sin algo más, la sesión de quien hubiera entrado con la contraseña vieja sobreviviría hasta que expire su token — hasta una hora. Eso vaciaría de sentido al caso que motiva cambiarla: alguien sospecha que entraron a su cuenta y quiere cortar ese acceso ya.

  Cada cuenta lleva un contador (`usuario.version_sesion`) que viaja dentro del token como el claim `vs`. El middleware lo compara contra el de la fila en el mismo request en el que ya consulta el estado, así que **no cuesta ninguna consulta extra**. Cambiar la contraseña incrementa el contador y todo token anterior deja de valer en el request siguiente. Los tres caminos lo hacen: el cambio voluntario (RF-01.7), el reset asistido por un Admin (RF-01.6) y la recuperación por autoservicio (RF-01.10).

  El orden importa y está escrito en el código: en `CambiarPassword` se incrementa **antes** de firmar el token nuevo. Al revés, quien acaba de cambiar su contraseña recibiría un token con la versión vieja y quedaría afuera por su propio cambio exitoso.

  Es un entero y no una marca de tiempo ("invalidar todo lo emitido antes de X"). `iat` en un JWT tiene resolución de **segundos**, así que comparado contra un `now()` con microsegundos el token recién firmado se rechaza a sí mismo; redondear al segundo lo arregla pero deja una ventana en la que las sesiones abiertas en ese mismo segundo sobreviven. Con un contador no hay nada que redondear.

  El `DEFAULT 0` de la columna coincide con el claim ausente: un token emitido sin el claim sigue valiendo hasta expirar, y recién el primer cambio de contraseña de esa cuenta invalida los suyos.

  Dar de baja o rechazar una cuenta **no** toca el contador: eso lo resuelve el chequeo de estado, y mezclar las dos cosas daría dos formas de expresar lo mismo.

- **El secreto y la verificación de cuenta viajan juntos.** El middleware se construye con los dos a la vez y `RegisterRoutes` de cada paquete recibe ese valor, no el secreto pelado. Es deliberado: pasarlos por separado permitiría montar una ruta que valide la firma y se saltee el estado de la cuenta, que es exactamente el agujero que esto cierra.

- **Qué se calla y qué se dice, y por qué no es lo mismo.** Antes de verificar la credencial, el sistema es deliberadamente opaco: un email inexistente y uno real con la contraseña equivocada devuelven el mismo error y consumen el mismo tiempo (ver el punto siguiente). **Después** de verificarla, deja de haber motivo para esconder nada: quien presentó la contraseña correcta —o un ID token de Google firmado— ya probó que la cuenta es suya, así que se le dice exactamente por qué no puede entrar: pendiente de aprobación, rechazada o dada de baja. Los tres son 403; lo que cambia es la explicación, no el veredicto.

  Un "cuenta no habilitada" genérico para los tres haría que quien se acaba de registrar y quien fue rechazado leyeran lo mismo, sin saber si tienen que esperar, insistir o hablar con alguien.

- **El login tarda lo mismo exista o no la cuenta.** Devolver sin hashear nada cuando el email no existe deja el mensaje de error igual pero no el reloj, y medir el tiempo de respuesta alcanza para enumerar quién tiene cuenta en la institución. Por eso ese camino corre un `argon2id` contra un hash de descarte que no le pertenece a nadie. El hash se calcula una sola vez por proceso: recalcularlo en cada intento habría igualado los tiempos, pero convertiría un endpoint sin autenticar en una forma de gastar 64 MB por request.

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

**Vincular por email es seguro solo porque antes se exigió `email_verified`.** Un docente que ya tenía cuenta con contraseña y entra con Google queda vinculado a su misma cuenta y **conserva la contraseña** — las dos formas de ingreso conviven (`usuario.password_hash` y `usuario.google_sub`, con el CHECK `chk_usuario_credencial` exigiendo al menos una). Una cuenta en `BAJA` no se vincula: RF-02.9 la hace terminal y no se reactiva por la puerta de atrás.

**Una cuenta creada con Google queda `PENDIENTE` igual que cualquier otra.** Tener una cuenta de Google válida prueba quién sos, no que la institución te conozca.

**El login con contraseña no revela que una cuenta entra con Google.** Una cuenta sin `password_hash` recibe exactamente el mismo error y consume exactamente el mismo tiempo que un email inexistente (mismo argumento que el párrafo anterior de §1). Decir "esta cuenta entra con Google" sería más amable, pero convertiría el endpoint en un oráculo de qué direcciones tienen cuenta en la institución y con qué la abrieron. La pantalla de login tiene el botón de Google al lado, que es donde esa persona encuentra la salida.

**Es lo único que carga código de un tercero.** La biblioteca de Google (`accounts.google.com/gsi/client`) no se puede empaquetar en el bundle: la URL es parte del contrato, porque el script se comunica con esa misma página, y el botón se dibuja en un iframe de ese origen. La CSP del HTML lo habilita explícitamente en `script-src`, `frame-src` y `connect-src` (ver §4) — si eso se quita, el botón deja de aparecer sin más síntoma que su ausencia.

**`POST /api/auth/cambiar-password` sí lo dice explícitamente** (409): quien llega ahí ya está autenticado y es dueño de la cuenta, así que no hay nada que revelarle sobre sí mismo. Un Admin puede darle una contraseña con `reset-password` sin romper el vínculo con Google — es la forma de devolverle el acceso a alguien que perdió su cuenta de Google.

## 2. Estructura del JWT

```json
{ "sub": "userId", "rol": "ADMIN|DOCENTE", "nombre": "...", "apellido": "...", "dcp": true, "vs": 3, "exp": ... }
```

`dcp` (`debe_cambiar_password`) viaja en el token para poder exigir el cambio
sin consultar la base en cada request. Solo aparece cuando es `true`.

`vs` (versión de sesión) es lo que permite **cerrar las sesiones abiertas**
al cambiar una contraseña — ver §1. Solo aparece cuando no es 0.

Los demás campos son para mostrar el nombre en la interfaz sin pedir el
perfil; **ninguno se usa para autorizar**: el rol que decide es el que sale
de la base al verificar la cuenta (§1).

## 3. Matriz RBAC

| Acción | Admin | Docente |
|---|:---:|:---:|
| Crear/aprobar otros Admins | ✅ | ❌ |
| Promover un docente a Admin | ✅ | ❌ |
| Quitarle el rol Admin a otro Admin | ✅ | ❌ |
| Crear/editar carros | ✅ | ❌ |
| Registrar/editar equipos, dar de baja un equipo | ✅ | ❌ |
| Ver inventario (carros y equipos, incl. software instalado y freezado) | ✅ | ✅ |
| Cambiar estado de un equipo | ✅ | ❌ |
| Registrar incidencia | ✅ | ✅ solo reportar |
| Ver el historial de incidencias de un equipo | ✅ | ✅ |
| Cambiar estado de incidencia / marcar envío a soporte | ✅ | ❌ |
| Aprobar cuentas de docentes | ✅ | ❌ |
| Resetear contraseña de un usuario | ✅ | ❌ |
| Dar de baja a un docente (permanente) | ✅ | ❌ |
| Eliminar definitivamente una cuenta en BAJA | ✅ | ❌ |
| Remover docente de una materia puntual | ✅ | ❌ |
| Gestionar ciclos, cursos, materias (crear, editar, eliminar sin reservas) | ✅ | ❌ |
| Archivar y clonar ciclo lectivo | ✅ | ❌ |
| Asignar docentes a materias | ✅ | ❌ |
| Ver calendario de un equipo | ✅ | ✅ |
| Reservar para cualquier materia | ✅ | ❌ |
| Reservar para materia asignada | ✅ | ✅ solo asignadas |
| Cancelar una reserva propia (un equipo o el grupo completo) | ✅ | ✅ |
| Cancelar reserva ajena (con motivo) | ✅ | ❌ |
| Ver el detalle de un grupo de reserva (`GET /grupos/{id}`) | ✅ | ✅ solo propias |
| Ver quién tiene tomado un equipo en una franja (RF-04.11) | ✅ | ✅ |
| Pedirle a otro que libere un equipo reservado (RF-04.12) | ✅ | ✅ solo ajenas |
| Bloquear equipos con un motivo (RF-04.7) | ✅ | ❌ |
| Ver reportes (activos e históricos) | ✅ | ❌ |
| Cambiar el equipo de una reserva (RF-08.14) | ✅ | ✅ solo propias |
| Entregar y recibir equipos, y ver qué está prestado (RF-08) | ✅ | ❌ |
| Gestionar licencias de software (RF-03.11 a RF-03.14) | ✅ | ❌ |
| Ver notificaciones propias | ✅ | ✅ |
| Elegir qué avisos recibir por correo (RF-05.13) | ✅ todas | ✅ solo las suyas — las seis de administración no las ve ni las puede activar |
| Pedir ayuda y escribir al equipo de administración (RF-09) | ✅ | ✅ |
| Contestar conversaciones ajenas y darlas por resueltas (RF-09) | ✅ | ❌ — en la suya sí escribe, y escribir la reabre |
| Configurar mi horario de disponibilidad | ✅ | ❌ |
| Ver disponibilidad de Admins | ✅ | ✅ |

## 4. Controles de infraestructura

| Control | Detalle |
|---|---|
| HTTPS | Cloudflare termina TLS; el túnel cifra hasta el servidor |
| Rate limiting | `/api/auth/login`: 30/min por IP **y** 10/min por cuenta. `/api/auth/registro`: 5/min por IP. `/api/auth/google`: 30/min por IP; `/api/auth/google/registro`: 5/min por IP. `/api/auth/password/olvide`: 5/min por IP y **3/min por email**; `/api/auth/password/restablecer`: 10/min por IP y por email |
| IP real del cliente | `CF-Connecting-IP`, aceptado solo desde `TRUSTED_PROXIES` |
| Password temporal | La API responde 403 mientras `debe_cambiar_password` siga en `true` |
| Revocación de sesiones | Cambiar la contraseña invalida los tokens ya emitidos de esa cuenta (`usuario.version_sesion` vs. el claim `vs`) |
| Headers | `HSTS`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, `CSP` restrictiva. En **dos** lugares: el binario Go los pone en `/api` y nginx en el HTML y los assets (ver abajo) |
| CORS | Solo dominio del frontend, sin wildcard |
| Validación | Estricta en cada handler; nunca se confía en el frontend |
| Secrets | `.env` fuera de git + Docker secrets. Secreto JWT nunca en el repo |
| Permisos DB | Un usuario Postgres de aplicación con GRANT sobre `sgrc_db`, sin permisos de `SUPERUSER` |

### Cómo se dan y cómo se quitan los permisos de Admin

Hay dos formas de que exista un Admin: que otro Admin lo cree directo (`POST /api/auth/admins`, RF-01.4) o que promueva a un docente ya aprobado (`POST /api/auth/usuarios/{id}/promover-a-admin`). Las dos las tiene que iniciar un Admin; ninguna es alcanzable desde afuera. El autorregistro —con contraseña o con Google— crea siempre rol DOCENTE, porque es un endpoint público sin autenticar y no puede ser una puerta por la que alguien se asigne un rol.

Promover **solo agrega** Admins, así que no pasa por el guard del último Admin ni necesita transacción: no hay forma de que deje al sistema sin ninguno. Y no toca nada más de la cuenta —conserva materias, reservas y formas de ingreso— porque un docente que pasa a coordinar suele seguir dando clase: `ExisteYAprobado` de academic nunca miró el rol, y reservar tampoco lo pide.

La inversa es `POST /api/auth/usuarios/{id}/degradar-a-docente`: le quita los permisos y lo deja como docente, **sin cerrarle la cuenta**. Hasta que existió, la única forma de sacar a un Admin era darle de baja la cuenta entera —perdiendo sus materias y cancelándole las reservas— por un cambio que era solo de permisos. Son dos rutas que dicen lo que hacen y no un `PATCH /rol`: cada una tiene sus propias condiciones y su propia entrada de auditoría.

Degradar sí puede reducir la cantidad de Admins, así que le corresponden dos frenos que promover no necesita:

- **El guard del último Admin (RF-01.8)**, el mismo de la baja y por lo tanto dentro de la misma transacción: contar y escribir tienen que ser atómicos o dos pedidos concurrentes —una baja y una degradación, por ejemplo— leen ambos "quedan 2", pasan los dos, y el sistema se queda sin nadie que pueda aprobar cuentas ni volver a promover a nadie.
- **Nadie se degrada a sí mismo.** No es por el guard, que ya cubre el caso del último: es que quien apretara el botón perdería en el acto las pantallas desde las que lo apretó, y para volver atrás dependería de otro Admin. El precio de prohibirlo es nulo, porque si no hubiera otro Admin el guard lo rechazaría igual.

Lo que degradar **no** toca es el resto de la cuenta, por lo mismo que promover: conserva materias, reservas y formas de ingreso. Lo único que deja de figurar es su horario de atención, porque la lista de Admins se arma filtrando por rol — y si más adelante lo vuelven a promover, reaparece con el horario que ya tenía cargado.

El cambio **tiene efecto en el request siguiente, sin volver a iniciar sesión**, por lo mismo que una baja es inmediata (§1): el middleware lee el rol de la base en cada pedido y pisa el del token. La contracara es que un token viejo no conserva el rol viejo, ni para bien ni para mal.

### Por qué la CSP está en dos lugares y no en uno

Los headers de `internal/shared/middleware/security.go` los pone el binario Go, así que salen únicamente en las respuestas de `/api` — que son JSON, donde una CSP no restringe nada real. **El HTML de la SPA lo sirve nginx**, y es justamente el único documento que un navegador puede ser engañado de ejecutar: una CSP que viviera solo del lado de Go no lo cubriría.

Eso lo resuelve `frontend/nginx-seguridad.conf`, incluido desde los dos `location` que sirven contenido propio. No se ponen en el bloque `server` por dos razones: nginx deja de heredar los `add_header` en cuanto un bloque define uno propio (y `/assets/` define su `Cache-Control`), y las rutas proxeadas a `/api` quedarían con dos headers `Content-Security-Policy` distintos en la misma respuesta.

Dos decisiones dentro de esa política merecen quedar escritas:

- **`script-src` autoriza el script inline de `index.html` por su hash SHA-256, no con `'unsafe-inline'`.** Ese script aplica el tema antes de pintar; sin él vuelve el fogonazo blanco al recargar en modo oscuro. Un `'unsafe-inline'` habría sido más cómodo pero deja pasar cualquier script que una inyección logre meter en el HTML, que es exactamente lo que la CSP está para impedir. El costo del hash es que se desincroniza en silencio: si alguien edita el script y no actualiza la CSP, el navegador lo bloquea y el único síntoma es el fogonazo, que nadie asocia con un header. Por eso hay un test (`frontend/csp.test.ts`) que recalcula el hash y falla con el valor nuevo listo para pegar.
- **`style-src` sí lleva `'unsafe-inline'`, y es deliberado.** La grilla del calendario posiciona cada bloque con `style={{top, height}}` y las barras de los reportes fijan su ancho igual; sin eso el calendario queda con todos los bloques apilados y sin alto. Una inyección de CSS es de otro orden de gravedad que una de script, que sí queda cerrada.

### Por qué el login se limita también por cuenta

Limitar solo por IP falla en las dos direcciones. Los docentes que entran desde el wifi de la institución salen todos por la misma IP NAT y se consumen la cuota entre ellos; y quien prueba contraseñas contra una cuenta puntual esquiva el límite cambiando de red. La cuenta atacada es lo único constante, así que se limita por las dos cosas a la vez.

Las ventanas son por minuto y no por segundo: `5 req/s` son 18.000 intentos por hora, que no frena a nadie.

### Por qué la recuperación de contraseña no dice nunca si un email existe

`POST /api/auth/password/olvide` responde **202 con el mismo cuerpo pase lo que pase**: exista la cuenta o no, esté aprobada o no, tenga contraseña o entre con Google. La pantalla tampoco lo desmiente — pasa al paso del código en todos los casos.

Es la única forma de que el formulario no sea un padrón. Es un endpoint público, sin autenticar y sin captcha: si distinguiera "te mandamos el código" de "ese email no existe", cualquiera podría ir probando direcciones y quedarse con la lista de los docentes de la institución, que después sirve para phishing dirigido con el nombre real de la institución.

Por el mismo motivo el **tiempo de respuesta** también tiene que ser indistinguible, y eso obligó a una decisión que se lee rara en el código: el código de recuperación se genera y se **hashea con argon2 antes de buscar la cuenta**, aunque quizás no haya a quién mandárselo. El hash cuesta cientos de milisegundos, mucho más que todo el resto de la operación; calculándolo recién después de encontrar la cuenta, un email registrado tardaría notoriamente más que uno inexistente y medir esa diferencia desde afuera es trivial. Es la misma idea que `consumirTiempoDeVerificacion` en el login (§1), aplicada del lado de la escritura.

Lo que **sí** se distingue, y a propósito, son dos errores del segundo paso: "el código venció" y "se agotaron los intentos". Los dos le pasan a la persona legítima, que ya demostró tener acceso a la casilla, y necesita saber que tiene que pedir otro código en vez de seguir tipeando el mismo. Todo lo demás —email inexistente, cuenta sin código pendiente, código equivocado— colapsa al mismo mensaje.

### Por qué seis dígitos alcanzan

Un millón de combinaciones no es mucho por sí solo. Lo que hace seguro al código es lo que lo rodea, y las cuatro cosas hacen falta juntas:

- **15 minutos de vigencia.** Acota la ventana de cualquier ataque y la de un código olvidado en una casilla abierta en la sala de profesores.
- **5 intentos por código.** Al quinto fallo el código se quema y hay que pedir otro. La probabilidad de acertar a ciegas queda en 5 en 1.000.000.
- **Un solo código vigente por persona.** Pedir uno nuevo invalida el anterior, en el mismo statement que inserta el nuevo (un CTE, ver `postgres_repo_recuperacion.go`). Sin esto el tope de intentos sería esquivable pidiendo veinte códigos y probando cinco veces con cada uno.
- **Rate limit por email, no solo por IP.** El límite por IP no frena a quien cambia de red, y acá hay algo más que adivinar códigos: pedir el código manda un mail a una casilla ajena, así que sin tope por cuenta el formulario es un botón para inundar de correo a un docente — y para quemarle la reputación a la casilla saliente, que tiene su propio límite diario.

Además el código se guarda **hasheado con argon2**, igual que una contraseña. Si la base se filtrara (un backup, un dump de soporte), un código en claro sería una cuenta abierta hasta que expire.

Dos cosas más que valen para el correo en sí. El mail del código es el **único del sistema sin enlace al sistema**: es también el único que contiene una credencial, y "código + botón para entrar" es exactamente la forma de un phishing — acostumbrar a los docentes a apretar un link dentro de un mail con un código es entrenarlos para el día que llegue uno falso. Y el handler que lo procesa es el único que **no loguea el payload** cuando el tipo del evento no es el esperado: un `%+v` dejaría el código en claro escrito en los logs del contenedor.

### Por qué hace falta `TRUSTED_PROXIES`

El tráfico llega Cloudflare → `cloudflared` → `nginx` → `sgrc-app`, así que la IP del socket es siempre la de un contenedor. Sin configurar `ProxyHeader`, el rate limiting por IP degrada a un balde único para toda la institución y `audit_log.ip_origen` guarda la IP de nginx en cada fila — presente, pero inútil para saber quién hizo qué.

Se usa `CF-Connecting-IP` y no `X-Forwarded-For` porque Cloudflare la **sobrescribe** siempre con la IP real del cliente: no es falsificable mientras el único camino hasta la app sea el túnel. De ahí que el compose de producción no publique el `8080` al host — ese atajo desde la LAN de la institución permitiría inventar el header. `TRUSTED_PROXIES` vacío degrada a usar la IP del socket, que es el default correcto: se pierde la IP real, no se gana una falsificable.

### Por qué el cambio de contraseña devuelve un token nuevo

`debe_cambiar_password` viaja dentro del JWT para poder exigirlo sin consultar la base en cada request, pero eso lo deja congelado en el token. `POST /api/auth/cambiar-password` responde con un token nuevo y el cliente tiene que reemplazar el anterior; si no, quien acaba de cambiar la contraseña quedaría bloqueado por su propio cambio exitoso hasta que el token expirara.

Las únicas dos rutas que aceptan un token con la contraseña temporal sin cambiar son `GET /api/auth/me` y `POST /api/auth/cambiar-password` — justamente las que hacen falta para salir de esa situación. Se marcan explícitamente con `JWTAuthPermitiendoPasswordVencida`; todo lo demás usa `JWTAuth`, que ya incluye la restricción, de modo que una ruta nueva queda protegida por omisión.

### Por qué la contraseña actual equivocada responde 400 y no 401

Un 401 significa "el que hace este pedido no está autenticado", y un cliente que recibe uno con un token que venía usando entiende que su sesión dejó de valer: la descarta y manda a iniciar sesión de nuevo. Es el comportamiento correcto, y el que necesita el sistema para los dos casos que lo motivan — una cuenta dada de baja (RF-02.8) y las sesiones que se cierran al cambiar la contraseña (RF-01.11).

Por eso `POST /api/auth/cambiar-password` **no** puede responder 401 cuando lo que está mal es el campo `passwordActual`. Quien llega hasta ese handler está perfectamente autenticado —su token se validó para dejarlo pasar— y lo que vino mal es un dato del cuerpo. Devolver 401 ahí le cerraba la sesión a alguien que se equivocó tipeando, y lo dejaba en el login leyendo "credenciales inválidas" sin ninguna pista de qué había pasado.

La distinción vive en el error de dominio (`ErrPasswordActualIncorrecta`, separado de `ErrCredencialesInvalidas`) y no en el handler, para que ningún camino nuevo hacia el mismo chequeo se lleve puesto el código equivocado.

## 5. Tabla de auditoría

```sql
CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id  UUID NOT NULL,          -- sin FK: lo que hizo una cuenta
    accion      VARCHAR(100) NOT NULL,  -- sobrevive a su eliminación
    entidad     VARCHAR(50) NOT NULL,
    entidad_id  UUID,
    detalle     JSONB,
    ip_origen   INET,
    creado_en   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_usuario ON audit_log (usuario_id, creado_en DESC);
```

`accion` es texto libre y no un enum a propósito: los valores guardados son el
nombre que tenía una operación **en su momento**. Si el sistema renombra algo,
las filas viejas conservan el nombre viejo — reescribir un registro de
auditoría es precisamente lo que un registro de auditoría no debe permitir.

Acciones auditadas: `CUENTA_APROBADA`, `CUENTA_RECHAZADA`, `CUENTA_BAJA`, `CUENTA_ELIMINADA_DEFINITIVAMENTE`, `ADMIN_CREADO`, `ROL_PROMOVIDO_A_ADMIN`, `ROL_DEGRADADO_A_DOCENTE`, `PASSWORD_RESETEADA`, `PASSWORD_RECUPERADA_POR_EMAIL`, `NOMBRE_CAMBIADO`, `DOCENTE_REMOVIDO_DE_MATERIA`, `DOCENTE_ROL_CAMBIADO`, `RESERVA_CANCELADA_POR_ADMIN`, `BLOQUEO_CREADO`, `EQUIPO_ESTADO_CAMBIADO`, `EQUIPO_DADO_DE_BAJA`, `EQUIPO_MOVIDO_DE_CARRO`, `CURSO_ELIMINADO`, `MATERIA_ELIMINADA`, `CICLO_ARCHIVADO_RESERVAS_ELIMINADAS`, `CICLO_CLONADO`.

> `CICLO_ARCHIVADO_RESERVAS_ELIMINADAS` tiene su propio nombre (en vez de un `CICLO_ARCHIVADO` genérico) porque implica un borrado físico de datos — vale la pena que quede explícito en el log qué admin lo disparó y cuántas filas se eliminaron (`detalle` puede guardar el conteo).

> `NOMBRE_CAMBIADO` es la única acción del catálogo que alguien hace **sobre su propia cuenta y sin ser `ADMIN`** (RF-01.12). Se audita igual porque el nombre es con lo que el resto de la escuela identifica a esa persona en las reservas y en las entregas: sin esta fila, "la reserva la había pedido otro" no tendría cómo verificarse. `usuario_id` es la propia cuenta, y `detalle` guarda el nombre con el que quedó.

> `PASSWORD_RECUPERADA_POR_EMAIL` es la **única acción del catálogo cuyo actor no está autenticado**: en RF-01.10 la persona prueba ser dueña de la cuenta con el código que le llegó al mail, no con un token. En esa fila `usuario_id` es la propia cuenta (no un Admin que actuó sobre ella), y lo que la hace útil es `ip_origen` — es lo que permite reconstruir qué pasó si alguien reporta que él no cambió su contraseña. Los intentos fallidos no se auditan: viven en `codigo_recuperacion.intentos`, que es donde tienen efecto.
