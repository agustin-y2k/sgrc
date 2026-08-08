package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// Casos de uso de las licencias de software (RF-03.11 a RF-03.14).
//
// Están en su propio archivo y no en service.go porque son los únicos del
// paquete que operan en LOTE: el mismo AutoCAD vive en las ocho PCs de un
// carro y se renueva de una sola vez. Esa forma —"esto salió bien para
// estas, se salteó para aquellas"— no la tiene ningún otro caso de uso de
// inventory.

var (
	// ErrSinEquipos: un alta masiva sin ninguna PC no es un error de la base ni
	// del dominio, es un request vacío.
	ErrSinEquipos = errors.New("hay que indicar al menos un equipo")
	// ErrSinLicencias: ídem para la renovación masiva.
	ErrSinLicencias = errors.New("hay que indicar al menos una licencia")

	// ErrVencimientoAmbiguo: se declaró el vencimiento de más de una forma
	// a la vez. Las tres son válidas por separado y darían fechas
	// distintas; adivinar cuál gana sería elegir por el Admin.
	ErrVencimientoAmbiguo = errors.New("el vencimiento se indica de una sola forma: " +
		"la fecha en que se renovó, los días que le quedan, o la fecha en que vence")
)

// VencimientoDeclarado es CÓMO el Admin dice cuándo vence, no cuándo vence.
//
// Son tres campos y no uno porque el dato llega en tres formas según dónde
// esté parado quien carga, y obligarlo a convertir de una a otra en la
// cabeza es la manera más segura de que entren fechas equivocadas:
//
//	RenovadaEl  "la renové el martes"      → martes + DiasDuracion
//	QuedanDias  "quedan 12 días"           → hoy + 12   (lo que dice la máquina)
//	VenceEl     "vence el 3 de septiembre" → esa fecha
//
// Los tres nil significan "todavía no sé", que es un estado legítimo y el
// que tiene una licencia recién cargada. Ver la migración 012.
type VencimientoDeclarado struct {
	RenovadaEl *time.Time
	QuedanDias *int
	VenceEl    *time.Time
}

func (v VencimientoDeclarado) declarado() bool {
	return v.RenovadaEl != nil || v.QuedanDias != nil || v.VenceEl != nil
}

func (v VencimientoDeclarado) formasDeclaradas() int {
	n := 0
	for _, declarada := range []bool{v.RenovadaEl != nil, v.QuedanDias != nil, v.VenceEl != nil} {
		if declarada {
			n++
		}
	}
	return n
}

// aplicarA fija el vencimiento de la licencia según la forma declarada. Sin
// declaración no toca nada: la licencia queda (o sigue) sin fecha.
func (v VencimientoDeclarado) aplicarA(l *domain.LicenciaSoftware, hoy, ahora time.Time, porUsuario string) error {
	if v.formasDeclaradas() > 1 {
		return ErrVencimientoAmbiguo
	}
	switch {
	case v.RenovadaEl != nil:
		l.RenovadaEl(*v.RenovadaEl, porUsuario, ahora)
	case v.QuedanDias != nil:
		return l.VenceEnDias(*v.QuedanDias, hoy, porUsuario, ahora)
	case v.VenceEl != nil:
		l.FijarVencimiento(*v.VenceEl, porUsuario, ahora)
	}
	return nil
}

// Hoy es la fecha de la escuela, recortada a día. s.ahora ya viene con la
// zona de APP_TIMEZONE desde cmd/main.go; recortarla acá es lo que hace que
// el contador no dependa de la hora a la que se mire.
func (s *Service) Hoy() time.Time {
	return domain.Dia(s.ahora())
}

// ── Alta ────────────────────────────────────────────────────────────────

// NuevaLicenciaParams es el alta de UNA licencia sobre VARIAS PCs. Todas
// comparten nombre, duración, antelación y vencimiento: el caso real es
// "AutoCAD, 30 días, en estas ocho máquinas".
type NuevaLicenciaParams struct {
	EquipoIDs    []string
	Nombre       string
	DiasDuracion int
	DiasAviso    int
	Vencimiento  VencimientoDeclarado
	PorUsuario   string
}

// ResultadoAltaMasiva separa lo que se creó de lo que ya estaba.
type ResultadoAltaMasiva struct {
	Creadas []*domain.LicenciaSoftware
	// EquiposQueYaLaTenian son las que ya tenían cargado ese software. No es un
	// error: el caso normal es marcar las diez PCs del carro cuando ocho ya
	// estaban cargadas de antes.
	EquiposQueYaLaTenian []string
}

// CrearLicencias da de alta la misma licencia en varios equipos.
//
// Una PC que ya la tiene se saltea y se informa, en vez de abortar el lote.
// Eso hace que la operación sea REINTENTABLE: si algo falla en la PC número
// seis, volver a mandar el mismo request completa lo que falta sin duplicar
// lo que ya entró. Por eso tampoco hay transacción — las que se crearon se
// quedan creadas a propósito.
func (s *Service) CrearLicencias(ctx context.Context, params NuevaLicenciaParams) (*ResultadoAltaMasiva, error) {
	if len(params.EquipoIDs) == 0 {
		return nil, ErrSinEquipos
	}
	if params.Vencimiento.formasDeclaradas() > 1 {
		return nil, ErrVencimientoAmbiguo
	}

	ahora := s.ahora()
	hoy := s.Hoy()
	resultado := &ResultadoAltaMasiva{}

	for _, equipoID := range params.EquipoIDs {
		l, err := domain.NuevaLicencia(s.nuevoID(), equipoID, params.Nombre,
			params.DiasDuracion, params.DiasAviso, ahora)
		if err != nil {
			// El nombre y los días son los mismos para todas: si la
			// validación falla, falla para el lote entero y no tiene
			// sentido seguir intentando con las demás PCs.
			return nil, err
		}
		if err := params.Vencimiento.aplicarA(l, hoy, ahora, params.PorUsuario); err != nil {
			return nil, err
		}

		if err := s.repo.CrearLicencia(ctx, l); err != nil {
			if errors.Is(err, ErrLicenciaDuplicada) {
				resultado.EquiposQueYaLaTenian = append(resultado.EquiposQueYaLaTenian, equipoID)
				continue
			}
			return nil, fmt.Errorf("creando la licencia en el equipo %s: %w", equipoID, err)
		}
		resultado.Creadas = append(resultado.Creadas, l)
	}

	return resultado, nil
}

