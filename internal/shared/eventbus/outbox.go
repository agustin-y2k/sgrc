package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/agustin-y2k/sgrc/internal/shared/pgtransaccion"
	"github.com/agustin-y2k/sgrc/internal/shared/secretos"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DurableBus struct {
	pool         *pgxpool.Pool
	cifrador     *secretos.Cifrador
	observadores *InMemoryEventBus
	mu           sync.RWMutex
	handlers     map[string][]func(context.Context, Evento) error
}

func NewDurableBus(pool *pgxpool.Pool, cifrador *secretos.Cifrador) *DurableBus {
	return &DurableBus{pool: pool, cifrador: cifrador, observadores: NewInMemoryEventBus(), handlers: make(map[string][]func(context.Context, Evento) error)}
}
func (b *DurableBus) EnOperacion(ctx context.Context, fn func(context.Context) error) error {
	return pgtransaccion.Ejecutar(ctx, b.pool, fn)
}
func (b *DurableBus) Publish(e Evento) {
	panic("publicación durable requiere Emitir con contexto transaccional")
}
func (b *DurableBus) Subscribe(tipo string, fn func(Evento)) { b.observadores.Subscribe(tipo, fn) }
func (b *DurableBus) SuscribirDurable(tipo string, fn func(context.Context, Evento) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[tipo] = append(b.handlers[tipo], fn)
}
func (b *DurableBus) Persistir(ctx context.Context, e Evento) error {
	if pgtransaccion.DeContexto(ctx) == nil {
		return errors.New("el evento debe compartir la transacción de su operación")
	}
	if e.Payload == nil {
		return errors.New("evento sin payload")
	}
	clase := reflect.TypeOf(e.Payload).String()
	if err := validarClase(e.Tipo, clase); err != nil {
		return err
	}
	if _, ok := tiposPayload[clase]; !ok {
		return fmt.Errorf("payload durable no registrado: %s", clase)
	}
	datos, err := json.Marshal(e.Payload)
	if err != nil {
		return err
	}
	cifrado, err := b.cifrador.Cifrar(string(datos))
	if err != nil {
		return err
	}
	_, err = pgtransaccion.Con(ctx, b.pool).Exec(ctx, `INSERT INTO evento_pendiente(tipo,clase,payload) VALUES($1,$2,$3)`, e.Tipo, clase, cifrado)
	return err
}
func (b *DurableBus) decodificar(tipo, clase, payload string) (Evento, error) {
	if err := validarClase(tipo, clase); err != nil {
		return Evento{}, err
	}
	t, ok := tiposPayload[clase]
	if !ok {
		return Evento{}, fmt.Errorf("clase durable desconocida: %s", clase)
	}
	claro, err := b.cifrador.Descifrar(payload)
	if err != nil {
		return Evento{}, err
	}
	destino := reflect.New(t)
	if err := json.Unmarshal([]byte(claro), destino.Interface()); err != nil {
		return Evento{}, err
	}
	return Evento{Tipo: tipo, Payload: destino.Elem().Interface()}, nil
}
func ejecutarConsumidor(ctx context.Context, fn func(context.Context, Evento) error, e Evento) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic procesando %s: %v", e.Tipo, p)
		}
	}()
	return fn(ctx, e)
}

// Despachar confirma un evento y todos sus avisos en la misma transacción.
// El lock de entrega ordena instancias y no omite un evento fallido: se conserva
// su lugar hasta reintentar. Correos y observadores se ejecutan tras ese commit.
func (b *DurableBus) Despachar(ctx context.Context, maximo int) (int, error) {
	completados := 0
	for range maximo {
		var id int64
		var listo bool
		err := pgtransaccion.Ejecutar(ctx, b.pool, func(ctx context.Context) error {
			id = 0
			listo = false
			tx := pgtransaccion.DeContexto(ctx)
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(0x534752430018)); err != nil {
				return err
			}
			var tipo, clase, payload string
			var despues time.Time
			err := tx.QueryRow(ctx, `SELECT id,tipo,clase,payload,reintentar_en FROM evento_pendiente WHERE procesado_en IS NULL ORDER BY id LIMIT 1 FOR UPDATE`).Scan(&id, &tipo, &clase, &payload, &despues)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			if despues.After(time.Now()) {
				return nil
			}
			e, err := b.decodificar(tipo, clase, payload)
			if err != nil {
				return err
			}
			b.mu.RLock()
			handlers := append([]func(context.Context, Evento) error{}, b.handlers[tipo]...)
			b.mu.RUnlock()
			for _, fn := range handlers {
				if err := ejecutarConsumidor(ctx, fn, e); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE evento_pendiente SET procesado_en=now(),ultimo_error=NULL,payload='' WHERE id=$1`, id); err != nil {
				return err
			}
			pgtransaccion.AlConfirmar(ctx, func(context.Context) { b.observadores.Publish(e) })
			listo = true
			return nil
		})
		if err != nil {
			// El intento y sus notificaciones ya se revirtieron. El evento permanece.
			if id != 0 {
				_, marcarErr := b.pool.Exec(ctx, `UPDATE evento_pendiente SET intentos=intentos+1,ultimo_error=$2,reintentar_en=now()+least(60,power(2,least(intentos,5))) * interval '1 second' WHERE id=$1 AND procesado_en IS NULL`, id, err.Error())
				if marcarErr != nil {
					return completados, errors.Join(err, marcarErr)
				}
			}
			return completados, err
		}
		if !listo {
			return completados, nil
		}
		completados++
	}
	return completados, nil
}
