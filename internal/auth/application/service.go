// Package application orquesta los casos de uso de RF-01 (usuarios y
// autenticación) y las partes de RF-02 que no dependen todavía de
// academic/reservation (ver el TODO en DarDeBaja).
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// Service implementa los casos de uso de auth. Todas sus dependencias
// externas (repo, bus, hash, verify, firma de JWT, generación de IDs,
// reloj) están inyectadas — nada de esto llama directamente a Postgres,
// argon2 o golang-jwt, así que se puede testear entero con fakes.
type Service struct {
	repo               Repo
	bus                eventbus.EventBus
	hash               HashFunc
	verify             VerifyFunc
	firmar             TokenSigner
	nuevoID            IDGenerator
	generarTemporal    GenerarTemporalFunc
	ahora              func() time.Time
	gestorMaterias     GestorMateriasDocente
	canceladorReservas CanceladorReservasDeMateria

	// verificadorGoogle es nil cuando el despliegue no configuró
	// GOOGLE_CLIENT_ID. Es opcional a propósito: el sistema tiene que
	// arrancar y funcionar igual sin ingreso con Google, que es como venía
	// funcionando hasta ahora. Los dos casos de uso que lo necesitan
	// chequean el nil y devuelven ErrLoginGoogleNoDisponible.
	verificadorGoogle VerificadorGoogle

	// Hash de descarte para igualar el costo del login con email
	// inexistente. Ver consumirTiempoDeVerificacion.
	hashDeDescarteUnaVez sync.Once
	hashDeDescarte       string
}

// NewService construye el Service con todas sus dependencias.
func NewService(
	repo Repo,
	bus eventbus.EventBus,
	hash HashFunc,
	verify VerifyFunc,
	firmar TokenSigner,
	nuevoID IDGenerator,
	generarTemporal GenerarTemporalFunc,
	ahora func() time.Time,
	gestorMaterias GestorMateriasDocente,
	canceladorReservas CanceladorReservasDeMateria,
	verificadorGoogle VerificadorGoogle,
) *Service {
	return &Service{
		repo:               repo,
		bus:                bus,
		hash:               hash,
		verify:             verify,
		firmar:             firmar,
		nuevoID:            nuevoID,
		generarTemporal:    generarTemporal,
		ahora:              ahora,
		gestorMaterias:     gestorMaterias,
		canceladorReservas: canceladorReservas,
		verificadorGoogle:  verificadorGoogle,
	}
}

// SolicitudDeAsignacion es lo que el docente declara al registrarse: qué
// curso y qué materia va a dictar. Los dos son opcionales — quien no lo
// sepa todavía puede registrarse igual y arreglarlo con el Admin.
type SolicitudDeAsignacion struct {
	Curso   string
	Materia string
}

// Registrar implementa RF-01.3: autorregistro de docente, queda PENDIENTE.
//
// La solicitud viaja con el registro para que el Admin, al aprobar, sepa a
// qué materia y curso asignarlo sin tener que preguntárselo por fuera del
// sistema (y para que sepa si tiene que crearlos antes).
func (s *Service) Registrar(ctx context.Context, nombre, apellido, email, password string, solicitud SolicitudDeAsignacion) (*domain.Usuario, error) {
	u, err := s.crearUsuario(ctx, nombre, apellido, email, password, domain.RolDocente, domain.EstadoPendiente, nil,
		strings.TrimSpace(solicitud.Curso), strings.TrimSpace(solicitud.Materia))
	if err != nil {
		return nil, err
	}

	s.avisarQueHayUnDocentePendiente(u)
	return u, nil
}

// avisarQueHayUnDocentePendiente publica RF-05.6 — el aviso a todos los
// Admin de que alguien se registró y está esperando aprobación.
//
// Sale igual se haya registrado con contraseña o con Google: para el Admin
// que tiene que aprobarla, las dos son la misma cuenta pendiente, y cómo
// entró esa persona no cambia nada de lo que él hace.
func (s *Service) avisarQueHayUnDocentePendiente(u *domain.Usuario) {
	s.bus.Publish(eventbus.Evento{
		Tipo: "docente.registro.pendiente",
		Payload: map[string]string{
			"usuarioId": u.ID,
			"nombre":    u.Nombre,
			"apellido":  u.Apellido,
			"email":     u.Email,
		},
	})
}

