import esbuild from 'esbuild';

const shouldWatch = process.argv.includes('--watch');

const config = {
  entryPoints: ['src/sqlite-promiser-bridge.worker.ts'],
  outfile: '../assets/sqlite-promiser-bridge.worker.js',
  bundle: true,
  format: 'iife',
  platform: 'browser',
  target: ['es2020'],
  minify: false, // Don't minify for easier debugging
  sourcemap: false,
  external: ['@sqlite.org/sqlite-wasm'], // Don't bundle SQLite WASM
};

if (shouldWatch) {
  const context = await esbuild.context(config);
  await context.watch();
  console.log('Watching for changes...');
} else {
  await esbuild.build(config);
  console.log('Build complete!');
}