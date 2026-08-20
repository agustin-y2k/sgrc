// Package application orquesta los casos de uso de RF-01 (usuarios y
// autenticación) y la parte de RF-02 que le toca: aprobación de cuentas y la
// cascada de DarDeBaja, que cruza a academic y a reservation por puertos (ver
// ports.go) sin importar ninguno de los dos.
package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ramiro/sgrc/internal/auth/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
	"github.com/ramiro/sgrc/internal/shared/paginacion"
)

// Service implementa los casos de uso de auth.
type Service struct {
	repo               Repo
	bus                eventbus.EventBus
	hash               HashFunc
	verify             VerifyFunc
	firmar             TokenSigner
	nuevoID            IDGenerator
	generarTemporal    GenerarTemporalFunc
	generarCodigo      GenerarCodigoFunc
	ahora              func() time.Time
	gestorMaterias     GestorMateriasDocente
	canceladorReservas CanceladorReservasDeMateria

	// correoHabilitado dice si el despliegue configuró SMTP. La recuperación de
	// contraseña por autoservicio depende enteramente de poder mandar un mail,
	// así que sin correo sus dos endpoints responden 503 en vez de aceptar un
	// pedido que no va a llegar a ningún lado (ver service_recuperacion.go).
	correoHabilitado bool

	// verificadorGoogle es nil cuando el despliegue no configuró
	// GOOGLE_CLIENT_ID. Es opcional a propósito: el sistema tiene que arrancar y
	// funcionar igual sin ingreso con Google.
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
	generarCodigo GenerarCodigoFunc,
	ahora func() time.Time,
	gestorMaterias GestorMateriasDocente,
	canceladorReservas CanceladorReservasDeMateria,
	verificadorGoogle VerificadorGoogle,
	correoHabilitado bool,
) *Service {
	return &Service{
		repo:               repo,
		bus:                bus,
		hash:               hash,
		verify:             verify,
		firmar:             firmar,
		nuevoID:            nuevoID,
		generarTemporal:    generarTemporal,
		generarCodigo:      generarCodigo,
		ahora:              ahora,
		gestorMaterias:     gestorMaterias,
		canceladorReservas: canceladorReservas,
		verificadorGoogle:  verificadorGoogle,
		correoHabilitado:   correoHabilitado,
	}
}

// RecuperacionPorEmailDisponible es lo que GET /api/auth/config le informa al
// frontend para decidir si dibuja el enlace "olvidé mi contraseña".
func (s *Service) RecuperacionPorEmailDisponible() bool { return s.correoHabilitado }

// SolicitudDeAsignacion es lo que se declara al registrarse: con qué cargo, si
// se ofrece como titular o suplente, y —solo si da clase— qué curso y qué
// materia.
type SolicitudDeAsignacion struct {
	Curso   string
	Materia string
	// Rol es "TITULAR" o "SUPLENTE". Obligatorio en el registro (lo declaran
	// los dos cargos); vacío solo en las cuentas que no se autorregistran.
	Rol string
	// Cargo es "DOCENTE" o "ADMIN_SISTEMA". Obligatorio en el registro. NO
	// otorga permisos: ver domain.NormalizarCargoSolicitado.
	Cargo string
}

// exigirCargoYRol es lo que separa al registro de las otras formas de crear
// una cuenta. Vive acá y no en crearUsuario porque esa función también la usa
// CrearAdmin (RF-01.4), donde no hay nadie declarando nada.
func exigirCargoYRol(solicitud SolicitudDeAsignacion) error {
	switch strings.TrimSpace(solicitud.Cargo) {
	case domain.CargoSolicitadoDocente, domain.CargoSolicitadoAdminSistema:
	default:
		return ErrCargoObligatorio
	}
	switch strings.TrimSpace(solicitud.Rol) {
	case domain.RolSolicitadoTitular, domain.RolSolicitadoSuplente:
	default:
		return ErrRolSolicitadoObligatorio
	}
	return nil
}

// soloLoQueDeclaraEseCargo descarta el curso y la materia de quien no se
// registró para dar clase. El formulario ya no los muestra, pero eso es una
// decisión del navegador: si igual llegan en el cuerpo, no se guardan.
func soloLoQueDeclaraEseCargo(solicitud SolicitudDeAsignacion) SolicitudDeAsignacion {
	if strings.TrimSpace(solicitud.Cargo) == domain.CargoSolicitadoAdminSistema {
		solicitud.Curso = ""
		solicitud.Materia = ""
	}
	return solicitud
}