// CrearAdmin implementa RF-01.4: un Admin crea otro Admin directamente,
// sin pasar por PENDIENTE — queda APROBADA de inmediato, con
// AprobadoPor apuntando a quien lo creó.
func (s *Service) CrearAdmin(ctx context.Context, creadoPorID, nombre, apellido, email, password string) (*domain.Usuario, error) {
	// Un Admin no declara curso ni materia: no se autorregistra para dictar,
	// lo crea otro Admin ya aprobado.
	return s.crearUsuario(ctx, nombre, apellido, email, password, domain.RolAdmin, domain.EstadoAprobada, &creadoPorID, "", "")
}

// crearUsuario centraliza la validación y creación común a Registrar y
// CrearAdmin — lo único que cambia entre los dos casos es el rol, el
// estado inicial, y si hay que dejar registro de quién aprobó.
func (s *Service) crearUsuario(ctx context.Context, nombre, apellido, email, password string, rol domain.Rol, estadoInicial domain.Estado, aprobadoPor *string, cursoSolicitado, materiaSolicitada string) (*domain.Usuario, error) {
	nombre = strings.TrimSpace(nombre)
	apellido = strings.TrimSpace(apellido)
	// El email se normaliza ANTES de buscarlo y de guardarlo, así la
	// comparación contra lo que ya existe y lo que termina en la fila usan
	// la misma forma (ver domain.NormalizarEmail).
	email = domain.NormalizarEmail(email)

	if nombre == "" || apellido == "" || email == "" {
		return nil, ErrDatosObligatorios
	}
	if err := domain.ValidarEmail(email); err != nil {
		return nil, err
	}
	if len(password) < minPasswordLen {
		return nil, ErrPasswordCorta
	}

	existente, err := s.repo.BuscarPorEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrUsuarioNoEncontrado) {
		return nil, fmt.Errorf("buscando usuario existente: %w", err)
	}
	if existente != nil {
		if existente.Estado == domain.EstadoBaja {
			return nil, ErrCuentaEnBaja
		}
		return nil, ErrEmailYaRegistrado
	}

	hash, err := s.hash(password)
	if err != nil {
		return nil, fmt.Errorf("hasheando password: %w", err)
	}

	ahora := s.ahora()
	u := &domain.Usuario{
		ID:            s.nuevoID(),
		Nombre:        nombre,
		Apellido:      apellido,
		Email:         email,
		PasswordHash:  hash,
		Rol:           rol,
		Estado:        estadoInicial,
		FechaRegistro: ahora,

		CursoSolicitado:   cursoSolicitado,
		MateriaSolicitada: materiaSolicitada,
	}
	if estadoInicial == domain.EstadoAprobada {
		u.FechaAprobacion = &ahora
		u.AprobadoPor = aprobadoPor
	}

	if err := s.repo.Crear(ctx, u); err != nil {
		return nil, fmt.Errorf("creando usuario: %w", err)
	}

	return u, nil
}

// LoginResultado es lo que el handler HTTP necesita para armar la
// respuesta de RF-01.1.
type LoginResultado struct {
	Token               string
	DebeCambiarPassword bool
}

