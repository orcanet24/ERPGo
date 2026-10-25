#!/usr/bin/env node
/* _build.mjs — genera doc/developer/indice.html a partir de:
     _plantilla_head.html  (head + cuerpo del HTML)
     _plantilla_js1.html   (bloque <script> con la lógica)
     _tree.json            (árbol de directorios, lo genera _tree.ps1)
     _deps.json            (aristas import, lo genera _deps.ps1)
   Uso:  node doc/developer/_build.mjs
*/
import { readFileSync, writeFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const read = f => readFileSync(join(here, f), 'utf8').replace(/^\uFEFF/, '');

const tree = read('_tree.json');
const deps = read('_deps.json');

/* métricas: recorre _tree.json para contar archivos .go y directorios */
let goFiles = 0, dirCount = 0;
function walk(n){
  dirCount++;
  (n.files || []).forEach(f => { if (/\.go$/i.test(f.name)) goFiles++; });
  (n.dirs  || []).forEach(walk);
}
walk(JSON.parse(tree));

let head = read('_plantilla_head.html');
let js   = read('_plantilla_js1.html');

/* sustituye los marcadores que viven en el bloque <script> */
js = js.replace('__TREE__', tree.trim())
       .replace('__DEPS__', deps.trim())
       .replace('__GO_COUNT__', String(goFiles))
       .replace('__DIR_COUNT__', String(dirCount));

const out = head + '\n' + js;
writeFileSync(join(here, 'indice.html'), out, 'utf8');

const kb = (statSync(join(here, 'indice.html')).size / 1024).toFixed(1);
console.log(`indice.html generado: ${kb} KB  (archivos .go: ${goFiles}, directorios: ${dirCount})`);