// Registrar implementa RF-01.3: autorregistro de docente, queda PENDIENTE. La
// solicitud viaja con el registro para que el Admin, al aprobar, sepa a qué
// materia y curso asignarlo sin tener que preguntárselo por fuera del sistema
// (y para que sepa si tiene que crearlos antes).
func (s *Service) Registrar(ctx context.Context, nombre, apellido, email, password string, solicitud SolicitudDeAsignacion) (*domain.Usuario, error) {
	if err := exigirCargoYRol(solicitud); err != nil {
		return nil, err
	}

	u, err := s.crearUsuario(ctx, nombre, apellido, email, password, domain.RolDocente, domain.EstadoPendiente, nil,
		soloLoQueDeclaraEseCargo(solicitud))
	if err != nil {
		return nil, err
	}

	s.avisarQueHayUnDocentePendiente(u)
	return u, nil
}

// avisarQueHayUnDocentePendiente publica RF-05.6 — el aviso a todos los Admin
// de que alguien se registró y está esperando aprobación.
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

// CrearAdmin implementa RF-01.4: un Admin crea otro Admin directamente, sin
// pasar por PENDIENTE — queda APROBADA de inmediato, con AprobadoPor
// apuntando a quien lo creó.
func (s *Service) CrearAdmin(ctx context.Context, creadoPorID, nombre, apellido, email, password string) (*domain.Usuario, error) {
	// Un Admin no declara curso, materia ni rol: no se autorregistra para
	// dictar, lo crea otro Admin ya aprobado.
	return s.crearUsuario(ctx, nombre, apellido, email, password, domain.RolAdmin, domain.EstadoAprobada, &creadoPorID, SolicitudDeAsignacion{})
}

// crearUsuario centraliza la validación y creación común a Registrar y
// CrearAdmin — lo único que cambia entre los dos casos es el rol, el estado
// inicial, y si hay que dejar registro de quién aprobó.
func (s *Service) crearUsuario(ctx context.Context, nombre, apellido, email, password string, rol domain.Rol, estadoInicial domain.Estado, aprobadoPor *string, solicitud SolicitudDeAsignacion) (*domain.Usuario, error) {
	// El email se normaliza ANTES de buscarlo y de guardarlo, así la comparación
	// contra lo que ya existe y lo que termina en la fila usan la misma forma
	// (ver domain.NormalizarEmail).
	email = domain.NormalizarEmail(email)

	nombre, apellido, errNombre := domain.NormalizarNombreYApellido(nombre, apellido)
	if errors.Is(errNombre, domain.ErrNombreVacio) || email == "" {
		// ErrDatosObligatorios y no el error del dominio: acá los campos
		// obligatorios son tres, y el mensaje tiene que nombrarlos a los tres.
		return nil, ErrDatosObligatorios
	}
	if errNombre != nil {
		return nil, errNombre
	}
	if err := domain.ValidarEmail(email); err != nil {
		return nil, err
	}
	if len(password) < minPasswordLen {
		return nil, ErrPasswordCorta
	}
	rolSolicitado, err := domain.NormalizarRolSolicitado(strings.TrimSpace(solicitud.Rol))
	if err != nil {
		return nil, err
	}
	cargoSolicitado, err := domain.NormalizarCargoSolicitado(strings.TrimSpace(solicitud.Cargo))
	if err != nil {
		return nil, err
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

		CursoSolicitado:   strings.TrimSpace(solicitud.Curso),
		MateriaSolicitada: strings.TrimSpace(solicitud.Materia),
		RolSolicitado:     rolSolicitado,
		CargoSolicitado:   cargoSolicitado,
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
	// Misma normalización que en crearUsuario: sin esto, quien se registró como
	// "Juan.Perez@…" y escribe "juan.perez@…" al entrar recibía "credenciales
	// inválidas" aunque la contraseña fuera correcta.
	u, err := s.repo.BuscarPorEmail(ctx, domain.NormalizarEmail(email))
	if err != nil {
		if errors.Is(err, ErrUsuarioNoEncontrado) {
			// El mensaje ya no revelaba si el email existe, pero el tiempo sí: con
			// email inexistente se volvía de inmediato, y con email real se corría
			// argon2id con 64 MB. La diferencia es de decenas de milisegundos, trivial
			// de medir en un loop, y alcanza para enumerar quién tiene cuenta en la
			// escuela.
			s.consumirTiempoDeVerificacion(password)
			return nil, ErrCredencialesInvalidas
		}
		return nil, fmt.Errorf("buscando usuario: %w", err)
	}

	// Una cuenta creada con Google no tiene contraseña contra la cual verificar.
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
		return nil, motivoPorElQueNoEntra(u.Estado)
	}

	token, err := s.firmar(u)
	if err != nil {
		return nil, fmt.Errorf("firmando token: %w", err)
	}

	return &LoginResultado{Token: token, DebeCambiarPassword: u.DebeCambiarPassword}, nil
}

