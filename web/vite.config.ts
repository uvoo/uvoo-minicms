import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: '/admin/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
    chunkSizeWarningLimit: 1000,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (id.includes('@codemirror/lang-') || id.includes('@codemirror/legacy-modes/mode/')) return
          if (id.includes('/@mdxeditor/') || id.includes('@mdxeditor')) return 'mdxeditor'
          if (id.includes('/@codemirror/') || id.includes('/codemirror/') || id.includes('cm6-theme-basic-light')) return 'codemirror'
          if (id.includes('/lexical/') || id.includes('@lexical')) return 'lexical'
          if (id.includes('/@ant-design/') || id.includes('/antd/') || id.includes('/rc-')) return 'antd'
          if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/scheduler/')) return 'react'
          if (id.includes('/micromark') || id.includes('/remark-') || id.includes('/mdast-') || id.includes('/hast-') || id.includes('/unist-') || id.includes('/vfile') || id.includes('/unified/') || id.includes('/@mdx-js/')) return 'markdown'
        }
      }
    }
  },
  server: { proxy: { '/cms.v1.CMSService': 'http://localhost:8080', '/uploads': 'http://localhost:8080' } }
})