// Login implementa RF-01.1. Nunca revela si el email existe o no en el
// mensaje de error — "credenciales inválidas" cubre tanto "no existe" como
// "existe pero la contraseña está mal".
func (s *Service) Login(ctx context.Context, email, password string) (*LoginResultado, error) {
	// Misma normalización que en crearUsuario: sin esto, quien se registró
	// como "Juan.Perez@…" y escribe "juan.perez@…" al entrar recibía
	// "credenciales inválidas" aunque la contraseña fuera correcta.
	u, err := s.repo.BuscarPorEmail(ctx, domain.NormalizarEmail(email))
	if err != nil {
		if errors.Is(err, ErrUsuarioNoEncontrado) {
			// El mensaje ya no revelaba si el email existe, pero el tiempo sí:
			// con email inexistente se volvía de inmediato, y con email real se
			// corría argon2id con 64 MB. La diferencia es de decenas de
			// milisegundos, trivial de medir en un loop, y alcanza para
			// enumerar quién tiene cuenta en la escuela.
			//
			// Se hashea igual contra un hash de descarte para que los dos
			// caminos cuesten lo mismo. El resultado se descarta a propósito.
			s.consumirTiempoDeVerificacion(password)
			return nil, ErrCredencialesInvalidas
		}
		return nil, fmt.Errorf("buscando usuario: %w", err)
	}

	// Una cuenta creada con Google no tiene contraseña contra la cual
	// verificar. Se responde exactamente igual que a un email inexistente
	// —mismo error y mismo tiempo consumido— y no "esta cuenta entra con
	// Google", que sería más amable pero convertiría este endpoint en un
	// oráculo: cualquiera podría preguntarle, dirección por dirección,
	// quién tiene cuenta en la escuela y con qué la abrió. La pantalla de
	// login tiene el botón de Google al lado, que es donde esa persona
	// encuentra la salida sin que haga falta decírselo acá.
	if !u.PuedeIngresarConPassword() {
		s.consumirTiempoDeVerificacion(password)
		return nil, ErrCredencialesInvalidas
	}

	ok, err := s.verify(password, u.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("verificando password: %w", err)
	}
	if !ok {
		return nil, ErrCredencialesInvalidas
	}

	if !u.EstaAprobado() {
		return nil, ErrCuentaNoHabilitada
	}

	token, err := s.firmar(u)
	if err != nil {
		return nil, fmt.Errorf("firmando token: %w", err)
	}

	return &LoginResultado{Token: token, DebeCambiarPassword: u.DebeCambiarPassword}, nil
}

// LoginConGoogle implementa el ingreso con una cuenta de Google ya
// existente en el sistema. NO crea nada: si no hay cuenta para ese email
// devuelve ErrCuentaGoogleNoRegistrada, y es el frontend el que a partir
// de ahí manda a la pantalla de registro.
//
// Que el registro sea un paso aparte no es ceremonia: al crear la cuenta
// hay que preguntarle a la persona qué curso y qué materia va a dictar
// (RF-01.3), y eso no viene en ningún token de Google. Sin ese paso el
// Admin recibiría cuentas pendientes sin ningún dato para saber a qué
// asignarlas, que es exactamente el problema que resolvió la migración
// 006.
func (s *Service) LoginConGoogle(ctx context.Context, idToken string) (*LoginResultado, error) {
	identidad, err := s.identidadDeGoogle(ctx, idToken)
	if err != nil {
		return nil, err
	}

	u, err := s.cuentaParaIdentidadGoogle(ctx, identidad)
	if err != nil {
		return nil, err
	}

	// El chequeo de estado va DESPUÉS de vincular: una cuenta PENDIENTE que
	// entra con Google por primera vez queda vinculada igual, así el día
	// que el Admin la apruebe funciona sin que la persona tenga que repetir
	// nada. Lo único que no se vincula es una cuenta en BAJA, que se
	// rechaza antes (ver cuentaParaIdentidadGoogle).
	if !u.EstaAprobado() {
		return nil, ErrCuentaNoHabilitada
	}

	token, err := s.firmar(u)
	if err != nil {
		return nil, fmt.Errorf("firmando token: %w", err)
	}
	return &LoginResultado{Token: token, DebeCambiarPassword: u.DebeCambiarPassword}, nil
}

