// -*- coding: utf-8 -*-
/**
 * Genera el Mapa del Proyecto ERPGo con Archify.
 *
 * Reagrupa los archivos de interfaces/web por módulos de negocio (facturación,
 * inventario, compras, contabilidad, RRHH, IA, CRM, reportes, add-ons,
 * administración y plataforma) y deja el resto de capas con sus paquetes reales.
 *
 * El mapa es de dos niveles (capas → módulos/paquetes) para que sea compacto y
 * legible; el detalle archivo a archivo sigue en docs/Diccionario/index.html.
 *
 * Entrada : docs/Diccionario/datos.js  (la misma fuente que el diccionario)
 * Salida  : candidate.json             (IR de Archify, listo para finalize)
 *
 * Uso: node generar-mapa.js
 */
const fs = require('fs');
const path = require('path');

const RAIZ = path.resolve(__dirname, '..', '..');
const DATOS = path.join(RAIZ, 'docs', 'Diccionario', 'datos.js');
const SALIDA = __dirname;

// ------------------------------------------------------------------ utilidades
function leerDatos() {
  let s = fs.readFileSync(DATOS, 'utf8');
  s = s.replace(/^\s*\/\/.*\r?\n/, '');
  s = s.replace(/^\s*const DATOS = /, '').replace(/;\s*$/, '');
  return JSON.parse(s);
}

function contarHojas(nodo) {
  if (nodo.tipo === 'file') return 1;
  let n = 0;
  for (const h of nodo.hijos || []) n += contarHojas(h);
  return n;
}

function archivosDe(nodo, out = []) {
  if (nodo.tipo === 'file') { out.push(nodo); return out; }
  for (const h of nodo.hijos || []) archivosDe(h, out);
  return out;
}

// "1 archivo" / "N archivos"
function plural(n) {
  return n === 1 ? '1 archivo' : `${n} archivos`;
}

// ----------------------------------------------------- módulos de negocio (web)
// Reglas evaluadas en orden; la primera coincidencia asigna el módulo.
const MODULOS_WEB = [
  { id: 'facturacion', nombre: 'Facturación y ventas', desc: 'Facturas, cotizaciones, órdenes, clientes, POS, facturación digital y fiscal' },
  { id: 'inventario', nombre: 'Inventario y almacén', desc: 'Stock, kardex, costos, conteos cíclicos y punto de reorden' },
  { id: 'compras', nombre: 'Compras y proveedores', desc: 'Compras, bandeja de entrada y proveedores' },
  { id: 'contabilidad', nombre: 'Contabilidad y finanzas', desc: 'Cuentas, libros legales, períodos, bancos, cartera y moneda' },
  { id: 'rrhh', nombre: 'RRHH y nómina', desc: 'Empleados, conceptos, permisos y nómina' },
  { id: 'ia', nombre: 'Inteligencia artificial', desc: 'Agentes, proveedores, herramientas, esquema y gobernanza' },
  { id: 'crm', nombre: 'CRM', desc: 'Clientes y remarketing' },
  { id: 'reportes', nombre: 'Reportes y dashboard', desc: 'Reportes, motor, PDF y panel principal' },
  { id: 'addons', nombre: 'Add-ons', desc: 'Registro, administración y ganchos de complementos' },
  { id: 'administracion', nombre: 'Administración y seguridad', desc: 'Menú, permisos, usuarios, sesiones, tenants y auditoría' },
  { id: 'plataforma', nombre: 'Plataforma y sistema', desc: 'Servidor, caché, gzip, workers, salud y arranque' },
];

