import { mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const backendDir = join(root, 'backend-go')
const outputDir = join(root, 'src-tauri', 'binaries')
const outputName = process.platform === 'win32' ? 'eucli-studio-backend.exe' : 'eucli-studio-backend'
const outputPath = join(outputDir, outputName)

mkdirSync(outputDir, { recursive: true })
rmSync(outputPath, { force: true })

const result = spawnSync('go', ['build', '-trimpath', '-o', outputPath, '.'], {
  cwd: backendDir,
  stdio: 'inherit',
  shell: process.platform === 'win32',
})

if (result.status !== 0) {
  process.exit(result.status || 1)
}

console.log(`[eucli-studio] backend built: ${outputPath}`)