// cuentaParaIdentidadGoogle resuelve a qué usuario corresponde una
// identidad de Google, vinculándola si hace falta.
//
// Busca primero por sub (el vínculo ya establecido) y recién después por
// email. El orden importa: el email de una cuenta de Google puede cambiar,
// el sub no, así que quien ya entró alguna vez sigue entrando a su misma
// cuenta aunque Google le haya cambiado la dirección.
//
// Cuando aparece por email, se trata de un docente que ya tenía cuenta con
// contraseña y ahora entra con Google: se le agrega el sub y conserva la
// contraseña — las dos formas de ingreso conviven (ver
// migrations/008_login_con_google.sql). Vincular por email es seguro
// justamente porque identidadDeGoogle ya exigió email_verified: sin eso,
// alguien podría poner la dirección de un docente en su propio perfil de
// Google y quedarse con su cuenta.
func (s *Service) cuentaParaIdentidadGoogle(ctx context.Context, identidad *IdentidadGoogle) (*domain.Usuario, error) {
	u, err := s.repo.BuscarPorGoogleSub(ctx, identidad.Sub)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, ErrUsuarioNoEncontrado) {
		return nil, fmt.Errorf("buscando cuenta por identificador de Google: %w", err)
	}

	u, err = s.repo.BuscarPorEmail(ctx, domain.NormalizarEmail(identidad.Email))
	if err != nil {
		if errors.Is(err, ErrUsuarioNoEncontrado) {
			return nil, ErrCuentaGoogleNoRegistrada
		}
		return nil, fmt.Errorf("buscando cuenta por email: %w", err)
	}

	// Misma regla que en el registro común: una cuenta dada de baja no se
	// reactiva por la puerta de atrás. RF-02.9 la hace terminal.
	if u.Estado == domain.EstadoBaja {
		return nil, ErrCuentaEnBaja
	}

	u.GoogleSub = identidad.Sub
	if err := s.repo.Guardar(ctx, u); err != nil {
		return nil, fmt.Errorf("vinculando la cuenta de Google: %w", err)
	}
	return u, nil
}

// RegistrarConGoogle crea una cuenta de docente a partir de un ID token de
// Google. Queda PENDIENTE igual que el autorregistro con contraseña
// (RF-01.3): entrar con Google prueba quién sos, no que la escuela te
// conozca — sin la aprobación de un Admin, cualquiera con una casilla de
// Gmail estaría adentro.
//
// nombre y apellido pueden venir vacíos: en ese caso se usan los del
// token. Se aceptan del request porque los claims de Google son lo que la
// persona puso en su cuenta personal, que no siempre es su nombre tal como
// figura en la escuela, y porque given_name/family_name no son
// obligatorios — hay cuentas donde vienen vacíos.
func (s *Service) RegistrarConGoogle(ctx context.Context, idToken, nombre, apellido string, solicitud SolicitudDeAsignacion) (*domain.Usuario, error) {
	identidad, err := s.identidadDeGoogle(ctx, idToken)
	if err != nil {
		return nil, err
	}

	nombre = primeroNoVacio(strings.TrimSpace(nombre), identidad.Nombre)
	apellido = primeroNoVacio(strings.TrimSpace(apellido), identidad.Apellido)
	email := domain.NormalizarEmail(identidad.Email)

	if nombre == "" || apellido == "" || email == "" {
		return nil, ErrDatosObligatorios
	}
	if err := domain.ValidarEmail(email); err != nil {
		return nil, err
	}

	// Dos consultas y no una: alguien puede tener ya la cuenta vinculada
	// (sub conocido) y volver a caer acá por un reintento del frontend, y
	// también puede existir una cuenta con ese email pero sin vincular. Los
	// dos casos son "ya tenés cuenta, andá a iniciar sesión", pero el
	// segundo tiene que distinguir BAJA para dar el mensaje de RF-01.3.
	if _, err := s.repo.BuscarPorGoogleSub(ctx, identidad.Sub); err == nil {
		return nil, ErrEmailYaRegistrado
	} else if !errors.Is(err, ErrUsuarioNoEncontrado) {
		return nil, fmt.Errorf("buscando cuenta por identificador de Google: %w", err)
	}

	existente, err := s.repo.BuscarPorEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrUsuarioNoEncontrado) {
		return nil, fmt.Errorf("buscando usuario existente: %w", err)
	}
	if existente != nil {
		if existente.Estado == domain.EstadoBaja {
			return nil, ErrCuentaEnBaja
		}
		// La cuenta existe con ese email pero sin vincular. No se vincula
		// acá: vincular es lo que hace LoginConGoogle, y hacerlo también
		// desde el registro dejaría dos caminos distintos para la misma
		// escritura. El frontend ya llamó al login antes de llegar acá, así
		// que en la práctica esto solo se alcanza con dos pestañas abiertas.
		return nil, ErrEmailYaRegistrado
	}

	ahora := s.ahora()
	u := &domain.Usuario{
		ID:        s.nuevoID(),
		Nombre:    nombre,
		Apellido:  apellido,
		Email:     email,
		GoogleSub: identidad.Sub,
		// Sin PasswordHash a propósito: no hay ninguna contraseña que la
		// persona haya elegido, y poner una aleatoria que nadie conoce sería
		// mentirle al esquema sobre lo que la cuenta puede hacer.
		Rol:           domain.RolDocente,
		Estado:        domain.EstadoPendiente,
		FechaRegistro: ahora,

		CursoSolicitado:   strings.TrimSpace(solicitud.Curso),
		MateriaSolicitada: strings.TrimSpace(solicitud.Materia),
	}

	if err := s.repo.Crear(ctx, u); err != nil {
		return nil, fmt.Errorf("creando usuario: %w", err)
	}

	s.avisarQueHayUnDocentePendiente(u)
	return u, nil
}

