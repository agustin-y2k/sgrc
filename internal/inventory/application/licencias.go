package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
	"github.com/ramiro/sgrc/internal/shared/eventbus"
)

// Casos de uso de las licencias de software (RF-03.11 a RF-03.14).

var (
	// ErrSinEquipos: un alta masiva sin ninguna PC no es un error de la base ni
	// del dominio, es un request vacío.
	ErrSinEquipos = errors.New("hay que indicar al menos un equipo")
	// ErrSinLicencias: ídem para la renovación masiva.
	ErrSinLicencias = errors.New("hay que indicar al menos una licencia")

	// ErrVencimientoAmbiguo: se declaró el vencimiento de más de una forma a la
	// vez.
	ErrVencimientoAmbiguo = errors.New("el vencimiento se indica de una sola forma: " +
		"la fecha en que se renovó, los días que le quedan, o la fecha en que vence")
)

// VencimientoDeclarado es CÓMO el Admin dice cuándo vence, no cuándo vence.
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

// Hoy es la fecha de la escuela, recortada a día.
func (s *Service) Hoy() time.Time {
	return domain.Dia(s.ahora())
}

// ── Alta ────────────────────────────────────────────────────────────────

// NuevaLicenciaParams es el alta de UNA licencia sobre VARIAS PCs.
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
	// EquiposQueYaLaTenian son las que ya tenían cargado ese software.
	EquiposQueYaLaTenian []string
}

// CrearLicencias da de alta la misma licencia en varios equipos.
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

	// Todo el lote en una transacción: un alta de sesenta máquinas que falla en
	// la cuarenta dejaba las treinta y nueve anteriores cargadas y devolvía un
	// error, sin decir cuáles. Revisar equipo por equipo para averiguarlo es
	// justo el trabajo que el alta masiva viene a evitar.
	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		// Se reinicia adentro: si la transacción se reintentara, el resultado no
		// puede arrastrar lo contado en el intento anterior.
		resultado = &ResultadoAltaMasiva{}

		for _, equipoID := range params.EquipoIDs {
			l, err := domain.NuevaLicencia(s.nuevoID(), equipoID, params.Nombre,
				params.DiasDuracion, params.DiasAviso, ahora)
			if err != nil {
				// El nombre y los días son los mismos para todas: si la validación
				// falla, falla para el lote entero y no tiene sentido seguir
				// intentando con las demás PCs.
				return err
			}
			if err := params.Vencimiento.aplicarA(l, hoy, ahora, params.PorUsuario); err != nil {
				return err
			}

			creada, err := repo.CrearLicencia(ctx, l)
			if err != nil {
				return fmt.Errorf("creando la licencia en el equipo %s: %w", equipoID, err)
			}
			if !creada {
				// Que ese equipo ya la tuviera no es un error y no voltea el lote:
				// se informa aparte y se sigue con el siguiente.
				resultado.EquiposQueYaLaTenian = append(resultado.EquiposQueYaLaTenian, equipoID)
				continue
			}
			resultado.Creadas = append(resultado.Creadas, l)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return resultado, nil
}

// ── Renovación ──────────────────────────────────────────────────────────

// ResultadoRenovacion separa lo renovado de lo que no se pudo renovar.
type ResultadoRenovacion struct {
	Renovadas []*domain.LicenciaSoftware
	// SinFechaPrevia son las que todavía no tienen vencimiento cargado.
	SinFechaPrevia []string
}

