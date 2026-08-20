import { GraduationCap, Wrench } from "lucide-react"
import { useFormContext } from "react-hook-form"
import { z } from "zod"

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Select } from "@/components/ui/select"
import { SelectorDeCursoSolicitado } from "@/features/auth/SelectorDeCursoSolicitado"
import { cn } from "@/lib/utils"

/**
 * Lo que se declara al registrarse, compartido por el registro con contraseña
 * y el registro con Google — antes estaba duplicado en los dos.
 *
 * Nada de esto otorga permisos: es una declaración, igual que el curso y la
 * materia. La cuenta nace DOCENTE/PENDIENTE en los dos casos y es el Admin
 * quien decide, después de aprobarla, si además la promueve (RF-01.3).
 */
export const camposDeclarados = {
  cargoSolicitado: z.enum(["DOCENTE", "ADMIN_SISTEMA"], {
    error: "Elegí con qué cargo te registrás",
  }),
  rolSolicitado: z.enum(["TITULAR", "SUPLENTE"], {
    error: "Elegí si sos titular o suplente",
  }),
  // Los dos siguen siendo opcionales, y siguen siendo texto libre: al
  // registrarse la persona no está autenticada, así que no hay lista que
  // consultar y el curso puede no existir todavía.
  cursoSolicitado: z.string().max(100).optional(),
  materiaSolicitada: z.string().max(100).optional(),
}

/**
 * Qué mandar al backend según el cargo elegido.
 *
 * Vacío viaja como ausente —"no lo declaró" y "lo dejó en blanco" no son dos
 * cosas distintas— y quien no da clase no manda curso ni materia, aunque
 * hayan quedado escritos de antes de cambiar de tarjeta. El backend los
 * descarta igual (no se confía en el formulario); esto es para que el cuerpo
 * del pedido no afirme algo que el sistema no va a guardar.
 */
export function loQueDictaEseCargo(values: {
  cargoSolicitado?: string
  cursoSolicitado?: string
  materiaSolicitada?: string
}) {
  if (values.cargoSolicitado !== "DOCENTE") {
    return { cursoSolicitado: undefined, materiaSolicitada: undefined }
  }
  return {
    cursoSolicitado: values.cursoSolicitado?.trim() || undefined,
    materiaSolicitada: values.materiaSolicitada?.trim() || undefined,
  }
}

const OPCIONES_DE_CARGO = [
  {
    valor: "DOCENTE",
    icono: GraduationCap,
    titulo: "Docente",
    detalle: "Das clase frente a alumnos y vas a reservar equipos para tus materias.",
  },
  {
    valor: "ADMIN_SISTEMA",
    icono: Wrench,
    titulo: "Administrador de Sistema",
    detalle:
      "Auxiliar informático, administrador de red y demás cargos docentes que " +
      "administran el laboratorio sin estar frente a alumnos.",
  },
] as const

/**
 * El bloque del registro donde se declara con qué cargo entrás.
 *
 * Son dos tarjetas y no un desplegable a propósito: elegir una cambia el
 * formulario —el bloque "¿qué vas a dictar?" aparece solo para Docente— y un
 * `<select>` esconde esa consecuencia detrás de una lista cerrada.
 *
 * Lee el formulario del contexto (`<Form>` es el FormProvider de
 * react-hook-form), así que las dos pantallas lo montan sin pasarle nada más
 * que un prefijo de id para que los dos selectores de curso no colisionen.
 */
export function DeclaracionDeCargo({ idPrefijo }: { idPrefijo: string }) {
  const form = useFormContext()
  const cargo = form.watch("cargoSolicitado")

  return (
    <>
      <FormField
        control={form.control}
        name="cargoSolicitado"
        render={({ field }) => (
          <FormItem>
            <FormLabel>¿Con qué cargo te registrás?</FormLabel>
            <FormControl>
              {/* radiogroup y no botones sueltos: son opciones excluyentes y
                  con esto un lector de pantalla las anuncia como tales. */}
              <div role="radiogroup" className="grid gap-2">
                {OPCIONES_DE_CARGO.map(({ valor, icono: Icono, titulo, detalle }) => (
                  <button
                    key={valor}
                    type="button"
                    role="radio"
                    aria-checked={field.value === valor}
                    onClick={() => field.onChange(valor)}
                    className={cn(
                      "focus-visible:ring-ring flex items-start gap-3 rounded-xl border p-3 text-left transition-colors focus-visible:ring-2 focus-visible:outline-none",
                      field.value === valor
                        ? "border-primary bg-accent"
                        : "bg-superficie hover:bg-muted"
                    )}
                  >
                    <span
                      aria-hidden="true"
                      className="bg-accent text-accent-foreground grid size-9 shrink-0 place-items-center rounded-lg"
                    >
                      <Icono className="size-5" />
                    </span>
                    <span>
                      <span className="block font-medium">{titulo}</span>
                      <span className="text-muted-foreground block text-sm">
                        {detalle}
                      </span>
                    </span>
                  </button>
                ))}
              </div>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      {/* El rol aplica a los dos cargos, así que vive afuera de "¿qué vas a
          dictar?": también hay auxiliares suplentes. */}
      <FormField
        control={form.control}
        name="rolSolicitado"
        render={({ field }) => (
          <FormItem>
            <FormLabel>¿Sos titular o suplente?</FormLabel>
            <FormControl>
              <Select {...field} value={field.value ?? ""}>
                <option value="">Elegí una opción…</option>
                <option value="TITULAR">Titular</option>
                <option value="SUPLENTE">Suplente</option>
              </Select>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      {cargo === "ADMIN_SISTEMA" && (
        <p className="text-muted-foreground text-sm">
          Si además dictás materias, podés pedirlas desde tu perfil una vez que te
          aprueben la cuenta.
        </p>
      )}

      {cargo === "DOCENTE" && (
        <div className="border-t pt-4">
          <p className="mb-1 text-sm font-medium">¿Qué vas a dictar?</p>
          <p className="text-muted-foreground mb-3 text-xs">
            Opcional, pero ayuda: es lo que el Admin va a mirar para asignarte a la
            materia correcta al aprobar tu cuenta. Si el curso o la materia todavía no
            existen, los crea.
          </p>
          <div className="grid gap-4">
            <FormField
              control={form.control}
              name="cursoSolicitado"
              render={({ field }) => (
                <FormItem>
                  <FormControl>
                    <SelectorDeCursoSolicitado
                      idPrefijo={`${idPrefijo}-curso`}
                      value={field.value ?? ""}
                      onChange={field.onChange}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="materiaSolicitada"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Materia</FormLabel>
                  <FormControl>
                    <Input placeholder="Ej.: Programación" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </div>
      )}
    </>
  )
}