// identidadDeGoogle centraliza lo que los dos casos de uso necesitan antes
// de tocar la base: que el ingreso con Google esté configurado, que el
// token sea creíble, y que Google confirme el email.
func (s *Service) identidadDeGoogle(ctx context.Context, idToken string) (*IdentidadGoogle, error) {
	if s.verificadorGoogle == nil {
		return nil, ErrLoginGoogleNoDisponible
	}
	if strings.TrimSpace(idToken) == "" {
		return nil, ErrTokenGoogleInvalido
	}

	identidad, err := s.verificadorGoogle.Verificar(ctx, idToken)
	if err != nil {
		// ErrDominioNoPermitido y ErrTokenGoogleInvalido ya son errores de
		// negocio con su propio código HTTP: se dejan pasar tal cual. El
		// resto (una falla de red buscando las claves de Google, por
		// ejemplo) es un 500 legítimo y no hay que disfrazarlo de token
		// inválido, o el problema se vuelve indepurable desde afuera.
		if errors.Is(err, ErrDominioNoPermitido) || errors.Is(err, ErrTokenGoogleInvalido) {
			return nil, err
		}
		return nil, fmt.Errorf("verificando el token de Google: %w", err)
	}

	if !identidad.EmailVerificado {
		return nil, ErrEmailNoVerificadoPorGoogle
	}
	if strings.TrimSpace(identidad.Sub) == "" || strings.TrimSpace(identidad.Email) == "" {
		// Un token sin sub o sin email pasó la firma pero no sirve para
		// identificar a nadie. No debería ocurrir con Google; si ocurre, es
		// un token que no es lo que decimos que aceptamos.
		return nil, ErrTokenGoogleInvalido
	}
	return identidad, nil
}

func primeroNoVacio(a, b string) string {
	if a != "" {
		return a
	}
	return strings.TrimSpace(b)
}

// consumirTiempoDeVerificacion corre el mismo argon2id que haría un login
// real, contra un hash que no le pertenece a nadie.
//
// El hash de descarte se calcula una sola vez y se guarda: hashear en cada
// intento fallido serviría igual para emparejar los tiempos, pero le daría a
// cualquiera una forma de gastar 64 MB por request contra un endpoint sin
// autenticar. Con sync.Once, el costo por intento es el de una verificación,
// exactamente como el camino que imita.
//
// Si el hasheo inicial falla —no debería, es la misma función que usa el
// registro— se deja el hash vacío: verify va a devolver error de formato,
// que también se descarta. Lo único que importa acá es haber gastado el
// tiempo, no el resultado.
func (s *Service) consumirTiempoDeVerificacion(password string) {
	s.hashDeDescarteUnaVez.Do(func() {
		h, err := s.hash("contraseña que no le pertenece a ninguna cuenta")
		if err != nil {
			return
		}
		s.hashDeDescarte = h
	})
	_, _ = s.verify(password, s.hashDeDescarte)
}

// Aprobar implementa la mitad "aprobar" de RF-02 (aprobación de cuentas).
func (s *Service) Aprobar(ctx context.Context, usuarioID string) error {
	if err := s.transicionar(ctx, usuarioID, domain.EstadoAprobada); err != nil {
		return err
	}
	s.avisarQueYaNoEstaPendiente(usuarioID)
	return nil
}

