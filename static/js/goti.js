/* ============================================================================
 * GOti — instructor de la empresa DEMO (ERPGo)
 * Cómo funciona: el render entrega {{GuiDemo}}; layout.html solo carga este
 * archivo cuando la empresa activa es demo. Aquí se pregunta /guia/active y se
 * dibuja UN globo a la vez sobre el elemento del paso (selector CSS). Reglas
 * clave: nunca bloquea la pantalla entera, siempre puede cerrarse, y el
 * progreso boldea en la base DEMO (guia_progreso).
 * ========================================================================== */
(function () {
  'use strict';

  var GT = {
    overlay: null,
    globo: null,
    paso: null,
    tour: null,
    pasos: []
  };

  function $(id) { return document.getElementById(id); }
  function http(method, url, body) {
    // Usa ERPGo.csrfFetch (definido en csrf.js del layout) para adjuntar el
    // token CSRF: /guia/progress ya NO está exento del CSRF, así que el POST
    // sin X-CSRF-Token cae en 403 "Token de seguridad inválido". GOti.js sólo
    // carga en empresa demo (GuiDemo), y csrf.js está en el layout, así que
    // ERPGo está definido.
    var fetcher = (typeof window !== 'undefined' && window.ERPGo && window.ERPGo.csrfFetch) ? window.ERPGo.csrfFetch : fetch;
    return fetcher(url, {
      method: method,
      headers: { 'Content-Type': 'application/json' },
      body: body ? JSON.stringify(body) : undefined,
      credentials: 'same-origin'
    }).then(function (r) { return r.json(); });
  }
  function esc(s) {
    var d = document.createElement('div');
    d.appendChild(document.createTextNode(s || ''));
    return d.innerHTML;
  }

  /* ---------------------- panel + globo visual ---------------------- */

  function ensureOverlay() {
    if (GT.overlay) return;
    var ov = document.createElement('div');
    ov.id = 'goti-overlay';
    ov.className = 'goti-overlay';
    var gl = document.createElement('div');
    gl.id = 'goti-globo';
    gl.className = 'goti-globo';
    ov.appendChild(gl);
    document.body.appendChild(ov);
    GT.overlay = ov;
    GT.globo = gl;
  }

  function findElement(sel) {
    if (!sel) return null;
    try { return document.querySelector(sel); } catch (e) { return null; }
  }

  function rollOut() {
    if (GT.overlay) { GT.overlay.remove(); }
    GT.overlay = null; GT.globo = null; GT.paso = null;
  }

  function drawPaso(p) {
    ensureOverlay();
    GT.paso = p;
    var el = findElement(p.selector);
    var pos = { top: '12vh', left: '50vw', transform: 'translateX(-50%)' };
    if (el) {
      var r = el.getBoundingClientRect();
      pos = {
        top: Math.max(60, r.bottom + 12) + 'px',
        left: (r.left + Math.min(r.width / 2, window.innerWidth - 220)) + 'px'
      };
      el.classList.add('goti-highlight');
    }
    var missesDep = p._depsMissing;
    var botones = '';
    if (missesDep) {
      botones = '<button class="goti-btn goti-principal" data-go="1">Llévame</button>' +
                '<button class="goti-btn" data-skip="1">Saltar</button>';
    } else if (p.check_msg) {
      botones = '<button class="goti-btn goti-principal" data-done="1">Ya lo hice, verificar</button>' +
                '<button class="goti-btn" data-skip="1">Saltar</button>';
    } else {
      botones = '<button class="goti-btn goti-principal" data-done="1">Hecho</button>' +
                '<button class="goti-btn" data-skip="1">Saltar</button>';
    }
    GT.globo.innerHTML =
      '<div class="goti-cabeza"><span class="goti-mascota">🎓</span>' +
        '<span class="goti-voz">GOti</span>' +
        '<button class="goti-x" data-hide="1" title="No volver a mostrar">&times;</button>' +
      '</div>' +
      '<div class="goti-cuerpo">' +
        '<strong>' + esc(p.titulo) + '</strong>' +
        '<p>' + esc(p.mensaje) + '</p>' +
        (missesDep ? '<div class="goti-aviso">Primero: ' + esc(missesDep.titulo) + '</div>' : '') +
        '<div class="goti-msg-error" hidden></div>' +
      '</div>' +
      '<div class="goti-pie">' + botones +
        '<a class="goti-ir" href="' + esc(p.url) + '">Ir a la ventana</a>' +
      '</div>';

    GT.globo.style.top = pos.top;
    GT.globo.style.left = pos.left;
    GT.globo.style.transform = pos.transform;

    GT.globo.querySelectorAll('[data-done],[data-skip],[data-hide],[data-go]').forEach(function (b) {
      b.addEventListener('click', onClickBoton);
    });
    GT.globo.querySelector('.goti-ir').addEventListener('click', function () {
      rollOut();
    });
  }

  function onError(msg) {
    if (!GT.globo) return;
    var box = GT.globo.querySelector('.goti-msg-error');
    if (box) { box.hidden = false; box.textContent = msg; }
  }

  function onClickBoton(ev) {
    var b = ev.currentTarget;
    if (!GT.paso) return;
    var accion = b.dataset.done ? 'done' : (b.dataset.skip ? 'skip' : (b.dataset.hide ? 'hide' : null));
    if (!accion) return;
    if (accion === 'hide') {
      http('POST', '/guia/progress', { paso_id: GT.paso.id, action: 'hide' }).then(rollOut);
      return;
    }
    http('POST', '/guia/progress', { paso_id: GT.paso.id, action: accion }).then(function (r) {
      if (r.status === 0 && r.error === 'verificacion') {
        onError(r.msg || 'Aún no se cumplió la verificación de este paso.');
        return;
      }
      if (r.status !== 1) { onError(r.msg || 'No se pudo registrar'); return; }
      // Se re-consulta: resuelve el paso siguiente y las dependencias por sesión.
      cargar().then(rollOut);
    }).catch(function () { onError('Sin respuesta del servidor'); });
  }

  /* ---------------------- arranque + fase ---------------------- */

  function depsResueltas(p) {
    if (!p.depends_on) return true;
    return GT.pasos.some(function (x) {
      return x.id === p.depends_on && (x.done || x.skipped);
    });
  }

  function cargar() {
    return http('GET', '/guia/active').then(function (r) {
      if (!r || r.active !== true) { rollOut(); return; }
      GT.tour = r; GT.pasos = r.pasos || [];
      GT.pasos.forEach(function (p) { p._depsMissing = !depsResueltas(p); });
      var actual = (r.pasos || []).find(function (p) { return p.id === r.actual_id; });
      if (actual) drawPaso(actual); else rollOut();
      return r;
    });
  }

  function iniciar() {
    cargar().catch(function () { /* sin sesión: no molestar nunca */ });
    // La navegación SPA no existe: cada página recarga, entonces se re-dibuja
    // el paso correspondiente al cargar cada ventana.
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', iniciar);
  } else {
    iniciar();
  }
})();
