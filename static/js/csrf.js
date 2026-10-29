// csrf.js — Token de seguridad en las peticiones.
//
// El servidor exige un token en toda petición que cambia datos (POST, PUT,
// DELETE). Este archivo lo adjunta automáticamente para que ningún formulario
// ni llamada AJAX tenga que acordarse de hacerlo.
//
// Va en un solo lugar porque el olvido es el fallo: un endpoint protegido que
// no manda el token se rompe en producción, y un endpoint sin protección en una
// pantalla que sí lo manda sigue desprotegido. Centralizarlo hace que las dos
// cosas sean el mismo código.
(function () {
    'use strict';

    var COOKIE = 'erpgo_csrf';
    var HEADER = 'X-CSRF-Token';

    // token() lee la cookie. Es pública a propósito: sin SameSite la lectura
    // desde JavaScript es justamente lo que permite al código de la propia
    // aplicación conseguir el token.
    function token() {
        var parts = document.cookie ? document.cookie.split(';') : [];
        for (var i = 0; i < parts.length; i++) {
            var pair = parts[i].trim();
            if (pair.indexOf(COOKIE + '=') === 0) {
                return decodeURIComponent(pair.substring(COOKIE.length + 1));
            }
        }
        return '';
    }

    // Peticiones fetch que modifican datos también necesitan el mismo token
    // que jQuery.ajax. Mantener la regla aquí evita que una pantalla nueva lo
    // olvide: el servidor sigue siendo quien valida y rechaza si falta.
    function fetchSeguro(input, init) {
        var opciones = init ? Object.assign({}, init) : {};
        var metodo = (opciones.method || (input && input.method) || 'GET').toUpperCase();
        var destino = typeof input === 'string' ? input : (input && input.url) || String(input);
        var mismaOrigen = false;
        try { mismaOrigen = new URL(destino, window.location.href).origin === window.location.origin; }
        catch (e) { mismaOrigen = false; }

        if (mismaOrigen && metodo !== 'GET' && metodo !== 'HEAD' && metodo !== 'OPTIONS' && metodo !== 'TRACE') {
            var cabeceras = new Headers(opciones.headers || (input && input.headers) || {});
            var t = token();
            if (t) { cabeceras.set(HEADER, t); }
            opciones.headers = cabeceras;
        }
        return window.fetch(input, opciones);
    }

    window.ERPGo = window.ERPGo || {};
    window.ERPGo.csrfFetch = fetchSeguro;

    function attach() {
        var t = token();
        if (!t) return;
        jQuery.ajaxSetup({
            beforeSend: function (xhr, settings) {
                // Solo en peticiones que cambian estado: ponerlo en un GET no
                // protege nada y ensucia la URL de la caché del navegador.
                var m = (settings.type || 'GET').toUpperCase();
                if (m === 'GET' || m === 'HEAD') return;
                if (!settings.crossDomain) {
                    xhr.setRequestHeader(HEADER, t);
                }
            }
        });
    }

    // Formularios HTML clásicos: no pasan por $.ajax, así que jQuery ajaxSetup
    // no los cubre. Se inyecta un campo oculto en cada envío.
    function hookForms() {
        jQuery(document).on('submit', 'form', function () {
            var $f = jQuery(this);
            if ($f.find('input[name="csrf_token"]').length) return;
            var t = token();
            if (!t) return;
            jQuery('<input>', { type: 'hidden', name: 'csrf_token', value: t }).appendTo($f);
        });
    }

    if (typeof jQuery !== 'undefined') {
        jQuery(function () {
            attach();
            hookForms();
        });
    }
})();