// Rechazar implementa la mitad "rechazar" de RF-02.
func (s *Service) Rechazar(ctx context.Context, usuarioID string) error {
	if err := s.transicionar(ctx, usuarioID, domain.EstadoRechazada); err != nil {
		return err
	}
	s.avisarQueYaNoEstaPendiente(usuarioID)
	return nil
}

// avisarQueYaNoEstaPendiente publica que una cuenta dejó de estar a la
// espera, para que notification pueda cerrar el aviso que pedía resolverla.
//
// Va DESPUÉS de la transición y no dentro: si se publicara adentro y la
// transacción se deshiciera, el aviso se cerraría sobre algo que no pasó, y
// el Admin perdería el único recordatorio de que hay alguien esperando.
//
// Se publica igual al rechazar: el aviso decía "está pendiente de
// aprobación", y una cuenta rechazada tampoco lo está.
func (s *Service) avisarQueYaNoEstaPendiente(usuarioID string) {
	s.bus.Publish(eventbus.Evento{
		Tipo:    "cuenta.pendiente.resuelta",
		Payload: map[string]string{"usuarioId": usuarioID},
	})
}

// DarDeBaja implementa el cambio de estado de RF-02.8/02.9, más la
// cascada completa: si el usuario es docente, identifica sus materias
// ANTES de aplicar la transición (mientras las filas docente_materia
// todavía existen), y para cada una donde no quede ningún otro docente
// APROBADA asignado, cancela sus reservas futuras vía
// canceladorReservas.CancelarReservasFuturasDeMateria y publica un evento
// para que internal/notification (cuando exista) avise a los Admin. Si en
// cambio sigue habiendo otro docente activo en esa materia, no cancela
// nada — solo publica el evento informativo (RF-05.4). Protege al último
// Admin igual que cualquier otra transición (vía transicionar).
func (s *Service) DarDeBaja(ctx context.Context, usuarioID string) error {
	u, err := s.repo.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return err
	}

	var materiasHuerfanas []string
	var materiasConOtroDocente []string

	if u.EsDocente() {
		materias, err := s.gestorMaterias.MateriasDeDocente(ctx, usuarioID)
		if err != nil {
			return fmt.Errorf("listando materias del docente: %w", err)
		}
		for _, materiaID := range materias {
			quedaOtro, err := s.gestorMaterias.QuedaOtroDocenteActivo(ctx, materiaID, usuarioID)
			if err != nil {
				return fmt.Errorf("verificando otros docentes de la materia %s: %w", materiaID, err)
			}
			if quedaOtro {
				materiasConOtroDocente = append(materiasConOtroDocente, materiaID)
			} else {
				materiasHuerfanas = append(materiasHuerfanas, materiaID)
			}
		}
	}

	if err := s.transicionar(ctx, usuarioID, domain.EstadoBaja); err != nil {
		return err
	}

	if !u.EsDocente() {
		return nil
	}

	// Las cancelaciones van ANTES de borrar los vínculos, y una materia que
	// falla no aborta las demás.
	//
	// Las dos cosas son por lo mismo: esta cascada cruza a reservation por
	// un puerto, con su propia transacción, así que no hay forma de que el
	// conjunto sea atómico sin romper el límite de dominio
	// (docs/06-arquitectura.md §3). Dado eso, lo que se puede elegir es qué
	// queda cuando algo falla. Cortando en la primera materia y con los
	// vínculos ya borrados, quedaba una cuenta en BAJA, sin asignaciones, y
	// con reservas vivas en materias que ya no tienen docente — sin ningún
	// registro de cuáles eran, porque el dato estaba justamente en los
	// vínculos recién borrados, y sin poder reintentar (BAJA→BAJA es una
	// transición inválida, RF-02.9). Ahora se intentan todas, y si alguna
	// falla los vínculos se conservan: el estado es recuperable a mano y el
	// error dice exactamente qué materias quedaron pendientes.
	var fallidas []error
	for _, materiaID := range materiasHuerfanas {
		motivo := "El docente asignado a esta materia fue dado de baja"
		canceladas, err := s.canceladorReservas.CancelarReservasFuturasDeMateria(ctx, materiaID, motivo)
		if err != nil {
			fallidas = append(fallidas, fmt.Errorf("materia %s: %w", materiaID, err))
			continue
		}
		// RF-02.8: aviso a Admin de que se canceló algo en cascada.
		s.bus.Publish(eventbus.Evento{
			Tipo: "docente.baja.materia-huerfana",
			Payload: map[string]any{
				"usuarioId":          usuarioID,
				"materiaId":          materiaID,
				"reservasCanceladas": canceladas,
			},
		})
	}

	if len(fallidas) > 0 {
		return fmt.Errorf("la cuenta quedó en BAJA pero no se pudieron cancelar las reservas de %d materia(s) sin docente; "+
			"sus asignaciones se conservaron para poder resolverlo: %w", len(fallidas), errors.Join(fallidas...))
	}

	if err := s.gestorMaterias.RemoverAsignacionesDeDocente(ctx, usuarioID); err != nil {
		return fmt.Errorf("removiendo asignaciones del docente: %w", err)
	}

	for _, materiaID := range materiasConOtroDocente {
		// RF-05.4: aviso informativo — sigue habiendo otro docente, no se
		// canceló nada.
		s.bus.Publish(eventbus.Evento{
			Tipo:    "docente.baja.notificar_admin",
			Payload: map[string]any{"usuarioId": usuarioID, "materiaId": materiaID},
		})
	}

	return nil
}