// ── Renovación ──────────────────────────────────────────────────────────

// ResultadoRenovacion separa lo renovado de lo que no se pudo renovar.
type ResultadoRenovacion struct {
	Renovadas []*domain.LicenciaSoftware
	// SinFechaPrevia son las que todavía no tienen vencimiento cargado.
	// Renovar mueve un contador que ya existe; una licencia sin fecha hay
	// que cargarla diciendo cómo se sabe, no "renovarla" (ver
	// domain.LicenciaSoftware.Renovar). Se informan en vez de abortar el
	// lote, por lo mismo que el alta.
	SinFechaPrevia []string
}

// RenovarLicencias corre el vencimiento de varias licencias de una vez.
//
// renovadaEl nil significa "hoy" — el botón que se aprieta el 99% de las
// veces. Con fecha, es el caso del olvido: "las renové el martes y recién
// hoy lo cargo".
func (s *Service) RenovarLicencias(ctx context.Context, ids []string, renovadaEl *time.Time, porUsuario string) (*ResultadoRenovacion, error) {
	if len(ids) == 0 {
		return nil, ErrSinLicencias
	}

	ahora := s.ahora()
	fechaRenovacion := s.Hoy()
	if renovadaEl != nil {
		fechaRenovacion = domain.Dia(*renovadaEl)
	}

	resultado := &ResultadoRenovacion{}
	for _, id := range ids {
		l, err := s.repo.BuscarLicenciaPorID(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := l.Renovar(fechaRenovacion, porUsuario, ahora); err != nil {
			if errors.Is(err, domain.ErrSinFechaDeVencimiento) {
				resultado.SinFechaPrevia = append(resultado.SinFechaPrevia, id)
				continue
			}
			return nil, err
		}
		if err := s.repo.GuardarLicencia(ctx, l); err != nil {
			return nil, fmt.Errorf("guardando la renovación de la licencia %s: %w", id, err)
		}
		resultado.Renovadas = append(resultado.Renovadas, l)
	}

	return resultado, nil
}

// ── Edición ─────────────────────────────────────────────────────────────

// EditarLicenciaParams — nil significa "no tocar ese campo", igual que en
// EditarEquipo. Vencimiento sin ninguna forma declarada tampoco toca la fecha.
type EditarLicenciaParams struct {
	Nombre       *string
	DiasDuracion *int
	DiasAviso    *int
	Vencimiento  VencimientoDeclarado
	PorUsuario   string
}

// EditarLicencia es el "editar el contador en cualquier momento" del
// requerimiento: corregir una fecha que quedó mal, cambiar la duración de
// 30 a 60 días, o cargar por primera vez el vencimiento de una licencia que
// se dio de alta sin él.
//
// Cambiar DiasDuracion NO recalcula el vencimiento vigente (ver
// domain.CambiarDuracion). Para recalcularlo, la pantalla manda además
// Vencimiento.RenovadaEl con la última renovación conocida — explícito, no
// como efecto colateral de tocar otro campo.
func (s *Service) EditarLicencia(ctx context.Context, licenciaID string, params EditarLicenciaParams) error {
	l, err := s.repo.BuscarLicenciaPorID(ctx, licenciaID)
	if err != nil {
		return err
	}

	if params.Nombre != nil {
		if err := l.RenombrarA(*params.Nombre); err != nil {
			return err
		}
	}
	if params.DiasDuracion != nil {
		if err := l.CambiarDuracion(*params.DiasDuracion); err != nil {
			return err
		}
	}
	if params.DiasAviso != nil {
		if err := l.CambiarDiasAviso(*params.DiasAviso); err != nil {
			return err
		}
	}
	// Va último a propósito: si en el mismo request cambian la duración y
	// declaran "renovada el martes", el vencimiento nuevo tiene que salir
	// de la duración nueva.
	if params.Vencimiento.declarado() {
		if err := params.Vencimiento.aplicarA(l, s.Hoy(), s.ahora(), params.PorUsuario); err != nil {
			return err
		}
	}

	return s.repo.GuardarLicencia(ctx, l)
}

// ── Lecturas y baja ─────────────────────────────────────────────────────

// ListarLicencias trae todas con su ubicación, ya ordenadas para la
// pantalla: primero las que no tienen fecha, después de la más vencida a la
// que más le falta.
func (s *Service) ListarLicencias(ctx context.Context) ([]*LicenciaConUbicacion, error) {
	return s.repo.ListarLicencias(ctx)
}

func (s *Service) ListarLicenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.LicenciaSoftware, error) {
	return s.repo.ListarLicenciasPorEquipo(ctx, equipoID)
}

// BorrarLicencia elimina la fila. Borrado real y no baja lógica: de una
// licencia no cuelga nada —ni reservas, ni incidencias, ni historial— así
// que conservarla solo dejaría ruido en la pantalla.
func (s *Service) BorrarLicencia(ctx context.Context, licenciaID string) error {
	return s.repo.BorrarLicencia(ctx, licenciaID)
}