const REGLAS_WEB = [
  ['ai', 'ia'],
  ['informes_ia', 'ia'],
  ['addon', 'addons'],
  ['pos_', 'facturacion'],
  ['digital_invoice', 'facturacion'],
  ['digitalinvoice', 'facturacion'],
  ['contingencia_digital', 'facturacion'],
  ['conasev', 'facturacion'],
  ['fiscal', 'facturacion'],
  ['retencion_numero', 'facturacion'],
  ['semilla_', 'facturacion'],
  ['guia_', 'facturacion'],
  ['agentes_control_test', 'facturacion'],
  ['invoice', 'facturacion'],
  ['quotation', 'facturacion'],
  ['order', 'facturacion'],
  ['customer', 'facturacion'],
  ['creditnote', 'facturacion'],
  ['debitnote', 'facturacion'],
  ['delivery_order', 'facturacion'],
  ['return', 'facturacion'],
  ['adjustment', 'facturacion'],
  ['price_list', 'facturacion'],
  ['entrada_rapida', 'facturacion'],
  ['voucher_extra', 'facturacion'],
  ['inventory_', 'inventario'],
  ['kardex', 'inventario'],
  ['stock_movement_helper', 'inventario'],
  ['cycle_count', 'inventario'],
  ['reorder_point', 'inventario'],
  ['cost_', 'inventario'],
  ['product', 'inventario'],
  ['categorias_arbol_test', 'inventario'],
  ['purchase', 'compras'],
  ['supplier', 'compras'],
  ['account', 'contabilidad'],
  ['coa_', 'contabilidad'],
  ['legal_books', 'contabilidad'],
  ['period_', 'contabilidad'],
  ['bank', 'contabilidad'],
  ['pagar', 'contabilidad'],
  ['cartera', 'contabilidad'],
  ['exchange_rate_daily', 'contabilidad'],
  ['finance_', 'contabilidad'],
  ['multi_currency', 'contabilidad'],
  ['retenciones_test', 'contabilidad'],
  ['payroll', 'rrhh'],
  ['hr_', 'rrhh'],
  ['crm_', 'crm'],
  ['report', 'reportes'],
  ['dashboard', 'reportes'],
  ['menu', 'administracion'],
  ['permission', 'administracion'],
  ['modulos_acceso_test', 'administracion'],
  ['user', 'administracion'],
  ['session', 'administracion'],
  ['logout', 'administracion'],
  ['password', 'administracion'],
  ['identity', 'administracion'],
  ['csrf', 'administracion'],
  ['tenant', 'administracion'],
  ['company_bootstrap', 'administracion'],
  ['setting', 'administracion'],
  ['appearance', 'administracion'],
  ['view_pref', 'administracion'],
  ['module_handlers', 'administracion'],
  ['audit_', 'administracion'],
  ['server', 'plataforma'],
  ['service', 'plataforma'],
  ['handlers', 'plataforma'],
  ['helpers', 'plataforma'],
  ['render', 'plataforma'],
  ['cache', 'plataforma'],
  ['cachas', 'plataforma'],
  ['gzip', 'plataforma'],
  ['json', 'plataforma'],
  ['dialect', 'plataforma'],
  ['md5compat', 'plataforma'],
  ['outbound', 'plataforma'],
  ['worker', 'plataforma'],
  ['orchestrator', 'plataforma'],
  ['health', 'plataforma'],
  ['backup_scheduler', 'plataforma'],
  ['notify_channels', 'plataforma'],
  ['database_settings', 'plataforma'],
  ['setup', 'plataforma'],
  ['first_run', 'plataforma'],
  ['regla_borrado', 'plataforma'],
  ['tabla_lista_metodos_test', 'plataforma'],
  ['vistas_enlaces_test', 'plataforma'],
  ['rutas_guardado_test', 'plataforma'],
  ['correcciones_modulos_test', 'plataforma'],
  ['recarga_blanda_test', 'plataforma'],
  ['sse_entrega_test', 'plataforma'],
];

function moduloDe(nombre) {
  for (const [prefijo, mod] of REGLAS_WEB) {
    if (nombre.startsWith(prefijo)) return mod;
  }
  return null;
}