// transicionar centraliza la protección del último Admin — cualquier
// transición que saque a un Admin de APROBADA pasa por acá.
// transicionar corre entero dentro de una transacción: el conteo de Admins
// activos y la escritura del nuevo estado tienen que ser atómicos entre sí
// o RF-01.8 se puede violar con dos pedidos concurrentes (ambos ven que
// "quedan 2", ambos pasan, el sistema se queda sin ningún Admin).
func (s *Service) transicionar(ctx context.Context, usuarioID string, nuevo domain.Estado) error {
	return s.repo.EnTransaccion(ctx, func(repo Repo) error {
		u, err := repo.BuscarPorID(ctx, usuarioID)
		if err != nil {
			return err
		}

		if u.EsAdmin() && u.Estado == domain.EstadoAprobada && nuevo != domain.EstadoAprobada {
			if err := s.verificarNoEsUltimoAdmin(ctx, repo); err != nil {
				return err
			}
		}

		if err := u.CambiarEstado(nuevo, s.ahora()); err != nil {
			return err
		}

		return repo.Guardar(ctx, u)
	})
}

func (s *Service) verificarNoEsUltimoAdmin(ctx context.Context, repo Repo) error {
	n, err := repo.ContarAdminsAprobados(ctx)
	if err != nil {
		return fmt.Errorf("contando admins activos: %w", err)
	}
	if n <= 1 {
		return ErrUltimoAdmin
	}
	return nil
}

// ResetearPassword implementa RF-01.6: un Admin resetea la contraseña de
// cualquier usuario a una temporal, y marca DebeCambiarPassword=true.
// Devuelve la temporal en texto plano — la única vez que existe en texto
// plano fuera del proceso de login, para que el Admin se la comunique al
// usuario.
func (s *Service) ResetearPassword(ctx context.Context, usuarioID string) (string, error) {
	u, err := s.repo.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return "", err
	}

	temporal, err := s.generarTemporal()
	if err != nil {
		return "", fmt.Errorf("generando contraseña temporal: %w", err)
	}

	hash, err := s.hash(temporal)
	if err != nil {
		return "", fmt.Errorf("hasheando contraseña temporal: %w", err)
	}

	u.PasswordHash = hash
	u.DebeCambiarPassword = true
	if err := s.repo.Guardar(ctx, u); err != nil {
		return "", err
	}

	return temporal, nil
}