// LoginConGoogle implementa el ingreso con una cuenta de Google ya existente
// en el sistema.
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
	// entra con Google por primera vez queda vinculada igual, así el día que el
	// Admin la apruebe funciona sin que la persona tenga que repetir nada.
	if !u.EstaAprobado() {
		return nil, motivoPorElQueNoEntra(u.Estado)
	}

	token, err := s.firmar(u)
	if err != nil {
		return nil, fmt.Errorf("firmando token: %w", err)
	}
	return &LoginResultado{Token: token, DebeCambiarPassword: u.DebeCambiarPassword}, nil
}

// cuentaParaIdentidadGoogle resuelve a qué usuario corresponde una identidad
// de Google, vinculándola si hace falta.
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
	// reactiva por la puerta de atrás.
	if u.Estado == domain.EstadoBaja {
		return nil, ErrIngresoCuentaEnBaja
	}

	u.GoogleSub = identidad.Sub
	if err := s.repo.Guardar(ctx, u); err != nil {
		return nil, fmt.Errorf("vinculando la cuenta de Google: %w", err)
	}
	return u, nil
}

// RegistrarConGoogle crea una cuenta de docente a partir de un ID token de
// Google.
func (s *Service) RegistrarConGoogle(ctx context.Context, idToken, nombre, apellido string, solicitud SolicitudDeAsignacion) (*domain.Usuario, error) {
	// El orden importa: primero el token. Si este despliegue no tiene el
	// ingreso con Google configurado, la respuesta tiene que ser "el sistema no
	// hace esto" (503) y no "te faltó elegir un cargo" (400), que mandaría a
	// corregir un formulario que igual no iba a funcionar.
	identidad, err := s.identidadDeGoogle(ctx, idToken)
	if err != nil {
		return nil, err
	}

	if err := exigirCargoYRol(solicitud); err != nil {
		return nil, err
	}
	solicitud = soloLoQueDeclaraEseCargo(solicitud)

	email := domain.NormalizarEmail(identidad.Email)

	nombre, apellido, errNombre := domain.NormalizarNombreYApellido(
		primeroNoVacio(strings.TrimSpace(nombre), identidad.Nombre),
		primeroNoVacio(strings.TrimSpace(apellido), identidad.Apellido),
	)
	if errors.Is(errNombre, domain.ErrNombreVacio) || email == "" {
		return nil, ErrDatosObligatorios
	}
	if errNombre != nil {
		return nil, errNombre
	}
	if err := domain.ValidarEmail(email); err != nil {
		return nil, err
	}
	rolSolicitado, err := domain.NormalizarRolSolicitado(strings.TrimSpace(solicitud.Rol))
	if err != nil {
		return nil, err
	}
	cargoSolicitado, err := domain.NormalizarCargoSolicitado(strings.TrimSpace(solicitud.Cargo))
	if err != nil {
		return nil, err
	}

	// Dos consultas y no una: alguien puede tener ya la cuenta vinculada (sub
	// conocido) y volver a caer acá por un reintento del frontend, y también
	// puede existir una cuenta con ese email pero sin vincular.
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
		// La cuenta existe con ese email pero sin vincular.
		return nil, ErrEmailYaRegistrado
	}

	ahora := s.ahora()
	u := &domain.Usuario{
		ID:        s.nuevoID(),
		Nombre:    nombre,
		Apellido:  apellido,
		Email:     email,
		GoogleSub: identidad.Sub,
		// Sin PasswordHash a propósito: no hay ninguna contraseña que la persona
		// haya elegido, y poner una aleatoria que nadie conoce sería mentirle al
		// esquema sobre lo que la cuenta puede hacer.
		Rol:           domain.RolDocente,
		Estado:        domain.EstadoPendiente,
		FechaRegistro: ahora,

		CursoSolicitado:   strings.TrimSpace(solicitud.Curso),
		MateriaSolicitada: strings.TrimSpace(solicitud.Materia),
		RolSolicitado:     rolSolicitado,
		CargoSolicitado:   cargoSolicitado,
	}

	if err := s.repo.Crear(ctx, u); err != nil {
		return nil, fmt.Errorf("creando usuario: %w", err)
	}

	s.avisarQueHayUnDocentePendiente(u)
	return u, nil
}

