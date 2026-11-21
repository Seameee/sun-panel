import { createApp } from 'vue'
import App from './App.vue'
import { setupI18n } from './locales'
import { setupAssets, setupScrollbarStyle } from './plugins'
import { setupStore } from './store'
import { setupRouter } from './router'
import { registerLocalIcons } from './plugins/localIcons'

async function bootstrap() {
  const app = createApp(App)
  setupAssets()
  registerLocalIcons()

  setupScrollbarStyle()

  setupStore(app)

  setupI18n(app)

  await setupRouter(app)
  app.mount('#app')
}

bootstrap()
