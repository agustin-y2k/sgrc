import { Button } from "@/components/ui/button"
import type { ImpactoDeJornada } from "@/features/disponibilidad/types"

/**
 * Lo que el cambio de jornada va a dejar afuera, con la decisión en la mano
 * del Admin.
 *
 * El conteo es la pieza importante y es lo primero que se lee. Un cambio
 * legítimo —la escuela dejó de abrir los sábados— deja afuera un puñado de
 * clases de un día; un tipeo de 15:00–16:00 en lugar de 07:00–18:00 deja
 * afuera casi todo. El número separa esos dos casos sin que el sistema tenga
 * que adivinar cuál es cuál.
 */

/** A partir de qué proporción el cambio se lee como un error y no como una decisión. */
const PROPORCION_SOSPECHOSA = 0.7

function plural(n: number, singular: string, plural: string): string {
  return n === 1 ? singular : plural
}

export function ImpactoDeLaJornada({
  impacto,
  guardando,
  onConfirmar,
  onCancelar,
}: {
  impacto: ImpactoDeJornada
  guardando: boolean
  onConfirmar: () => void
  onCancelar: () => void
}) {
  // Se cuenta en CLASES, que es la unidad que reconoce quien decide: una
  // clase con cinco máquinas es una clase que se cae, no cinco reservas. Los
  // equipos se muestran al lado porque son lo que el sistema cancela de
  // verdad, y en un carro entero la diferencia entre los dos números es la
  // diferencia entre entender el cambio y no.
  const clases = impacto.clasesAfectadas
  const equipos = impacto.totalAfectadas
  const recortada = equipos > impacto.reservas.length
  // Los docentes se cuentan sobre la lista, así que solo se pueden nombrar
  // cuando la lista está completa: con el recorte, "de 12 docentes" sería el
  // número de los primeros cincuenta y no el de los afectados.
  const docentes = recortada
    ? 0
    : new Set(impacto.reservas.map((r) => r.docente).filter((d) => d !== "")).size
  // Contra el total, no en absoluto: veinte cancelaciones sobre veinticinco
  // reservas no es lo mismo que veinte sobre trescientas.
  const casiTodo =
    impacto.totalDeClases > 0 && clases / impacto.totalDeClases >= PROPORCION_SOSPECHOSA

  return (
    <div className="border-destructive/50 mb-4 grid gap-3 rounded-md border p-4">
      <div>
        <p className="font-medium">Este cambio deja reservas fuera del horario</p>
        {clases > 0 && (
          <p className="text-destructive mt-1 text-sm">
            Se van a cancelar <strong>{clases}</strong>{" "}
            {plural(clases, "clase", "clases")}
            {/* Los equipos solo cuando el número difiere: con una máquina por
                clase, "(3 equipos)" al lado de "3 clases" es ruido. */}
            {equipos !== clases && (
              <>
                {" "}
                ({equipos} {plural(equipos, "equipo", "equipos")})
              </>
            )}
            {docentes > 0 && (
              <>
                {" "}
                de {docentes} {plural(docentes, "docente", "docentes")}
              </>
            )}
            , y se les va a avisar por correo.
          </p>
        )}
      </div>

      {/* El cartel del tipeo. Un cambio real de horario no suele llevarse
          puesta a la escuela entera, así que cuando lo hace conviene decirlo
          con otras palabras antes de que alguien apriete confirmar. */}
      {casiTodo && (
        <p className="text-sm font-medium">
          Eso es casi todo lo que hay reservado. Si no era la intención, revisá las horas
          del tramo antes de confirmar.
        </p>
      )}

      {equipos > 0 && (
        <div className="max-h-60 overflow-y-auto rounded-md border">
          <ul className="divide-y text-sm">
            {impacto.reservas.map((r) => (
              <li key={r.id} className="px-3 py-1.5">
                <span className="font-medium">{r.fecha}</span>{" "}
                <span className="text-muted-foreground">
                  {r.horaInicio}–{r.horaFin}
                </span>{" "}
                · {r.equipo}
                {r.materia !== "" && <> · {r.materia}</>}
                {r.docente !== "" && (
                  <span className="text-muted-foreground"> · {r.docente}</span>
                )}
              </li>
            ))}
          </ul>
          {/* Sin esta línea la lista recortada parece la lista completa. */}
          {recortada && (
            <p className="text-muted-foreground px-3 py-1.5 text-sm">
              y {equipos - impacto.reservas.length} más
            </p>
          )}
        </div>
      )}

      {/* Lo que cambia una decisión: cancelar una clase cuyas máquinas siguen
          en el laboratorio es un correo; cancelar una que el docente ya retiró
          es dejarlo parado con las computadoras en la mano. */}
      {impacto.prestamos.length > 0 && (
        <div className="rounded-md border p-3">
          <p className="text-sm font-medium">
            Atención: {impacto.prestamos.length}{" "}
            {plural(
              impacto.prestamos.length,
              "de esas clases ya tiene su computadora entregada",
              "de esas clases ya tienen sus computadoras entregadas"
            )}
          </p>
          <ul className="text-muted-foreground mt-1 text-sm">
            {impacto.prestamos.map((p) => (
              <li key={p.id}>
                {p.equipo} · la tiene {p.quien}
              </li>
            ))}
          </ul>
          <p className="text-muted-foreground mt-1 text-sm">
            El préstamo no se cancela: la máquina está afuera y se recibe como siempre.
            Pero conviene avisarle a mano antes de que se entere por el correo.
          </p>
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="destructive"
          disabled={guardando}
          onClick={onConfirmar}
        >
          {clases > 0
            ? `Guardar y cancelar ${clases} ${plural(clases, "clase", "clases")}`
            : "Guardar igual"}
        </Button>
        <Button variant="outline" size="sm" disabled={guardando} onClick={onCancelar}>
          Volver sin cambiar nada
        </Button>
      </div>
    </div>
  )
}
