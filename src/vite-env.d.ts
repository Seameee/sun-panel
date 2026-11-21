
/// <reference types="vite-plugin-pwa/vue" />

// Plain TypeScript hosts resolve SFC imports through this declaration.
// vue-tsc checks their component types and templates.
declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent
  export default component
}
