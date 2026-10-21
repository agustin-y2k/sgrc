package eventbus

import (
	"fmt"
	"reflect"
)

// Los nombres de clase persistidos deben conservarse al evolucionar el código.
// Validar también el tópico evita confirmar una entrega cuyo consumidor
// descartaría un payload de otro tipo.
var clasePorEvento = map[string]string{
	"reserva.reubicada":                    "eventbus.ReubicacionesDeUsuario",
	"docente.registro.pendiente":           "map[string]string",
	"cuenta.pendiente.resuelta":            "map[string]string",
	"docente.baja.materia-huerfana":        "map[string]interface {}",
	"docente.desasignado.materia-huerfana": "map[string]interface {}",
	"sugerencia.nueva":                     "eventbus.SugerenciaNueva",
	"sugerencia.seguimiento":               "eventbus.SugerenciaSeguimiento",
	"sugerencia.respondida":                "eventbus.SugerenciaRespondida",
	"sugerencia.resuelta":                  "map[string]string",
	"materia.pedido.nuevo":                 "eventbus.PedidoDeMateriaNuevo",
	"materia.pedido.resuelto":              "eventbus.PedidoDeMateriaResuelto",
	"licencia.por-vencer":                  "eventbus.AvisoDeLicencias",
	"licencia.pendientes":                  "eventbus.PendientesDeLicencia",
	"prestamo.cierre.pendientes":           "eventbus.PendientesDelCierre",
	"prestamo.sin-devolver.cierre":         "eventbus.EquiposSinDevolverAlCierre",
	"reserva.cancelada":                    "eventbus.CancelacionesDeUsuario",
	"reserva.recordatorio":                 "eventbus.RecordatorioDeReserva",
	"reserva.pedido-de-liberacion":         "eventbus.PedidoDeLiberacion",
	"cuenta.aprobada":                      "eventbus.CuentaAprobada",
	"password.recuperacion.solicitada":     "eventbus.DatosDeRecuperacion",
	"password.recuperacion.cuenta-google":  "eventbus.CuentaSoloConGoogle",
}

func validarClase(tipo, clase string) error {
	esperada, ok := clasePorEvento[tipo]
	if !ok || clase != esperada {
		return fmt.Errorf("contrato durable inválido: evento %s con clase %s", tipo, clase)
	}
	return nil
}

var tiposPayload = map[string]reflect.Type{
	"eventbus.ReubicacionesDeUsuario":     reflect.TypeOf(ReubicacionesDeUsuario{}),
	"eventbus.ReservaCancelada":           reflect.TypeOf(ReservaCancelada{}),
	"eventbus.CancelacionesDeUsuario":     reflect.TypeOf(CancelacionesDeUsuario{}),
	"eventbus.LicenciaPorVencer":          reflect.TypeOf(LicenciaPorVencer{}),
	"eventbus.AvisoDeLicencias":           reflect.TypeOf(AvisoDeLicencias{}),
	"eventbus.DatosDeRecuperacion":        reflect.TypeOf(DatosDeRecuperacion{}),
	"eventbus.CuentaSoloConGoogle":        reflect.TypeOf(CuentaSoloConGoogle{}),
	"eventbus.CuentaAprobada":             reflect.TypeOf(CuentaAprobada{}),
	"eventbus.RecordatorioDeReserva":      reflect.TypeOf(RecordatorioDeReserva{}),
	"eventbus.PedidoDeLiberacion":         reflect.TypeOf(PedidoDeLiberacion{}),
	"eventbus.EquipoSinDevolverAlCierre":  reflect.TypeOf(EquipoSinDevolverAlCierre{}),
	"eventbus.PendientesDeLicencia":       reflect.TypeOf(PendientesDeLicencia{}),
	"eventbus.PendientesDelCierre":        reflect.TypeOf(PendientesDelCierre{}),
	"eventbus.EquiposSinDevolverAlCierre": reflect.TypeOf(EquiposSinDevolverAlCierre{}),
	"eventbus.SugerenciaNueva":            reflect.TypeOf(SugerenciaNueva{}),
	"eventbus.SugerenciaSeguimiento":      reflect.TypeOf(SugerenciaSeguimiento{}),
	"eventbus.SugerenciaRespondida":       reflect.TypeOf(SugerenciaRespondida{}),
	"eventbus.PedidoDeMateriaNuevo":       reflect.TypeOf(PedidoDeMateriaNuevo{}),
	"eventbus.DocenteDeMateria":           reflect.TypeOf(DocenteDeMateria{}),
	"eventbus.PedidoDeMateriaResuelto":    reflect.TypeOf(PedidoDeMateriaResuelto{}),
	"map[string]string":                   reflect.TypeOf(map[string]string{}),
	"map[string]interface {}":             reflect.TypeOf(map[string]any{}),
}
