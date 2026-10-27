package web

// Pruebas del árbol de categorías jerárquicas (Fase F1 del plan 2026-10-26).
//
// Se prueban las funciones puras `construirArbol` y `productosRama`, que son
// donde está lo delicado: ordenar por hermano (sort_order, id) y no por
// texto —el 10 antes que el 2—, promover huérfanos a raíz, y sumar productos
// por rama completa. La consulta misma es un SELECT plano que no necesita
// servidor; estas son las decisiones que sí pueden romperse sin que nadie lo
// note: un catálogo mal ordenado o un conteo de rama que miente.

import "testing"

func TestConstruirArbolOrdenaPorHermanoNoPorTexto(t *testing.T) {
	// Raíces 1, 2 y 10; el 1 tiene dos hijos (11, 12). Si el orden fuera
	// lexicográfico, el 10 saldría antes que el 2 y el catálogo quedaría
	// raro para cualquier humano.
	todo := []catNodo{
		{ID: 10, Name: "Zeta"},
		{ID: 2, Name: "Beta"},
		{ID: 1, Name: "Alfa"},
		{ID: 12, Name: "Alfa/Hija-B", Parent: 1},
		{ID: 11, Name: "Alfa/Hija-A", Parent: 1},
	}
	plano := construirArbol(todo)

	esperado := []int64{1, 11, 12, 2, 10}
	if len(plano) != len(esperado) {
		t.Fatalf("esperaba %d nodos, salieron %d", len(esperado), len(plano))
	}
	for i, id := range esperado {
		if plano[i].ID != id {
			t.Errorf("posición %d: esperaba el id %d, salió %d", i, id, plano[i].ID)
		}
	}
	// La rama va con la profundidad correcta para indentar.
	profundidad := map[int64]int{1: 0, 11: 1, 12: 1, 2: 0, 10: 0}
	for _, n := range plano {
		if n.Depth != profundidad[n.ID] {
			t.Errorf("nodo %d: profundidad %d, esperada %d", n.ID, n.Depth, profundidad[n.ID])
		}
	}
}

func TestConstruirArbolPromueveHuerfanosARaiz(t *testing.T) {
	// Un padre inexistente (borrado a mano, dato heredado) no puede dejar a la
	// categoría invisible: sube a raíz.
	todo := []catNodo{
		{ID: 7, Name: "Huérfana", Parent: 99},
		{ID: 8, Name: "Raíz de verdad"},
	}
	plano := construirArbol(todo)
	if len(plano) != 2 {
		t.Fatalf("esperaba 2 nodos, salieron %d", len(plano))
	}
	for _, n := range plano {
		if n.ID == 7 && (n.Parent != 0 || n.Depth != 0) {
			t.Errorf("la huérfana 7 debe quedar como raíz; salió con padre %d y profundidad %d", n.Parent, n.Depth)
		}
	}
}

func TestConstruirArbolRompeCiclos(t *testing.T) {
	// Datos corruptos: 5 y 7 se apuntan mutuamente. Sin guard, bajar()
	// recursiona hasta reventar el stack y la página pantea. Con guard, TODO
	// miembro del ciclo sube a raíz y un hijo externo (9) conserva su padre
	// legítimo (5, ahora raíz).
	todo := []catNodo{
		{ID: 5, Name: "A", Parent: 7},
		{ID: 7, Name: "B", Parent: 5},
		{ID: 9, Name: "C", Parent: 5},
	}
	plano := construirArbol(todo) // antes de esto: recursión infinita
	if len(plano) != 3 {
		t.Fatalf("esperaba 3 nodos, salieron %d", len(plano))
	}
	por := map[int64]catNodo{}
	for _, n := range plano {
		por[n.ID] = n
	}
	for _, id := range []int64{5, 7} {
		if por[id].Parent != 0 || por[id].Depth != 0 {
			t.Errorf("miembro del ciclo %d debe subir a raíz; salió con padre %d y profundidad %d", id, por[id].Parent, por[id].Depth)
		}
	}
	if por[9].Parent != 5 || por[9].Depth != 1 {
		t.Errorf("el hijo externo 9 conserva su padre 5; salió padre %d profundidad %d", por[9].Parent, por[9].Depth)
	}
}

func TestConstruirArbolRompeAutobucle(t *testing.T) {
	// Ciclo mínimo: una sola categoría que es su propio padre.
	plano := construirArbol([]catNodo{
		{ID: 4, Name: "Solo", Parent: 4},
		{ID: 6, Name: "Raíz"},
	})
	if len(plano) != 2 {
		t.Fatalf("esperaba 2 nodos, salieron %d", len(plano))
	}
	for _, n := range plano {
		if n.Parent == 4 {
			t.Errorf("el autobucle del 4 rompió el árbol: el nodo %d quedó colgando de él", n.ID)
		}
	}
}

func TestProductosRamaSumaLaRamaCompleta(t *testing.T) {
	// 1 (2 directos) > 2 (3 directos) > 3 (5 directos): la rama de 1 vale 10.
	nodos := []catNodo{
		{ID: 1, Products: 2},
		{ID: 2, Parent: 1, Products: 3},
		{ID: 3, Parent: 2, Products: 5},
	}
	rama := productosRama(nodos)
	if rama[3] != 5 {
		t.Errorf("rama del nodo 3: %d, esperada 5", rama[3])
	}
	if rama[2] != 8 {
		t.Errorf("rama del nodo 2: %d, esperada 8 (3 propios + 5 de la hija)", rama[2])
	}
	if rama[1] != 10 {
		t.Errorf("rama del nodo 1: %d, esperada 10 (2 propios + 8 de la rama)", rama[1])
	}
}
