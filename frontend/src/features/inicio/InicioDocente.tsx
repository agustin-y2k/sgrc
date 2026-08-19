import { useState } from "react"
import { Link } from "react-router"
import {
  BellRing,
  CalendarDays,
  CalendarPlus,
  Laptop,
  MessageSquare,
  TriangleAlert,
  UserRound,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { AccesoDirecto } from "@/features/inicio/AccesoDirecto"
import { AvisarUnaFalla } from "@/features/inicio/AvisarUnaFalla"
import { TarjetaDeClase } from "@/features/inicio/TarjetaDeClase"
import type { GrupoDeReservas } from "@/features/reservas/types"

/** Cuántas clases se muestran enteras, con sus botones. */
const CLASES_A_LA_VISTA = 3

/** La pantalla de inicio de un docente. */
export function InicioDocente({
  grupos,
  hoy,
  noLeidas,
  hayError,
}: {
  /** Las clases confirmadas de hoy en adelante, ya ordenadas. */
  grupos: GrupoDeReservas[]
  hoy: string
  noLeidas: number
  /** La consulta de reservas falló: no se puede decir "no tenés ninguna". */
  hayError: boolean
}) {
  const [avisandoFalla, setAvisandoFalla] = useState(false)

  const aLaVista = grupos.slice(0, CLASES_A_LA_VISTA)
  const deMas = grupos.length - aLaVista.length

  return (
    <>
      {/* Los avisos, dichos en una frase y con el botón al lado. El contador
          de la barra alcanza para el que ya sabe que existe una bandeja;
          este renglón es para el que no. Solo aparece cuando hay algo: un
          "0 sin leer" permanente es ruido que se aprende a ignorar, y
          entonces tampoco se ve el día que dice 3. */}
      {noLeidas > 0 && (
        <div className="border-alerta/40 bg-alerta/10 flex flex-wrap items-center justify-between gap-3 rounded-xl border p-4">
          <p className="flex items-center gap-2 font-medium">
            <BellRing aria-hidden="true" className="size-5 shrink-0" />
            {noLeidas === 1
              ? "Tenés 1 aviso sin leer"
              : `Tenés ${noLeidas} avisos sin leer`}
          </p>
          {/* 44px de alto, igual que los botones de cada clase: ver la nota
              sobre el blanco táctil en TarjetaDeClase. */}
          <Button asChild size="lg" className="h-11 px-4">
            <Link to="/notificaciones">Ver los avisos</Link>
          </Button>
        </div>
      )}

      {/* La acción principal, sola en su bloque y a ancho completo: es lo
          único de esta pantalla que se hace en serio, y tiene que ganarle a
          todo lo demás sin que haya que buscarla. */}
      <div className="bg-superficie grid gap-3 rounded-xl border p-5 sm:flex sm:items-center sm:justify-between sm:gap-6">
        <div className="flex items-start gap-3">
          <span
            aria-hidden="true"
            className="bg-primary text-primary-foreground grid size-11 shrink-0 place-items-center rounded-xl"
          >
            <CalendarPlus className="size-6" />
          </span>
          <div>
            <p className="text-lg font-semibold">Reservar computadoras</p>
            <p className="text-muted-foreground text-sm">
              Para una clase o para todas las semanas. Elegís el día, la hora y cuántas
              necesitás.
            </p>
          </div>
        </div>
        <Button asChild size="lg" className="h-11 shrink-0 px-5 text-base">
          <Link to="/reservas/nueva">Reservar</Link>
        </Button>
      </div>

      <section className="grid gap-3">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="text-lg font-semibold">Tus próximas clases</h2>
          {/* `py-3` para que el renglón sea tocable y no una línea de texto
              de 20px: es un enlace, pero acá cumple la función de un botón, y
              con el padding llega a los mismos 44px que ellos. */}
          {grupos.length > 0 && (
            <Link
              to="/reservas"
              className="text-primary py-3 text-sm font-medium underline"
            >
              Ver todas mis reservas
            </Link>
          )}
        </div>

        {/* Con la consulta caída no se puede afirmar que no hay nada: el
            docente cerraría la pantalla creyendo que perdió la reserva. El
            aviso de que algo falló ya está arriba; acá solo hay que no
            mentir. */}
        {hayError ? (
          <p className="text-muted-foreground">
            No se pudieron consultar tus reservas. Probá recargar la página.
          </p>
        ) : grupos.length === 0 ? (
          <div className="bg-superficie grid gap-1 rounded-xl border p-4">
            <p className="font-medium">
              No tenés ninguna clase con computadoras reservadas.
            </p>
            <p className="text-muted-foreground text-sm">
              Cuando reserves, la clase va a aparecer acá con el día, la hora y qué
              computadoras te tocan.
            </p>
          </div>
        ) : (
          <>
            {aLaVista.map((grupo, i) => (
              <TarjetaDeClase
                key={`${grupo.grupoId ?? grupo.fecha}-${grupo.horaInicio}-${grupo.reservas[0]?.id}`}
                grupo={grupo}
                hoy={hoy}
                destacada={i === 0}
              />
            ))}
            {deMas > 0 && (
              <p className="text-muted-foreground text-sm">
                Y {deMas} {deMas === 1 ? "clase más" : "clases más"} después de estas.{" "}
                {/* Va dentro de una frase, así que no puede crecer a 44px sin
                    partir el renglón; `inline-block py-1` le da los 24px que
                    pide WCAG 2.5.8 sin romper el párrafo. */}
                <Link
                  to="/reservas"
                  className="text-primary inline-block py-1 font-medium underline"
                >
                  Verlas todas
                </Link>
                .
              </p>
            )}
          </>
        )}
      </section>

      <section className="grid gap-3">
        <h2 className="text-lg font-semibold">Otras cosas que podés hacer</h2>

        {avisandoFalla ? (
          <AvisarUnaFalla onCerrar={() => setAvisandoFalla(false)} />
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            <AccesoDirecto
              icono={Laptop}
              titulo="Ver las computadoras"
              ayuda="Cuáles hay en cada carro y qué programas tienen instalados."
              a="/inventario"
            />
            <AccesoDirecto
              icono={TriangleAlert}
              titulo="Avisar que una no anda"
              ayuda="Contá qué le pasa y los Admin lo van a ver."
              onClick={() => setAvisandoFalla(true)}
            />
            <AccesoDirecto
              icono={CalendarDays}
              titulo="Ver todas mis reservas"
              ayuda="Las que vienen y las que ya pasaron, con su estado."
              a="/reservas"
            />
            <AccesoDirecto
              icono={UserRound}
              titulo="Quién te puede ayudar"
              ayuda="En qué días y horarios está cada Admin del laboratorio."
              a="/disponibilidad"
            />
            <AccesoDirecto
              icono={BellRing}
              titulo="Mis avisos"
              ayuda="Lo que el sistema te fue avisando sobre tus reservas."
              a="/notificaciones"
            />
            {/* Reemplaza al atajo de "Cambiar mi contraseña": la
                contraseña está adentro del perfil, junto con la foto y las
                materias. Un atajo por cada cosa que se puede hacer sobre la
                cuenta habría vuelto esta lista una lista de opciones, que es
                lo contrario de lo que busca esta pantalla. */}
            <AccesoDirecto
              icono={UserRound}
              titulo="Mi perfil"
              ayuda="Tu foto, las materias que das y tu contraseña."
              a="/perfil"
            />
            <AccesoDirecto
              icono={MessageSquare}
              titulo="Escribirnos"
              ayuda="Si algo no anda o se te ocurre una mejora, contanos."
              a="/mis-mensajes"
            />
          </div>
        )}
      </section>
    </>
  )
}