// identidadDeGoogle centraliza lo que los dos casos de uso necesitan antes de
// tocar la base: que el ingreso con Google esté configurado, que el token sea
// creíble, y que Google confirme el email.
func (s *Service) identidadDeGoogle(ctx context.Context, idToken string) (*IdentidadGoogle, error) {
	if s.verificadorGoogle == nil {
		return nil, ErrLoginGoogleNoDisponible
	}
	if strings.TrimSpace(idToken) == "" {
		return nil, ErrTokenGoogleInvalido
	}

	identidad, err := s.verificadorGoogle.Verificar(ctx, idToken)
	if err != nil {
		// ErrDominioNoPermitido y ErrTokenGoogleInvalido ya son errores de negocio
		// con su propio código HTTP: se dejan pasar tal cual.
		if errors.Is(err, ErrDominioNoPermitido) || errors.Is(err, ErrTokenGoogleInvalido) {
			return nil, err
		}
		return nil, fmt.Errorf("verificando el token de Google: %w", err)
	}

	if !identidad.EmailVerificado {
		return nil, ErrEmailNoVerificadoPorGoogle
	}
	if strings.TrimSpace(identidad.Sub) == "" || strings.TrimSpace(identidad.Email) == "" {
		// Un token sin sub o sin email pasó la firma pero no sirve para identificar
		// a nadie.
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

// motivoPorElQueNoEntra traduce el estado de una cuenta que no puede ingresar
// al error que se lo explica a su dueño.
func motivoPorElQueNoEntra(estado domain.Estado) error {
	switch estado {
	case domain.EstadoPendiente:
		return ErrIngresoCuentaPendiente
	case domain.EstadoRechazada:
		return ErrIngresoCuentaRechazada
	case domain.EstadoBaja:
		return ErrIngresoCuentaEnBaja
	default:
		return ErrCuentaNoHabilitada
	}
}

// consumirTiempoDeVerificacion corre el mismo argon2id que haría un login
// real, contra un hash que no le pertenece a nadie.
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
	s.avisarALaPersonaQueLaAprobaron(ctx, usuarioID)
	return nil
}

// avisarALaPersonaQueLaAprobaron publica el evento con el que se le manda un
// mail al docente contándole que ya puede entrar.
func (s *Service) avisarALaPersonaQueLaAprobaron(ctx context.Context, usuarioID string) {
	u, err := s.repo.BuscarPorID(ctx, usuarioID)
	if err != nil {
		log.Printf("auth: cuenta %s aprobada, pero no se pudo leer para avisarle por mail: %v", usuarioID, err)
		return
	}
	s.bus.Publish(eventbus.Evento{
		Tipo: "cuenta.aprobada",
		Payload: eventbus.CuentaAprobada{
			UsuarioID: u.ID,
			Email:     u.Email,
			Nombre:    u.Nombre,
		},
	})
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
func (s *Service) avisarQueYaNoEstaPendiente(usuarioID string) {
	s.bus.Publish(eventbus.Evento{
		Tipo:    "cuenta.pendiente.resuelta",
		Payload: map[string]string{"usuarioId": usuarioID},
	})
}

// DarDeBaja implementa el cambio de estado de RF-02.8/02.9, más la cascada
// completa: si el usuario es docente, identifica sus materias ANTES de
// aplicar la transición (mientras las filas docente_materia todavía existen),
// y para cada una donde no quede ningún otro docente APROBADA asignado,
// cancela sus reservas futuras vía
// canceladorReservas.CancelarReservasFuturasDeMateria y publica un evento
// para que internal/notification (cuando exista) avise a los Admin.
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
	// RF-01.11: el caso que motiva un reset asistido es alguien que perdió el
	// control de su cuenta, así que dejar viva la sesión de quien entró con la
	// contraseña vieja haría que el reset no sirva de nada.
	u.InvalidarSesiones()
	if err := s.repo.Guardar(ctx, u); err != nil {
		return "", err
	}

	return temporal, nil
}

// CambiarPassword implementa RF-01.7: cualquier usuario autenticado puede
// cambiar su propia contraseña, indicando la actual.
func (s *Service) CambiarPassword(ctx context.Context, usuarioID, passwordActual, passwordNueva string) (string, error) {
	u, err := s.repo.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return "", err
	}

	// Una cuenta creada con Google no tiene contraseña actual que verificar.
	if !u.PuedeIngresarConPassword() {
		return "", ErrCuentaSinPassword
	}

	ok, err := s.verify(passwordActual, u.PasswordHash)
	if err != nil {
		return "", fmt.Errorf("verificando password actual: %w", err)
	}
	if !ok {
		return "", ErrPasswordActualIncorrecta
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
	// El orden importa: invalidar ANTES de firmar.
	u.InvalidarSesiones()
	if err := s.repo.Guardar(ctx, u); err != nil {
		return "", err
	}

	token, err := s.firmar(u)
	if err != nil {
		return "", fmt.Errorf("firmando token nuevo: %w", err)
	}
	return token, nil
}

// PromoverAAdmin le da rol ADMIN a un docente ya aprobado.
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

// DegradarADocente es la inversa de PromoverAAdmin: le saca los permisos de
// Admin a alguien que sigue siendo docente, sin cerrarle la cuenta.
func (s *Service) DegradarADocente(ctx context.Context, usuarioID, solicitanteID string) error {
	if usuarioID == solicitanteID {
		return ErrAutoDegradacion
	}

	return s.repo.EnTransaccion(ctx, func(repo Repo) error {
		u, err := repo.BuscarPorID(ctx, usuarioID)
		if err != nil {
			return err
		}

		// Contar antes de tocar nada, igual que en transicionar(): el guard mira
		// cuántos Admins activos hay, y esta cuenta todavía es uno de ellos.
		if u.EsAdmin() && u.EstaAprobado() {
			if err := s.verificarNoEsUltimoAdmin(ctx, repo); err != nil {
				return err
			}
		}

		if err := u.DegradarADocente(); err != nil {
			return err
		}

		return repo.Guardar(ctx, u)
	})
}

// EliminarDefinitivamente implementa RF-01.9: hard delete desde cualquiera de
// los dos estados terminales, BAJA o RECHAZADA. Que RECHAZADA cuente es lo
// que hace que rechazar deje de ser una trampa.
func (s *Service) EliminarDefinitivamente(ctx context.Context, usuarioID string) error {
	u, err := s.repo.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return err
	}
	if u.Estado != domain.EstadoBaja && u.Estado != domain.EstadoRechazada {
		return ErrSoloDesdeBajaORechazada
	}
	return s.repo.Eliminar(ctx, usuarioID)
}

// Listar devuelve una página de usuarios filtrados por estado/rol (nil = sin
// filtrar por ese campo), más el total que matchean.
func (s *Service) Listar(ctx context.Context, filtroEstado *domain.Estado, filtroRol *domain.Rol, pagina paginacion.Pagina) ([]*domain.Usuario, int, error) {
	if pagina.Tamanio <= 0 || pagina.Numero <= 0 {
		pagina = paginacion.PorDefecto()
	}
	return s.repo.Listar(ctx, filtroEstado, filtroRol, pagina)
}

// ObtenerPerfil devuelve los datos del propio usuario autenticado (GET
// /api/auth/me).
func (s *Service) ObtenerPerfil(ctx context.Context, usuarioID string) (*domain.Usuario, error) {
	return s.repo.BuscarPorID(ctx, usuarioID)
}
