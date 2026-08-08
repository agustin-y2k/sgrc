package application

import (
	"errors"
	"fmt"
)

var (
	ErrReservaGrupoNoEncontrado = errors.New("reserva no encontrada")
	ErrReservaNoEncontrada      = errors.New("reserva de equipo no encontrado")

	// ErrDocenteNoAsignado: RF-04.1 — solo un docente asignado a la
	// materia puede reservar para ella.
	ErrDocenteNoAsignado = errors.New("el docente no está asignado a esta materia")

	// ErrMateriaArchivada: RF-04.1 — una materia de un ciclo cerrado
	// conserva su registro pero no admite reservas nuevas.
	ErrMateriaArchivada = errors.New("la materia está archivada y no admite reservas nuevas")

	// ErrEquipoNoDisponible: la PC no existe, está dada de baja, o no está en
	// estado DISPONIBLE.
	ErrEquipoNoDisponible = errors.New("el equipo no está disponible para reservar")

	// ErrSolapamiento: alguna de los equipos pedidos ya tiene una reserva
	// confirmada que se superpone con el horario pedido. Se intenta
	// detectar esto ANTES de golpear la constraint de la base
	// (mejor mensaje), pero la constraint EXCLUDE sigue siendo la
	// garantía real ante condiciones de carrera.
	ErrSolapamiento = errors.New("uno o más equipos ya tienen una reserva en ese horario")

	// ErrSinEquipos: una reserva necesita al menos una PC.
	ErrSinEquipos = errors.New("hay que seleccionar al menos un equipo")

	// ErrDemasiadosEquipos acota el tamaño del lote. No lo pide ninguna regla
	// de la escuela: lo pide que la operación es de lote y el pedido lo arma
	// el cliente. Sin tope, mandar diez mil identificadores hace que el
	// servidor arme diez mil filas en una transacción antes de que la base
	// pueda quejarse de nada.
	//
	// El límite es holgado a propósito: MaxEquiposPorOperacion es varias
	// veces el inventario de una escuela como esta, así que frena el pedido
	// absurdo sin poder molestar a ninguno legítimo.
	ErrDemasiadosEquipos = fmt.Errorf("no se pueden pedir más de %d equipos en una sola operación", MaxEquiposPorOperacion)

	// ErrDemasiadasOcurrencias: RF-04.2 materializa un ReservaGrupo por
	// cada fecha de la serie, todo en una transacción. Sin un tope, un
	// rango de fechas largo (fechaFin en 2099) genera miles de grupos y
	// reservas en un solo request: además de bloquear el proceso, deja al
	// docente con una serie que no puede administrar y una respuesta JSON
	// de megabytes.
	ErrDemasiadasOcurrencias = errors.New("el período pedido genera demasiadas clases; acotá la fecha de fin")

	// ErrSinOcurrencias: el rango es válido pero no contiene ni un solo
	// día de la semana pedido (ej. "los martes, del 4 al 6 de marzo").
	// Sin esto se creaba una regla de recurrencia que no materializaba
	// nada y quedaba huérfana en la base.
	ErrSinOcurrencias = errors.New("el período pedido no incluye ningún día de la semana elegido")

	// ErrMotivoObligatorio: RF-04.8 — cuando un Admin cancela la reserva
	// de otra persona tiene que decir por qué. El docente recibe ese texto
	// en la notificación (RF-05.1), así que un motivo vacío la dejaría sin
	// explicación. Cancelar una reserva propia no lo exige.
	ErrMotivoObligatorio = errors.New("el motivo es obligatorio para cancelar una reserva ajena")

	// ── Préstamos ───────────────────────────────────────────────────

	ErrPrestamoNoEncontrado = errors.New("no se encontró ese registro de entrega")

	// ErrEquipoYaPrestado: el índice único parcial de la migración 013 impide
	// dos préstamos abiertos sobre la misma PC. Es la garantía que el papel
	// no puede dar — entregar dos veces la misma máquina porque dos Admin
	// la anotaron a la vez no lo detecta nadie hasta que aparece un docente
	// sin computadora.
	ErrEquipoYaPrestado = errors.New("ese equipo ya figura entregado y todavía no volvió")

	// ErrEquipoDadoDeBaja: no se entrega una máquina que salió del inventario.
	// Las EN_MANTENIMIENTO y FUERA_DE_SERVICIO sí se pueden entregar: llevar
	// una PC rota al técnico es justamente un préstamo.
	ErrEquipoDadoDeBaja = errors.New("ese equipo está dado de baja del inventario")

	// ErrReservaAjena: RF-04.4 — un docente solo toca sus propias reservas.
	// El Admin puede tocar cualquiera.
	ErrReservaAjena = errors.New("esa reserva es de otra persona")

	ErrIDInvalido = errors.New("el ID indicado no tiene un formato válido")

	// ErrReferenciaInexistente: SQLSTATE 23503 (foreign_key_violation) — el
	// request nombró un padre que no existe (un carro, un ciclo, una PC, un
	// usuario). Es un error del cliente, no una falla del servidor: sin este
	// sentinel caía al 500 genérico de mapearError, que era el modo de falla
	// más común de toda la API para cualquier ID válido-pero-inexistente.
	ErrReferenciaInexistente = errors.New("alguno de los datos referenciados no existe")
)
