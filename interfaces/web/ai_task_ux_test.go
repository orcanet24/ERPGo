package web

import (
	"io/fs"
	"strings"
	"testing"

	"erpgo"
)

func TestLasTareasDeIASeGuardanYEjecutanSinRecargarLaVista(t *testing.T) {
	lista, err := fs.ReadFile(erpgo.Templates(), "templates/ai_task_list.html")
	if err != nil {
		t.Fatalf("no se pudo leer la lista de tareas: %v", err)
	}
	formulario, err := fs.ReadFile(erpgo.Templates(), "templates/ai_task_form.html")
	if err != nil {
		t.Fatalf("no se pudo leer el formulario de tareas: %v", err)
	}
	fuente := string(lista)
	inicio := strings.Index(fuente, "function runTask(")
	fin := strings.Index(fuente, "function delTask(")
	if inicio < 0 || fin <= inicio {
		t.Fatal("la lista no define runTask antes de delTask")
	}
	corrida := fuente[inicio:fin]
	for _, esperado := range []string{
		"$.ajax({ url: '/ai/tasks/run/' + id",
		"celdaEstado.html('<span class=\"label label-info\">En ejecución…</span>')",
		"tabla.ajax.reload(function ()",
		"}, false)",
		"toastr.success(",
		"toastr.error(",
	} {
		if !strings.Contains(corrida, esperado) {
			t.Errorf("la ejecución AJAX de tareas no contiene %q", esperado)
		}
	}
	if strings.Contains(corrida, "window.location") {
		t.Error("Ejecutar tarea no debe navegar fuera del listado")
	}

	form := string(formulario)
	for _, esperado := range []string{
		"e.preventDefault()",
		"$.ajax({ url: '/ai/tasks/save'",
		"toastr.success(r.msg",
		"toastr.error(",
		"data.action = accion",
		"if (accion === 'save_exit')",
		"window.location.assign(r.redirect || '/ai/tasks')",
		"name: 'id', value: r.id",
	} {
		if !strings.Contains(form, esperado) {
			t.Errorf("el formulario AJAX de tareas no contiene %q", esperado)
		}
	}
}
