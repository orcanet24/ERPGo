// viewpref.js — Vistas y plantillas por usuario.
//
// El servidor guarda la disposición de una pantalla y la devuelve tal cual; este
// archivo decide qué se hace con ella: qué piezas se ven, cuáles se esconden y
// qué plantillas hay guardadas.
//
// Todo pasa por /user/view-pref/{pantalla}, que exige el token CSRF en los POST;
// csrf.js ya lo adjunta automáticamente, así que aquí no se gestiona.
//
// La pantalla declara sus piezas con data-widget="clave" y este archivo solo
// necesita saber cuáles son. No hay una lista escrita en el JavaScript: si
// mañana el panel muestra otra ficha, el personalizador la recoge sin que nadie
// la añada a mano en dos sitios distintos.
(function () {
    'use strict';

    var HIDE_CLASS = 'hidden';

    function endpoint(view, name) {
        var url = '/user/view-pref/' + encodeURIComponent(view);
        return name ? url + '?name=' + encodeURIComponent(name) : url;
    }

    var api = {
        // prefs es un objeto plano: {hidden: ['por_cobrar', 'ventas_hoy']}.
        load: function (view, name) {
            return $.getJSON(endpoint(view, name));
        },
        save: function (view, prefs, name) {
            return $.post(endpoint(view), {
                payload: JSON.stringify(prefs || {}),
                name: name || ''
            });
        },
        remove: function (view, name) {
            return $.post(endpoint(view), { action: 'delete', name: name });
        }
    };

    // hiddenList extrae las piezas ocultas de lo que devolvió el servidor.
    // Se acepta también un arreglo pelado porque es la forma más simple que
    // puede guardar alguien; decidirlo aquí evita repetir la comprobación en
    // cada pantalla.
    function hiddenList(prefs) {
        if (!prefs) return [];
        if ($.isArray(prefs)) return prefs;
        if ($.isArray(prefs.hidden)) return prefs.hidden;
        return [];
    }

    // Editor es el personalizador de una pantalla.
    //
    // opts: {modal, list, templates, name, saveAs, remove, reset}
    //   todos son selectores jQuery salvo el nombre de la opción, que es la
    //   clave de la pantalla ("dashboard", "invoice.list").
    function Editor(view, opts) {
        this.view = view;
        this.opts = opts;
        this.$modal = $(opts.modal);
        this.$list = $(opts.list);
        this.$templates = $(opts.templates);
        this.$name = $(opts.name);
        this.bind();
        this.start();
    }

    // root es donde viven las piezas con data-widget.
    Editor.prototype.root = function () {
        return this.opts.root ? $(this.opts.root) : this.$modal;
    };

    Editor.prototype.widgets = function () {
        return this.root().find('[data-widget]');
    };

    // hidden son las claves de las piezas ocultas ahora mismo.
    Editor.prototype.hidden = function () {
        var out = [];
        this.widgets().each(function () {
            var $w = $(this);
            if ($w.hasClass(HIDE_CLASS)) out.push($w.attr('data-widget'));
        });
        return out;
    };

    Editor.prototype.prefs = function () {
        return { hidden: this.hidden() };
    };

    // apply deja la pantalla como dice la vista. Toggle por clase y no por
    // display:none en línea para que al recargar siga mandando la vista y no el
    // estilo que quedó pegado en el DOM.
    Editor.prototype.apply = function (prefs) {
        var hidden = hiddenList(prefs);
        this.widgets().each(function () {
            var $w = $(this);
            var isHidden = $.inArray($w.attr('data-widget'), hidden) !== -1;
            $w.toggleClass(HIDE_CLASS, isHidden);
        });
        this.syncList();
    };

    // syncList refleja el estado real de la pantalla en las casillas. Se llama
    // también al aplicar una plantilla, para que el formulario no se quede
    // marcando lo que ya no está.
    Editor.prototype.syncList = function () {
        var hidden = this.hidden();
        this.$list.find('input[type=checkbox]').each(function () {
            $(this).prop('checked', $.inArray($(this).val(), hidden) === -1);
        });
    };

    // buildForm arma el listado de piezas con el texto que el usuario ya ve en
    // pantalla, no con una copia que pueda quedarse vieja.
    Editor.prototype.buildForm = function () {
        var list = this.$list;
        this.$list.empty();
        this.widgets().each(function () {
            var $w = $(this);
            var label = $w.attr('data-widget-label') || $w.find('.info-box-text').first().text().trim();
            $('<label class="checkbox"></label>')
                .append($('<input type="checkbox">').val($w.attr('data-widget')))
                .append(document.createTextNode(' ' + label))
                .appendTo(list);
        });
        this.syncList();
    };

    // persist guarda la vista activa. Es un autoguardado: cambiar una casilla
    // ya es decidir cómo se ve la pantalla, y pedir un segundo "Guardar" después
    // sería pedir la confirmación de algo que el usuario acaba de hacer.
    //
    // Nunca hay dos guardados en vuelo. Con una casilla por detrás, dos POST
    // salen prácticamente en el mismo instante y el servidor no tiene por qué
    // escribirlos en ese orden: el que llegara último se encontraría la fila
    // ya cambiada y el resultado sería una vista con la casilla equivocada. Aquí
    // solo se admite una petición abierta y, si el usuario movió otra cosa
    // mientras tanto, se manda el estado más reciente al terminarla.
    Editor.prototype.persist = function () {
        var self = this;
        this.pending = this.prefs();
        if (this.saving) return this.saving;

        this.saving = api.save(this.view, this.pending).fail(function () {
            toastr.error('No se pudo guardar su vista');
        }).always(function () {
            self.saving = null;
            if (self.pending) self.persist();
        });
        this.pending = null;
        return this.saving;
    };

    Editor.prototype.refreshTemplates = function () {
        var self = this;
        var selected = this.$templates.val();
        return api.load(this.view).done(function (d) {
            self.$templates.empty().append($('<option></option>').val('').text('Vistas guardadas'));
            (d.saved || []).forEach(function (name) {
                self.$templates.append($('<option></option>').val(name).text(name));
            });
            if (selected) self.$templates.val(selected);
        });
    };

    Editor.prototype.applyTemplate = function (name) {
        var self = this;
        if (!name) return;
        api.load(this.view, name).done(function (d) {
            self.apply(d.prefs);
            toastr.info('Vista "' + name + '" aplicada');
        });
    };

    Editor.prototype.saveAs = function () {
        var self = this;
        var name = $.trim(this.$name.val());
        if (!name) {
            toastr.warning('Póngale un nombre a la plantilla');
            this.$name.focus();
            return;
        }
        api.save(this.view, this.prefs(), name).done(function (d) {
            self.$name.val('');
            self.refreshTemplates();
            self.$templates.val(d.name);
            toastr.success(d.msg || 'Plantilla guardada');
        });
    };

    Editor.prototype.deleteTemplate = function () {
        var self = this;
        var name = this.$templates.val();
        if (!name) {
            toastr.warning('Elija la plantilla que quiere eliminar');
            return;
        }
        api.remove(this.view, name).done(function (d) {
            self.refreshTemplates();
            toastr.success(d.msg || 'Plantilla eliminada');
        });
    };

    Editor.prototype.reset = function () {
        this.apply({ hidden: [] });
        this.persist();
        toastr.info('Vista restablecida');
    };

    Editor.prototype.bind = function () {
        var self = this;

        this.$list.on('change', 'input[type=checkbox]', function () {
            var $w = self.widgets().filter('[data-widget="' + $(this).val() + '"]');
            $w.toggleClass(HIDE_CLASS, !this.checked);
            self.persist();
        });

        this.$templates.on('change', function () {
            self.applyTemplate($(this).val());
        });

        // El botón «Personalizar vista» vive en la barra de la pantalla, no
        // dentro de la rejilla de fichas; enganchado a root() nunca se enteraba
        // del clic y el personalizador no se abría. Va en el documento para que
        // funcione sin importar dónde ponga cada pantalla su botón.
        $(document).on('click', '[data-view-open]', function () {
            self.syncList();
            self.$modal.modal('show');
        });

        if (this.opts.saveAs) $(this.opts.saveAs).on('click', function () { self.saveAs(); });
        if (this.opts.remove) $(this.opts.remove).on('click', function () { self.deleteTemplate(); });
        if (this.opts.reset) $(this.opts.reset).on('click', function () { self.reset(); });
    };

    // start pinta el estado guardado. Va al final para que la pantalla no
    // aparezca con todas las fichas y luego se apaguen: el destello se nota.
    Editor.prototype.start = function () {
        var self = this;
        this.buildForm();
        api.load(this.view).done(function (d) {
            self.apply(d.prefs);
        });
        this.refreshTemplates();
    };

    window.ERPGo = window.ERPGo || {};
    window.ERPGo.viewPref = api;
    window.ERPGo.ViewEditor = Editor;
})();