// ------------------------------------------------------------------ construcción
function construir(datos) {
  const nodos = [];
  const porModulo = {};
  for (const m of MODULOS_WEB) porModulo[m.id] = 0;

  nodos.push({
    id: 'raiz',
    label: 'ERPGo',
    sublabel: `${datos.totalArchivos} archivos · ${datos.totalFunciones} funciones`,
  });

  // --- interfaces/web: módulos de negocio -----------------------------------
  const web = datos.arbol.hijos
    .find(h => h.nombre === 'interfaces')
    .hijos.find(h => h.nombre === 'web');

  nodos.push({
    id: 'capa_interfaces',
    label: 'interfaces/web',
    parent: 'raiz',
    sublabel: plural(contarHojas(web)),
  });

  const sinClasificar = [];
  for (const h of web.hijos || []) {
    const esDir = h.tipo === 'dir';
    const nombre = esDir ? h.nombre : h.nombre;
    const mod = esDir ? 'facturacion' : moduloDe(nombre);
    if (!esDir && !mod) {
      sinClasificar.push(h.ruta);
      continue;
    }
    porModulo[mod] += contarHojas(h);
  }

  for (const m of MODULOS_WEB) {
    if (!porModulo[m.id]) continue;
    nodos.push({
      id: 'mod_' + m.id,
      label: m.nombre,
      parent: 'capa_interfaces',
      sublabel: plural(porModulo[m.id]),
    });
  }

  // --- resto de capas: paquetes ---------------------------------------------
  const CAPAS = [
    { id: 'capa_internal', nombre: 'internal', desc: 'Lógica interna, servicios y trabajadores' },
    { id: 'capa_infrastructure', nombre: 'infrastructure', desc: 'Infraestructura y persistencia' },
    { id: 'capa_migrations', nombre: 'migrations', desc: 'Migraciones y esquemas SQL' },
    { id: 'capa_cmd', nombre: 'cmd', desc: 'Comandos y puntos de entrada' },
    { id: 'capa_addons', nombre: 'addons', desc: 'Complementos (add-ons) del ERP' },
    { id: 'capa_tools', nombre: 'tools', desc: 'Herramientas de desarrollo' },
    { id: 'capa_configs', nombre: 'configs', desc: 'Configuración de tenants' },
    { id: 'capa_static', nombre: 'static', desc: 'Recursos estáticos (JS)' },
    { id: 'capa_scripts', nombre: 'scripts', desc: 'Construcción y despliegue' },
    { id: 'capa_fiscalprinteradapter', nombre: 'fiscalprinteradapter', label: 'impresora fiscal', desc: 'Adaptador de impresora fiscal' },
  ];

  for (const c of CAPAS) {
    const nodoCapa = datos.arbol.hijos.find(h => h.nombre === c.nombre);
    if (!nodoCapa) continue;
    nodos.push({
      id: c.id,
      label: c.label || c.nombre,
      parent: 'raiz',
      sublabel: plural(contarHojas(nodoCapa)),
    });

    // scripts: se agrupan los archivos sueltos para no ensanchar el mapa.
    if (c.nombre === 'scripts') {
      const sueltos = (nodoCapa.hijos || []).filter(h => h.tipo === 'file');
      const dirs = (nodoCapa.hijos || []).filter(h => h.tipo === 'dir');
      for (const d of dirs) {
        nodos.push({
          id: 'n_' + d.ruta.replace(/[^a-zA-Z0-9_-]/g, '_'),
          label: d.nombre + '/',
          parent: c.id,
          sublabel: plural(contarHojas(d)),
        });
      }
      if (sueltos.length) {
        nodos.push({
          id: 'scripts_sueltos',
          label: 'build y verificación',
          parent: c.id,
          sublabel: plural(sueltos.length),
        });
      }
      continue;
    }

    // configs: los tres archivos son un solo nodo de configuración de tenants.
    if (c.nombre === 'configs') {
      nodos.push({
        id: 'configs_nucleo',
        label: 'configuración de tenants',
        parent: c.id,
        sublabel: plural(contarHojas(nodoCapa)),
      });
      continue;
    }

    // migrations: 61 SQL sueltos se agrupan por prefijo para no alargar el mapa.
    if (c.nombre === 'migrations') {
      const GRUPOS = [
        { id: 'mig_base', nombre: 'Esquema base', match: /^schema(_[a-z]+)?\.sql$|^schema_menu_|^schema_ve_|^schema_zz_vendor/ },
        { id: 'mig_reconstruido', nombre: 'Reconstrucción por módulos', match: /^schema_reconstruido_/ },
        { id: 'mig_ampliaciones', nombre: 'Ampliaciones z', match: /^schema_z_/ },
        { id: 'mig_ajustes', nombre: 'Ajustes finales', match: /^schema_zz/ },
      ];
      const porGrupo = {};
      for (const g of GRUPOS) porGrupo[g.id] = 0;
      for (const h of nodoCapa.hijos || []) {
        if (h.tipo !== 'file') continue;
        let asignado = null;
        for (const g of GRUPOS) {
          if (g.match.test(h.nombre)) { asignado = g.id; break; }
        }
        if (asignado) porGrupo[asignado] += 1;
        else sinClasificar.push(h.ruta);
      }
      for (const g of GRUPOS) {
        if (!porGrupo[g.id]) continue;
        nodos.push({
          id: g.id,
          label: g.nombre,
          parent: c.id,
          sublabel: `${plural(porGrupo[g.id])} SQL`,
        });
      }
      continue;
    }

    for (const h of nodoCapa.hijos || []) {
      if (h.tipo === 'file') {
        // Archivo suelto: se representa como un nodo con su nombre.
        nodos.push({ id: 'n_' + h.ruta.replace(/[^a-zA-Z0-9_-]/g, '_'), label: h.nombre, parent: c.id });
      } else {
        nodos.push({
          id: 'n_' + h.ruta.replace(/[^a-zA-Z0-9_-]/g, '_'),
          label: h.nombre + '/',
          parent: c.id,
          sublabel: plural(contarHojas(h)),
        });
      }
    }
  }

  // --- archivos sueltos de la raíz ------------------------------------------
  for (const h of datos.arbol.hijos || []) {
    if (h.tipo === 'file') {
      nodos.push({ id: 'n_' + h.ruta.replace(/[^a-zA-Z0-9_-]/g, '_'), label: h.nombre, parent: 'raiz' });
    }
  }

  return { nodos, sinClasificar };
}

