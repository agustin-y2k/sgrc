import "@testing-library/jest-dom/vitest"
import { configure } from "@testing-library/react"

// El default de findBy*/waitFor es 1s. Alcanza cuando un archivo corre
// solo, pero con los 12 archivos en paralelo el tiempo de pared se estira
// y algún caso que encadena varios userEvent + una query lo rozaba: fallaba
// al azar en la suite completa y pasaba aislado. Un test que falla a veces
// no señala nada real y hace perder el tiempo, así que se le da margen — no
// cambia lo que se verifica, solo cuánto se espera.
configure({ asyncUtilTimeout: 5000 })

// jsdom no implementa ResizeObserver, pero varios primitivos de Radix
// (Checkbox, Select, …) lo usan para medirse en un layout effect. Sin este
// polyfill el componente lanza "ResizeObserver is not defined" DESPUÉS del
// primer render: React desmonta el árbol entero y el test falla con un DOM
// vacío, sin ningún error visible que apunte a la causa real.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver
