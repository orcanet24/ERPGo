// appearance.js — Tema y densidad de quien está usando el sistema.
//
// El atributo que decide el color y el ritmo de la pantalla ya lo
// pone el servidor en el <html>, así que entrar no produce destello.
// Este archivo solo hace falta cuando ALGUIEN cambia de opinión:
//
//   1. escribe el atributo en el acto, para que el cambio se vea sin
//      recargar ni una sola petición;
//   2. guarda una cookie, para que el login y el selector de empresa
//      —que se pintan sin sesión— salgan como lo dejó esa persona;
//   3. avisa al servidor, para que la preferencia sobreviva al cambio
//      de navegador y de equipo.
//
// El paso 1 no depende de nadie, así que el interruptor responde
// siempre. Los otros dos viajan por detrás y no llegan a estorbar.
(function () {
    'use strict';

    var COOKIE = 'erpgo_appearance';
    var CSRF_COOKIE = 'erpgo_csrf';
    var VIEW = 'appearance';
    var YEAR = 365 * 24 * 3600;

    var THEMES = ['light', 'dark', 'auto'];
    var DENSITIES = ['comfortable', 'compact'];

    function root() {
        return document.documentElement;
    }

    // allowed es la lista de lo que existe en CSS. Un valor que no
    // esté aquí no se escribe: escribir un atributo sin bloque que lo
    // atienda deja la pantalla a medio camino, que es peor que no
    // cambiar nada.
    function current(group) {
        var el = root();
        if (group === 'theme') return el.getAttribute('data-theme') || 'light';
        return el.getAttribute('data-density') || 'comfortable';
    }

    function isAllowed(group, value) {
        return group === 'theme' ? THEMES.indexOf(value) !== -1 : DENSITIES.indexOf(value) !== -1;
    }

    function cookieToken() {
        var parts = document.cookie ? document.cookie.split(';') : [];
        for (var i = 0; i < parts.length; i++) {
            var pair = parts[i].trim();
            if (pair.indexOf(CSRF_COOKIE + '=') === 0) {
                return decodeURIComponent(pair.substring(CSRF_COOKIE.length + 1));
            }
        }
        return '';
    }

    // persist guarda la preferencia como una vista del usuario. Reutiliza
    // la tabla y el endpoint que ya sirven para las vistas del panel, de
    // modo que la apariencia no necesita migración ni endpoint propio.
    //
    // Con XMLHttpRequest y no con jQuery a propósito: el login carga este
    // archivo sin jQuery, y un solo camino para las dos pantallas es mejor
    // que dos.
    function persist(theme, density) {
        var token = cookieToken();
        if (!token) return;   // sin sesión no hay a quién guardarle

        var body = 'payload=' + encodeURIComponent(JSON.stringify({ theme: theme, density: density }));
        var xhr = new XMLHttpRequest();
        xhr.open('POST', '/user/view-pref/' + VIEW, true);
        xhr.setRequestHeader('Content-Type', 'application/x-www-form-urlencoded');
        xhr.setRequestHeader('X-CSRF-Token', token);
        xhr.send(body);
    }

    function remember(theme, density) {
        // El separador es un punto y NO se codifica: Request.Cookie() de Go
        // devuelve el valor tal cual, sin decodificarlo, así que un
        // separador que el navegador escapara llegaría al servidor ilegible y
        // la preferencia se perdería sin avisar.
        document.cookie = COOKIE + '=' + theme + '.' + density +
            ';path=/;max-age=' + YEAR + ';SameSite=Lax';
    }

    // mark deja la opción activa marcada sin tocar el resto de clases del
    // botón.
    //
    // Se AÑADE y se QUITA 'is-on' en vez de reescribir className: el mismo
    // archivo gobierna el menú de la aplicación y los tres botones del
    // login, y esos llevan sus propias clases. Reescribir la lista entera
    // las borraba y el login se quedaba sin estilo.
    function mark() {
        var groups = { theme: current('theme'), density: current('density') };
        var buttons = document.querySelectorAll('[data-appearance-choice] [data-appearance-value]');
        for (var i = 0; i < buttons.length; i++) {
            var b = buttons[i];
            var group = b.parentNode.getAttribute('data-appearance-choice');
            if (!group) continue;
            var on = groups[group] === b.getAttribute('data-appearance-value');
            if (on) b.classList.add('is-on');
            else b.classList.remove('is-on');
            b.setAttribute('aria-pressed', on ? 'true' : 'false');
        }
    }

    // apply escribe los atributos y deja marcada la opción activa.
    function apply(group, value) {
        if (!isAllowed(group, value)) return;
        if (current(group) === value) return;

        root().setAttribute(group === 'theme' ? 'data-theme' : 'data-density', value);
        mark();

        var theme = current('theme');
        var density = current('density');
        remember(theme, density);
        persist(theme, density);
    }

    // bind engancha los controles. Se delega en document porque el menú
    // del usuario vive dentro de una lista que el tema original
    // reordena al abrir: delegar sobrevive a eso, y atar cada botón por
    // separado no.
    function bind() {
        document.addEventListener('click', function (e) {
            var b = e.target.closest ? e.target.closest('[data-appearance-value]') : null;
            if (!b) return;
            var group = b.parentNode.getAttribute('data-appearance-choice');
            if (!group) return;
            e.preventDefault();
            apply(group, b.getAttribute('data-appearance-value'));
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', start);
    } else {
        start();
    }

    function start() {
        mark();
        bind();
    }

    // window.ERPGo.appearance queda a mano para los scripts de página que
    // necesitan saber el tema actual: los gráficos, por ejemplo, pintan
    // con colores y tienen que leerlos del CSS en vez de inventarlos.
    window.ERPGo = window.ERPGo || {};
    window.ERPGo.appearance = {
        current: current,
        apply: apply,
        // css devuelve el valor de una variable CSS ya resuelta, con un
        // valor de repuesto por si el navegador no la expone.
        css: function (name, fallback) {
            var v = getComputedStyle(root()).getPropertyValue(name);
            v = v ? v.trim() : '';
            return v || fallback || '';
        }
    };
})();