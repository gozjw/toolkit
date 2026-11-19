import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'
import { viteSingleFile } from 'vite-plugin-singlefile'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    vueDevTools(),
    viteSingleFile(),
    AutoImport({
      resolvers: [ElementPlusResolver()],
    }),
    Components({
      resolvers: [ElementPlusResolver()],
    }),
  ],
  build: {
    // 设为 0 表示完全禁用资源转 Base64 内联，所有资源独立输出为文件
    assetsInlineLimit: 0,
    rollupOptions: {
      onwarn(warning, warn) {
        // 核心：如果是这个无效注释的警告，直接跳过（return），不打印也不报错
        if (warning.code === 'INVALID_ANNOTATION') {
          return
        }
        // 其他正常的警告正常弹出来
        warn(warning)
      },
    },
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
})
