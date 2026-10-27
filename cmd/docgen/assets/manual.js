// Manual del desarrollador — renderizador.
//
// Todo el contenido sale del modelo JSON que la plantilla inyecta; este
// archivo solo lo dibuja. Cero peticiones de red, cero librerías. Se ejecuta
// como cuerpo de new Function("model", "doc", ...): la plantilla lo recibe
// como literal de cadena JS y lo invoca con el modelo ya parseado.
//
// ¿POR QUÉ vanilla? El manual se regenera con cada build; una dependencia
// más sería otra cosa que desfasa. Y el contenido viene del código
// analizado, no de usuarios: el escape aquí es doblemente prudente, por si
// un día la fuente cambia.

function render(model, doc) {
  var ENT = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
  function esc(s) {
    return String(s === null || s === undefined ? "" : s).replace(/[&<>"']/g, function (m) {
      return ENT[m];
    });
  }
  function plural(n, cosa) {
    return n + " " + cosa + (n === 1 ? "" : "s");
  }

  // Índice de archivos por id: los clicks no recorren el modelo entero.
  var archivoPorId = {};
  model.files.forEach(function (f) { archivoPorId[f.id] = f; });

  var KINDS = ["todos", "code", "test", "migración", "plantilla", "doc", "script", "config"];
  var KINDS_ALT = ["todos", "code", "test", "migration", "plantilla", "doc", "script", "config"];

  var app = doc.getElementById("app");
  app.innerHTML =
    '<nav class="tabs"><div class="inner" id="tabs"></div></nav>' +
    '<section class="vista" id="vista"></section>';
  var tabs = doc.getElementById("tabs");
  var contenedor = doc.getElementById("vista");

  function segmentoPorId(id) {
    for (var i = 0; i < model.segments.length; i++) {
      if (model.segments[i].id === id) return model.segments[i];
    }
    return null;
  }

  // --- pestañas ---------------------------------------------------------------
  var destinos = [];
  model.segments.forEach(function (s) { destinos.push(s.id); });
  destinos.push("__todos__");

  destinos.forEach(function (id) {
    var b = doc.createElement("button");
    var s = segmentoPorId(id);
    b.textContent = s ? s.title : "Todos los archivos";
    b.dataset.destino = id;
    b.addEventListener("click", function () { mostrar(id); });
    tabs.appendChild(b);
  });

  function activar(id) {
    Array.prototype.forEach.call(tabs.children, function (b) {
      b.className = b.dataset.destino === id ? "activo" : "";
    });
  }

  function mostrar(id) {
    activar(id);
    if (id === "__todos__") {
      vistaTodos();
      return;
    }
    var s = segmentoPorId(id);
    if (!s) {
      contenedor.innerHTML = '<p class="cargando">Segmento no encontrado: ' + esc(id) + "</p>";
      return;
    }
    vistaSegmento(s);
  }

  // --- piezas compartidas --------------------------------------------------------
  function cabecera() {
    var st = model.stats || {};
    var cov = model.coverage || { curated: 0, total: 0, percent: 0 };
    var badges = [
      plural(st.files || 0, "archivo"),
      plural(st.funcs || 0, "función"),
      plural(st.types || 0, "tipo"),
      plural(st.edges || 0, "relación"),
      (st.calls || 0) + " llamadas (" + (st.resolved || 0) + " resueltas, " + (st.unresolved || 0) + " sin resolver)",
      "curados: " + (cov.curated || 0) + "/" + (cov.total || 0) + " (" + (cov.percent || 0) + "%)"
    ].map(function (t) { return '<span class="badge">' + esc(t) + "</span>"; }).join("");
    return '<p class="lema">' + esc(model.meta.tagline || "") + "</p>" +
      '<div class="badges">' + badges + "</div>";
  }

  function tablaArchivos(zona, lista) {
    var html = '<table class="archivos"><thead><tr>'
      + "<th>Archivo</th><th>Paquete</th><th>Tipo</th><th>Líneas</th><th>Curado</th>"
      + "</tr></thead><tbody>"
      + lista.map(function (f) {
        return '<tr class="fila" data-id="' + esc(f.id) + '">'
          + '<td class="ruta">' + esc(f.id) + "</td>"
          + "<td>" + esc(f.pkg || "") + "</td>"
          + "<td>" + esc(f.kind || "") + "</td>"
          + "<td>" + esc(f.lines) + "</td>"
          + "<td>" + (f.curated
            ? '<span class="tag curado">curado</span>'
            : '<span class="tag generado">parseado</span>') + "</td>"
          + "</tr>";
      }).join("")
      + "</tbody></table>";
    if (!lista.length) html = '<p class="cargando">Sin coincidencias.</p>';
    zona.innerHTML = html;
    Array.prototype.forEach.call(zona.querySelectorAll("tr.fila"), function (tr) {
      tr.addEventListener("click", function () { verArchivo(tr.dataset.id); });
    });
  }

  function abrirArchivo(id) {
    var f = archivoPorId[id];
    if (!f) return;
    var destino = doc.getElementById("detalle") || contenedor;
    destino.insertBefore(panelDeArchivo(f), destino.firstChild);
    if (destino.firstChild.scrollIntoView) destino.firstChild.scrollIntoView({ behavior: "smooth" });
  }

  function panelDeArchivo(f) {
    var html = '<div class="detalle"><h3 style="margin-top:0">' + esc(f.id) + "</h3>"
      + "<p><b>Paquete:</b> " + esc(f.pkg || "")
      + (f.pkgPath ? ' <span class="tag">' + esc(f.pkgPath) + "</span>" : "")
      + " · <b>Tipo:</b> " + esc(f.kind || "")
      + " · <b>Líneas:</b> " + esc(f.lines) + " (código: " + esc(f.codeLines || 0) + ")"
      + " · " + (f.curated ? '<span class="tag curado">curado</span>' : '<span class="tag generado">parseado</span>')
      + "</p>";
    if (f.doc) html += "<p>" + esc(f.doc) + "</p>";
    if (f.role) html += "<p><b>Rol:</b> " + esc(f.role) + "</p>";
    if (f.how) html += "<p><b>Cómo:</b> " + esc(f.how) + "</p>";
    if (f.relations) html += "<p><b>Relaciones:</b> " + esc(f.relations) + "</p>";
    if (f.watchouts) html += '<p class="watchout"><b>Ojo:</b> ' + esc(f.watchouts) + "</p>";
    if ((f.imports || []).length) {
      html += "<h3>Imports</h3><p>"
        + f.imports.map(function (im) {
          var ruta = (im && im.path) ? im.path : im;
          return "<code>" + esc(ruta) + "</code>";
        }).join(" · ") + "</p>";
    }
    if ((f.importedBy || []).length) {
      html += "<h3>Importado por</h3><p>"
        + f.importedBy.map(function (p) { return "<code>" + esc(p) + "</code>"; }).join(" · ")
        + "</p>";
    }
    if ((f.rules || []).length) {
      html += "<h3>Reglas que aplican</h3><ul>"
        + f.rules.map(function (r) { return "<li>" + esc(r) + "</li>"; }).join("") + "</ul>";
    }
    (f.types || []).forEach(function (t) {
      html += '<h3>Tipo: ' + esc(t.name) + ' <span class="tag">' + esc(t.kind) + "</span></h3>";
      if (t.doc) html += "<p>" + esc(t.doc) + "</p>";
      if (t.how) html += "<p>" + esc(t.how) + "</p>";
      if (t.relations) html += '<p class="lema">' + esc(t.relations) + "</p>";
    });
    if ((f.funcs || []).length) {
      html += "<h3>Funciones</h3>";
      f.funcs.forEach(function (fn) {
        var nombre = (fn.recv ? fn.recv + "." : "") + fn.name;
        html += '<p class="sig"><b>' + esc(nombre) + "</b> " + esc(fn.sig || "") + "</p>";
        if (fn.what) html += "<p>" + esc(fn.what) + "</p>";
        if ((fn.how || []).length) {
          html += '<ul class="how-lista">'
            + fn.how.map(function (h) { return "<li>" + esc(h) + "</li>"; }).join("")
            + "</ul>";
        }
        if (fn.relations) html += '<p class="lema">' + esc(fn.relations) + "</p>";
      });
    }
    html += "</div>";
    var tmp = doc.createElement("div");
    tmp.innerHTML = html;
    return tmp.firstElementChild;
  }

  // --- vistas ----------------------------------------------------------------------
  function vistaSegmento(s) {
    var propios = (s.files || []).map(function (fid) { return archivoPorId[fid]; }).filter(Boolean);
    var html = cabecera()
      + "<h1 style=\"font-size:20px\">" + esc(s.title) + "</h1>"
      + "<p>" + esc(s.summary || "") + "</p>";
    if ((s.responsibilities || []).length) {
      html += '<h2 class="tituloV">Responsabilidades</h2><ul>'
        + s.responsibilities.map(function (r) { return "<li>" + esc(r) + "</li>"; }).join("") + "</ul>";
    }
    if ((s.invariants || []).length) {
      html += '<h2 class="tituloV">Invariantes</h2><ul>'
        + s.invariants.map(function (i) { return "<li>" + esc(i) + "</li>"; }).join("") + "</ul>";
    }
    if ((s.paths || []).length) {
      html += '<h2 class="tituloV">Rutas</h2><div class="cards">'
        + s.paths.map(function (p) { return '<div class="card"><code>' + esc(p) + "</code></div>"; }).join("")
        + "</div>";
    }
    html += '<h2 class="tituloV">Archivos del segmento <span style="color:#57616e;font-weight:400;font-size:13px">('
      + plural(propios.length, "archivo") + ")</span></h2>"
      + '<div id="detalle"></div><div id="tabla-seg"></div>';
    contenedor.innerHTML = html;
    tablaArchivos(doc.getElementById("tabla-seg"), propios);
  }

  function vistaTodos() {
    contenedor.innerHTML = cabecera()
      + '<h2 class="tituloV">Todos los archivos</h2>'
      + '<div class="barra-busqueda">'
      + '<input id="buscar" type="search" placeholder="Buscar por ruta, paquete, función o tipo…">'
      + '<select id="filtro-kind">'
      + KINDS.map(function (k, i) {
        return '<option value="' + esc(k) + '">' + esc(k) + "</option>";
      }).join("")
      + "</select></div>"
      + '<p id="conteo" class="cargando"></p>'
      + '<div id="detalle"></div><div id="tabla-todos"></div>';
    var inp = doc.getElementById("buscar");
    var sel = doc.getElementById("filtro-kind");
    function refrescar() {
      var term = (inp.value || "").toLowerCase();
      var kind = sel.value;
      var lista = model.files.filter(function (f) { return coincide(f, term, kind); });
      doc.getElementById("conteo").textContent =
        plural(lista.length, "archivo") + " de " + plural(model.files.length, "archivo");
      tablaArchivos(doc.getElementById("tabla-todos"), lista);
    }
    inp.addEventListener("input", refrescar);
    sel.addEventListener("change", refrescar);
    refrescar();
  }

  function coincide(f, term, kind) {
    if (kind !== "todos" && (f.kind || "") !== kind) {
      // El parser llama “migración” a los .sql; si un overlay usa otra
      // grafía no rompemos la búsqueda: normalizamos ambas.
      var a = (f.kind || "").slice(0, 4).toLowerCase();
      var b = kind.slice(0, 4).toLowerCase();
      if (a !== b) return false;
    }
    if (!term) return true;
    if (f.id.toLowerCase().indexOf(term) !== -1) return true;
    if ((f.pkg || "").toLowerCase().indexOf(term) !== -1) return true;
    var i, arr = f.funcs || [];
    for (i = 0; i < arr.length; i++) {
      if (arr[i].name.toLowerCase().indexOf(term) !== -1) return true;
    }
    arr = f.types || [];
    for (i = 0; i < arr.length; i++) {
      if (arr[i].name.toLowerCase().indexOf(term) !== -1) return true;
    }
    return false;
  }

  vistaSegmento(model.segments[0] || {
    title: "Resumen", tagline: "", summary: "",
    responsibilities: [], invariants: [], paths: [], files: []
  });
  activar(destinos[0]);
}
