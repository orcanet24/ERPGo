// guardado.js — Guardar sin que se recargue la vista.
//
// El ERP se recargaba entero por dos caminos distintos:
//
//   1. Los formularios con method="POST" que nadie interceptaba: el navegador
//      hacía la petición, el servidor devolvía JSON y en pantalla se quedaba
//      el JSON crudo encima del formulario. Le pasaba a Cotizaciones,
//      Servicios, Factura de servicios, Devoluciones, Clases de cliente y la
//      tasa de cambio: seis pantallas que «guardaban» mostrando
//      {"status":1,...}.
//
//   2. Las pantallas que, tras un $.post exitoso, llamaban a location.reload():
//      aparecía el «Espere...», la barra de pace, y se volvían a dibujar el
//      encabezado, el menú y el pie para refrescar una tabla.
//
// Las dos se resuelven con la misma pieza: ERPGo.refrescar() vuelve a pedir la
// página al servidor y cambia SOLO la zona de contenido y los scripts de esa
// página. No hay navegación, no hay «Espere...», el menú no se toca y la
// posición de scroll se conserva. Es lo mismo que hacía location.reload(), con
// la diferencia de que no se nota.
//
// LO QUE NO TOCA ESTE ARCHIVO (y por qué sigue siendo seguro):
//
//   - El servidor: mismo POST, mismo token CSRF (por cabecera y por campo),
//     misma auditoría, mismas puertas de permisos. Solo cambia cómo se pinta
//     la respuesta en el navegador.
//   - Los formularios que ya manejaba su propia vista: si alguien hizo
//     preventDefault() antes, este interceptador no se entera. Es la regla que
//     separa a los 79 formularios AJAX de los 6 que se enviaban solos.
//   - El login, el asistente de instalación y el cambio de empresa: se marcan
//     como ajenos a la app y se dejan enviándose a la antigua, porque sus
//     respuestas son HTML con errores y no JSON.
//
// Si refrescar() no puede hacer su trabajo, cae a location.reload(): nunca
// peor que antes.
(function () {
    'use strict';

    var ERPGo = window.ERPGo = window.ERPGo || {};

    // cookie() lee una cookie legible por JavaScript. La del token CSRF es
    // justamente una de esas: el propio csrf.js la lee de la misma forma.
    function cookie(nombre) {
        var partes = document.cookie ? document.cookie.split(';') : [];
        for (var i = 0; i < partes.length; i++) {
            var par = partes[i].trim();
            if (par.indexOf(nombre + '=') === 0) {
                return decodeURIComponent(par.substring(nombre.length + 1));
            }
        }
        return '';
    }

    function aviso(msg) {
        if (window.toastr) { toastr.success(msg); }
    }

    function fallo(msg) {
        if (window.toastr) { toastr.error(msg); }
        else { console.error(msg); }
    }

    // ------------------------------------------------------------------
    // 1. Recarga blanda del contenido
    // ------------------------------------------------------------------

    var refrescando = false;

    // reEjecutarScripts vuelve a correr los <script> de un bloque recién
    // pegado. innerHTML no los ejecuta (por diseño del navegador), y sin esta
    // parte los manejadores de la vista quedarían colgados de nodos que ya no
    // existen: la tabla se vería bien y no respondería a nada.
    //
    // Un script que falle mientras se re-ejecuta dispara un evento 'error' en
    // window de forma síncrona. Ese es el único fallo que no se puede ver
    // desde aquí, así que se vigila y, si ocurre, la vista entera se recarga
    // a lo que había antes.
    function reEjecutarScripts(host) {
        var originales = host.querySelectorAll('script');
        var falloReejecucion = false;

        function vigila() { falloReejecucion = true; }
        window.addEventListener('error', vigila, true);

        try {
            for (var i = 0; i < originales.length; i++) {
                var viejo = originales[i];
                var nuevo = document.createElement('script');
                for (var j = 0; j < viejo.attributes.length; j++) {
                    var a = viejo.attributes[j];
                    if (a.name === 'async') { continue; }
                    nuevo.setAttribute(a.name, a.value);
                }
                // En orden y sin esperar: el código de una página supone que
                // las funciones de arriba ya existen cuando se ejecuta el de
                // abajo.
                nuevo.async = false;
                nuevo.text = viejo.text || viejo.textContent || '';
                viejo.parentNode.replaceChild(nuevo, viejo);
            }
        } finally {
            window.removeEventListener('error', vigila, true);
        }

        return !falloReejecucion;
    }

    function aplicar(html, scroll) {
        var doc = new DOMParser().parseFromString(html, 'text/html');
        var actual = document.querySelector('.content');
        var nueva = doc.querySelector('.content');
        if (!actual || !nueva) { return false; }

        if (doc.title) { document.title = doc.title; }

        actual.innerHTML = nueva.innerHTML;

        // El pie lleva el nombre, la dirección y el texto de la empresa: son
        // justamente los campos que se cambian desde Ajustes, así que si no se
        // toca, quien guarda el nombre de la empresa sigue viendo el viejo
        // abajo. No tiene scripts, solo texto y campos ocultos.
        var pie = document.querySelector('footer.main-footer');
        var pieNuevo = doc.querySelector('footer.main-footer');
        if (pie && pieNuevo) { pie.innerHTML = pieNuevo.innerHTML; }

        // Si había un modal abierto —el de Guardar de Ajustes de notificaciones,
        // por ejemplo—, su animación de cierre no va a terminar: el nodo del
        // modal acaba de ser sustituido y Bootstrap quedaría esperando a un
        // evento que no llega, con el fondo gris y el body sin scroll. El
        // refresco se ocupa de deshacerlo aquí mismo.
        var fondos = document.querySelectorAll('.modal-backdrop');
        for (var f = 0; f < fondos.length; f++) { fondos[f].parentNode.removeChild(fondos[f]); }
        document.body.classList.remove('modal-open');
        document.body.style.removeProperty('padding-right');

        // Los scripts escritos dentro del contenido tampoco se ejecutan solos
        // al pegarlos: sin esto, una vista como el catálogo del plan contable
        // se quedaría con los botones sin manejador tras el refresco.
        if (!reEjecutarScripts(actual)) { return false; }

        // Los scripts de la página viven fuera del contenido (layout los
        // pinta después del pie), así que se sustituyen aparte y en este
        // orden: primero el HTML, luego el código que se le agarra.
        var host = document.getElementById('pagejs-host');
        var hostNuevo = doc.getElementById('pagejs-host');
        if (host && hostNuevo) {
            host.innerHTML = hostNuevo.innerHTML;
            if (!reEjecutarScripts(host)) { return false; }
        }

        window.scrollTo(0, scroll);
        return true;
    }

    // ERPGo.refrescar vuelve a pedir la página actual y pinta solo la zona de
    // contenido. Sin navegación: ni «Espere...», ni barra de carga, ni menú
    // redibujado, ni pérdida del punto de scroll.
    //
    // Es el sustituto directo de location.reload(). Cualquier fallo —respuesta
    // que no es la esperada, redirección a otra página, script que no vuelve a
    // correr— cae en location.reload(), que es exactamente lo que había antes.
    ERPGo.refrescar = function () {
        if (refrescando) { return; }
        refrescando = true;

        var scroll = window.pageYOffset || document.documentElement.scrollTop || 0;

        fetch(location.href, {
            method: 'GET',
            credentials: 'same-origin',
            cache: 'no-store'
        }).then(function (res) {
            // Redirigida: la sesión venció o cambió de sitio. Eso no se pinta
            // dentro del contenedor, se navega.
            if (res.redirected) {
                if (res.url && res.url !== location.href) { location.href = res.url; }
                else { location.reload(); }
                return null;
            }
            if (!res.ok) { location.reload(); return null; }
            return res.text();
        }).then(function (html) {
            if (html === null || html === undefined) { return; }
            if (!aplicar(html, scroll)) { location.reload(); return; }
            refrescando = false;
        }).catch(function () {
            location.reload();
        });
    };

    // ------------------------------------------------------------------
    // 2. Formularios que se enviaban solos (sin JavaScript)
    // ------------------------------------------------------------------

    // Rutas que se quedan como estaban: su respuesta es una página HTML con el
    // error escrito en ella, no JSON, y refrescar el contenedor borraría el
    // motivo por el que el usuario no pudo entrar.
    var AJENO = ['/login', '/setup/', '/switch-tenant', '/logout'];

    function esAjeno(action) {
        for (var i = 0; i < AJENO.length; i++) {
            if (action.indexOf(AJENO[i]) === 0) { return true; }
        }
        return false;
    }

    // restaurar devuelve el formulario a su estado normal. El flag de "en vuelo"
    // es el que corta el doble envío: los botones se desactivan, pero una
    // segunda pulsación de Enter dentro de un campo vuelve a disparar submit y
    // esa vía no siempre mira los botones. En un ERP que mueve inventario y
    // cuentas, mandar dos cotizaciones con un mismo clic no es un detalle.
    function restaurar(form, botones) {
        if (form) { delete form.dataset.enviando; }
        for (var i = 0; i < botones.length; i++) { botones[i].disabled = false; }
    }

    function cuerpo(form) {
        // FormData del formulario, y encima el botón pulsado: el navegador lo
        // añade solo en un envío real y aquí no hay envío real. Sin este paso,
        // «Guardar y Salir» llegaría al servidor sin action=save_exit y
        // saveRedirect mandaría al listado equivocado.
        var fd = new FormData(form);
        var hayArchivo = false;
        fd.forEach(function (valor, clave) {
            if (typeof File !== 'undefined' && valor instanceof File && valor.name) {
                hayArchivo = true;
            }
        });
        if (hayArchivo) { return { body: fd, tipo: null }; }

        // Sin archivos: idéntico byte a byte a lo que manda un envío normal
        // del navegador (application/x-www-form-urlencoded). Es deliberado:
        // los handlers leen con r.FormValue, que solo entiende urlencoded y
        // multipart; mandar cualquier otra cosa sería silenciar los campos.
        var pares = [];
        fd.forEach(function (valor, clave) { pares.push([clave, valor]); });
        return {
            body: new URLSearchParams(pares).toString(),
            tipo: 'application/x-www-form-urlencoded; charset=UTF-8'
        };
    }

    function terminado(form, botones, res, texto) {
        restaurar(form, botones);

        var ct = (res.headers.get('Content-Type') || '').toLowerCase();

        if (ct.indexOf('application/json') >= 0) {
            var r;
            try {
                r = JSON.parse(texto);
            } catch (e) {
                fallo('El servidor devolvió una respuesta ilegible.');
                return;
            }
            var ok = r && (r.status === 1 || r.status === 'success' || r.ok === true);
            if (!ok) {
                // El error se ve aquí mismo, en la vista. Antes el navegador
                // se iba a una pantalla de JSON y el formulario quedaba atrás.
                fallo((r && (r.error || r.exception || r.msg)) || 'No se pudo guardar.');
                return;
            }
            if (r.msg || r.message) { aviso(r.msg || r.message); }

            var destino = r.redirect || r.location || '';
            if (destino) {
                var abs;
                try { abs = new URL(destino, location.href).href; }
                catch (e) { abs = ''; }
                if (abs && abs !== location.href) { location.href = abs; return; }
            }
            ERPGo.refrescar();
            return;
        }

        // No es JSON: o el servidor redirigió (tasa de cambio) o devolvió una
        // página. En ambos casos la respuesta correcta es estar en esa página.
        if (res.redirected && res.url && res.url !== location.href) {
            location.href = res.url;
            return;
        }
        ERPGo.refrescar();
    }

    // El botón pulsado no siempre llega en el evento: submitter es reciente y
    // se cayó de algunos navegadores. Un capturador de clics en fase de
    // captura recuerda cuál fue, que es lo que el navegador habría puesto en
    // el cuerpo del envío. Se comprueba contra el formulario para no arrastrar
    // el botón de otro modal.
    var ultimoPulsado = null;
    document.addEventListener('click', function (ev) {
        var t = ev.target;
        var b = (t && t.closest)
            ? t.closest('button[type="submit"], input[type="submit"], button:not([type])')
            : null;
        ultimoPulsado = b || null;
    }, true);

    jQuery(document).on('submit', 'form', function (e) {
        // Si la vista ya se hizo cargo, éste no es asunto nuestro. Es la regla
        // que mantiene intactos los 79 formularios que ya usan AJAX.
        if (e.isDefaultPrevented()) { return; }

        var form = this;
        var metodo = (form.getAttribute('method') || '').toUpperCase();
        if (metodo !== 'POST') { return; }
        if (form.target && form.target !== '_self') { return; }
        if (form.getAttribute('data-no-ajax') !== null) { return; }

        // Sin action el navegador manda a la URL actual, consulta incluida.
        var action = form.getAttribute('action') ||
            (location.pathname + location.search);
        if (esAjeno(action)) { return; }

        if (form.dataset.enviando === '1') { e.preventDefault(); return; }

        e.preventDefault();
        form.dataset.enviando = '1';

        var botones = form.querySelectorAll(
            'button[type="submit"], button:not([type]), input[type="submit"]');
        var pulsado = (e.originalEvent && e.originalEvent.submitter) || ultimoPulsado;
        ultimoPulsado = null;
        // type==='submit' cubre tanto <input type=submit> como <button> sin tipo,
        // que el navegador trata como de envío. Un <button type="button"> no.
        if (pulsado && pulsado.form === form && pulsado.type === 'submit' && pulsado.name) {
            // El name/value del botón viaja aparte: se añade después de
            // serializar porque los botones se desactivan a continuación y un
            // botón desactivado no entra en FormData.
            var p = cuerpo(form);
            if (p.tipo === null) {
                // multipart: se reconstruye con el campo del botón.
                var fd = new FormData(form);
                fd.append(pulsado.name, pulsado.value);
                p = { body: fd, tipo: null };
            } else {
                var u = new URLSearchParams(p.body);
                u.append(pulsado.name, pulsado.value);
                p = { body: u.toString(), tipo: p.tipo };
            }
            enviar(form, botones, action, p);
            return;
        }

        enviar(form, botones, action, cuerpo(form));
    });

    function enviar(form, botones, action, p) {
        // Desactivar mientras vuelve: es lo que evita que un doble clic
        // mande dos cotizaciones. Se desactiva después de serializar, no
        // antes, para no perder el name/value del botón pulsado.
        for (var i = 0; i < botones.length; i++) { botones[i].disabled = true; }

        var encabezados = { 'X-Requested-With': 'XMLHttpRequest' };
        if (p.tipo) { encabezados['Content-Type'] = p.tipo; }
        var tok = cookie('erpgo_csrf');
        if (tok) { encabezados['X-CSRF-Token'] = tok; }

        fetch(action, {
            method: 'POST',
            body: p.body,
            credentials: 'same-origin',
            cache: 'no-store',
            headers: encabezados
        }).then(function (res) {
            return res.text().then(function (texto) {
                terminado(form, botones, res, texto);
            });
        }).catch(function () {
            restaurar(form, botones);
            fallo('No se pudo contactar con el servidor. Intente de nuevo.');
        });
    }
})();