// RenovarLicencias corre el vencimiento de varias licencias de una vez.
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

	// Mismo motivo que el alta: renovar treinta licencias y que se guarden
	// diecisiete deja al Admin sin saber cuáles quedaron sin renovar, y la
	// pantalla no se lo puede decir.
	err := s.repo.EnTransaccion(ctx, func(repo Repo) error {
		resultado = &ResultadoRenovacion{}

		// El lote entero de una: preguntar de a una eran treinta viajes a la
		// base con la conexión de la transacción tomada todo ese tiempo.
		licencias, err := repo.BuscarLicenciasPorIDs(ctx, ids)
		if err != nil {
			return err
		}

		// Se recorre `ids` y no el mapa: el orden del resultado tiene que ser el
		// que mandó quien pidió la renovación, no el que devuelva la base.
		for _, id := range ids {
			l, existe := licencias[id]
			if !existe {
				return ErrLicenciaNoEncontrada
			}
			if err := l.Renovar(fechaRenovacion, porUsuario, ahora); err != nil {
				if errors.Is(err, domain.ErrSinFechaDeVencimiento) {
					// No tiene fecha cargada todavía: no hay nada que renovar y no es
					// un error del lote. Se informa aparte.
					resultado.SinFechaPrevia = append(resultado.SinFechaPrevia, id)
					continue
				}
				return err
			}
			if err := repo.GuardarLicencia(ctx, l); err != nil {
				return fmt.Errorf("guardando la renovación de la licencia %s: %w", id, err)
			}
			resultado.Renovadas = append(resultado.Renovadas, l)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Fuera de la transacción a propósito: el aviso es una consecuencia de que
	// la renovación ya esté firme, y un fallo al contar no puede voltearla.
	s.avisarSiNoQuedanPendientes(ctx)
	return resultado, nil
}

// avisarSiNoQuedanPendientes publica cuántas licencias siguen por vencer o
// vencidas después de este cambio. Con cero, el aviso de la campana se cierra
// para todos los Admin (ver notification).
//
// Se publica SIEMPRE el número y no solo el cero: quien escucha decide, y así
// el evento sirve igual el día que haga falta mostrar el contador en algún
// lado. Un fallo al contar no puede voltear la renovación, que ya está
// guardada: se loguea y sigue, y el peor caso es un aviso que queda abierto
// hasta el próximo cambio.
func (s *Service) avisarSiNoQuedanPendientes(ctx context.Context) {
	if s.bus == nil {
		return
	}
	pendientes, err := s.repo.ContarPendientesDeRenovar(ctx, s.Hoy())
	if err != nil {
		log.Printf("licencias: no se pudo contar las pendientes tras el cambio "+
			"(el aviso de la campana puede quedar abierto de más): %v", err)
		return
	}
	s.bus.Publish(eventbus.Evento{
		Tipo:    "licencia.pendientes",
		Payload: eventbus.PendientesDeLicencia{Pendientes: pendientes},
	})
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
// requerimiento: corregir una fecha que quedó mal, cambiar la duración de 30
// a 60 días, o cargar por primera vez el vencimiento de una licencia que se
// dio de alta sin él.
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
	// declaran "renovada el martes", el vencimiento nuevo tiene que salir de la
	// duración nueva.
	cambioElVencimiento := params.Vencimiento.declarado()
	if cambioElVencimiento {
		if err := params.Vencimiento.aplicarA(l, s.Hoy(), s.ahora(), params.PorUsuario); err != nil {
			return err
		}
	}

	if err := s.repo.GuardarLicencia(ctx, l); err != nil {
		return err
	}
	// Solo si se tocó la fecha: renombrar el software o cambiarle los días de
	// duración no resuelve nada pendiente. Los días de AVISO sí mueven la
	// ventana, y por eso también cuentan.
	if cambioElVencimiento || params.DiasAviso != nil {
		s.avisarSiNoQuedanPendientes(ctx)
	}
	return nil
}

// ── Lecturas y baja ─────────────────────────────────────────────────────

// ListarLicencias trae todas con su ubicación, ya ordenadas para la pantalla:
// primero las que no tienen fecha, después de la más vencida a la que más le
// falta.
func (s *Service) ListarLicencias(ctx context.Context) ([]*LicenciaConUbicacion, error) {
	return s.repo.ListarLicencias(ctx)
}

func (s *Service) ListarLicenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.LicenciaSoftware, error) {
	return s.repo.ListarLicenciasPorEquipo(ctx, equipoID)
}

// BorrarLicencia elimina la fila.
func (s *Service) BorrarLicencia(ctx context.Context, licenciaID string) error {
	return s.repo.BorrarLicencia(ctx, licenciaID)
}

// ObtenerLicencia — ver el bloque «Obtener uno solo» de service.go.
func (s *Service) ObtenerLicencia(ctx context.Context, id string) (*domain.LicenciaSoftware, error) {
	return s.repo.BuscarLicenciaPorID(ctx, id)
}
