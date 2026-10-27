// integridad.js — el aviso central de los borrados que la base impide.
//
// POR QUÉ EXISTE. Cuando una operación choca con una regla de integridad, el
// texto que llega al navegador es un aviso para programar que dice qué regla se
// violó y qué hacer. Eso no puede salir en un toast de tres segundos que se va
// solo: es la explicación de por qué NO se borró algo, y quien lo intentó
// necesita leerla entera y decidir (corregir el dato, anular con un reverso,
// dejarlo inactivo).
//
// POR QUÉ AQUÍ Y NO EN CADA PANTALLA. Las plantillas llaman a `toastr.error`
// en más de cien sitios, cada una por su cuenta. Envolver esa llamada una sola
// vez cubre todas las pantallas que ya existen y las que se escriban después,
// sin tocar ninguna: si el mensaje es de la familia de los borrados impedidos,
// se enseña en el centro; si es cualquier otro error, se comporta igual que
// siempre (toast de la esquina).
//
// La familia de mensajes se reconoce por su texto: los del servidor —tanto los
// de la guarda del handler como los que devuelve el trigger de la base— empiezan
// por «No se puede eliminar…» o «No se puede comprobar…». Es una convención
// escrita aquí, en un solo sitio.
(function () {
    if (typeof toastr === 'undefined' || typeof toastr.error !== 'function') {
        return;
    }

    var FAMILIA = /^\s*No se puede (eliminar|comprobar|guardar)/i;
    var original = toastr.error;

    toastr.error = function (mensaje) {
        var texto = (typeof mensaje === 'string') ? mensaje : '';
        if (FAMILIA.test(texto) && typeof swal === 'function') {
            try {
                swal({
                    title: 'La operación no se puede completar',
                    text: texto,
                    type: 'error',
                    confirmButtonText: 'Entendido'
                });
                return;
            } catch (e) {
                // Si el diálogo central no está disponible, el aviso de siempre
                // es mejor que ningún aviso.
            }
        }
        return original.apply(toastr, arguments);
    };
})();