// ------------------------------------------------------------------ main
function main() {
  const datos = leerDatos();
  const { nodos, sinClasificar } = construir(datos);

  if (sinClasificar.length) {
    console.error('ARCHIVOS SIN CLASIFICAR:');
    for (const r of sinClasificar) console.error('  ' + r);
  }

  const candidato = {
    schema_version: 1,
    diagram_type: 'tree',
    meta: {
      title: 'Mapa del proyecto ERPGo',
      subtitle: 'Capas y módulos de negocio · el detalle archivo a archivo está en el Diccionario de Scripts',
      output: 'docs/Mapa-Proyecto/mapa-proyecto.html',
      locale: 'es',
      quality_profile: 'showcase',
    },
    layout: { direction: 'down', gapX: 16, gapY: 24, nodeW: 96, nodeMaxW: 120 },
    nodes: nodos,
    cards: [
      {
        dot: 'cyan',
        title: 'Cómo leer este mapa',
        items: [
          'La raíz es el proyecto ERPGo; el primer nivel son las capas (interfaces, internal, infrastructure, migrations, cmd, addons, tools, configs, static, scripts y el adaptador fiscal).',
          'Dentro de interfaces/web los 209 archivos se agrupan por módulo de negocio, no por carpeta: son 11 módulos funcionales.',
          'Cada nodo indica cuántos archivos contiene. Usa el buscador (/) para encontrar un módulo o capa, y haz clic en un nodo para centrarlo.',
        ],
      },
      {
        dot: 'emerald',
        title: 'Capas del ERP',
        items: [
          'interfaces/web — capa web: handlers HTTP, plantillas y lógica de presentación (212 archivos).',
          'internal — lógica interna: servicios, orquestador, cola, seguridad, setup y verificación web (27 archivos).',
          'infrastructure/persistence — MySQL/SQLite, migraciones, protección y respaldos (15 archivos).',
          'migrations — esquemas SQL del modelo de datos (61 archivos).',
          'cmd — puntos de entrada: servidor, docgen, diag, genhash y semillar (12 archivos).',
        ],
      },
      {
        dot: 'violet',
        title: 'Diccionario de Scripts',
        items: [
          'Este mapa es la vista estructural. El diccionario (docs/Diccionario/index.html) es el catálogo buscable con la descripción de cada archivo y de sus 2933 funciones.',
        ],
      },
    ],
  };

  fs.writeFileSync(path.join(SALIDA, 'candidate.json'), JSON.stringify(candidato, null, 2), 'utf8');
  console.log(`Nodos: ${nodos.length} | candidate.json escrito en ${SALIDA}`);
  return sinClasificar.length ? 1 : 0;
}

if (require.main === module) process.exit(main());
module.exports = { construir, leerDatos, MODULOS_WEB, REGLAS_WEB };