// CambiarPassword implementa RF-01.7: cualquier usuario autenticado puede
// cambiar su propia contraseña, indicando la actual.
//
// Devuelve un token nuevo porque DebeCambiarPassword viaja dentro del JWT
// (ver middleware.Claims): el token con el que se llega acá dice todavía
// "true", así que si el cliente lo siguiera usando quedaría bloqueado por
// su propio cambio exitoso hasta que expirara.
func (s *Service) CambiarPassword(ctx context.Context, usuarioID, passwordActual, passwordNueva string) (string, error) {
	u, err := s.repo.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return "", err
	}

	// Una cuenta creada con Google no tiene contraseña actual que
	// verificar. Acá sí se dice explícitamente (a diferencia del login):
	// quien llega hasta este punto ya está autenticado y es dueño de la
	// cuenta, así que no hay nada que revelarle sobre sí mismo.
	if !u.PuedeIngresarConPassword() {
		return "", ErrCuentaSinPassword
	}

	ok, err := s.verify(passwordActual, u.PasswordHash)
	if err != nil {
		return "", fmt.Errorf("verificando password actual: %w", err)
	}
	if !ok {
		return "", ErrCredencialesInvalidas
	}

	if len(passwordNueva) < minPasswordLen {
		return "", ErrPasswordCorta
	}

	hash, err := s.hash(passwordNueva)
	if err != nil {
		return "", fmt.Errorf("hasheando password nueva: %w", err)
	}

	u.PasswordHash = hash
	u.DebeCambiarPassword = false
	if err := s.repo.Guardar(ctx, u); err != nil {
		return "", err
	}

	token, err := s.firmar(u)
	if err != nil {
		return "", fmt.Errorf("firmando token nuevo: %w", err)
	}
	return token, nil
}

// PromoverAAdmin le da rol ADMIN a un docente ya aprobado. Solo lo puede
// pedir un Admin (lo impone el RBAC de la ruta).
//
// No hace falta transacción ni el guard del último Admin (RF-01.8): esto
// solo agrega Admins, nunca los saca, así que no hay forma de que deje al
// sistema sin ninguno. Es la razón por la que no pasa por transicionar().
//
// Tampoco toca nada más de la cuenta, y eso es deliberado: conserva sus
// materias asignadas y sus reservas. Un docente que pasa a coordinar suele
// seguir dando clase, y el sistema no lo impide — academic solo exige que
// el usuario esté APROBADA para asignarlo a una materia (nunca miró el
// rol), y reservar tampoco pide rol. Borrarle las materias "porque ahora es
// Admin" le cancelaría las clases que ya tiene tomadas por una promoción.
//
// El cambio tiene efecto en el request siguiente, sin volver a iniciar
// sesión: el middleware lee el rol de la base en cada pedido y pisa el del
// token (ver internal/shared/middleware/jwt.go).
func (s *Service) PromoverAAdmin(ctx context.Context, usuarioID string) error {
	u, err := s.repo.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return err
	}

	if err := u.PromoverAAdmin(); err != nil {
		return err
	}

	return s.repo.Guardar(ctx, u)
}

// EliminarDefinitivamente implementa RF-01.9: hard delete, solo permitido
// desde estado BAJA.
func (s *Service) EliminarDefinitivamente(ctx context.Context, usuarioID string) error {
	u, err := s.repo.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return err
	}
	if u.Estado != domain.EstadoBaja {
		return ErrSoloDesdeBaja
	}
	return s.repo.Eliminar(ctx, usuarioID)
}

// Listar devuelve una página de usuarios filtrados por estado/rol (nil = sin
// filtrar por ese campo), más el total que matchean. No hay lógica de negocio
// propia acá — el RBAC de la ruta ya restringe esto a Admin — salvo completar
// la ventana: una Pagina en cero daría LIMIT 0, o sea una lista vacía sin
// ningún error.
func (s *Service) Listar(ctx context.Context, filtroEstado *domain.Estado, filtroRol *domain.Rol, pagina paginacion.Pagina) ([]*domain.Usuario, int, error) {
	if pagina.Tamanio <= 0 || pagina.Numero <= 0 {
		pagina = paginacion.PorDefecto()
	}
	return s.repo.Listar(ctx, filtroEstado, filtroRol, pagina)
}

// ObtenerPerfil devuelve los datos del propio usuario autenticado
// (GET /api/auth/me). Passthrough directo — cualquier usuario puede ver
// su propio perfil, no hay regla de negocio que aplicar acá.
func (s *Service) ObtenerPerfil(ctx context.Context, usuarioID string) (*domain.Usuario, error) {
	return s.repo.BuscarPorID(ctx, usuarioID)
}
