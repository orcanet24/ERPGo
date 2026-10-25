package queue

import "context"

// tenantKey es la clave bajo la que el contexto lleva la empresa de la tarea.
type tenantKey struct{}

// WithTenant devuelve un contexto que identifica la empresa de la tarea. Todo
// handler debe leer la empresa de aquí y NO de un estado global: es lo que
// impide que una tarea de la empresa A se ejecute contra los datos de la B.
func WithTenant(ctx context.Context, tenant string) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenant)
}

// TenantFromContext devuelve la empresa de la tarea (vacía si no la hay).
func TenantFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tenantKey{}).(string); ok {
		return v
	}
	return ""
}

// rowBudgetKey lleva el presupuesto de filas de una tarea.
type rowBudgetKey struct{}

// WithRowBudget fija el máximo de filas que una tarea puede traer a memoria.
// Es un límite de memoria explícito: una consulta sin tope puede pedir dos
// millones de filas y tumbar el proceso por mucho que la cola esté acotada.
func WithRowBudget(ctx context.Context, max int) context.Context {
	if max <= 0 {
		max = 500
	}
	return context.WithValue(ctx, rowBudgetKey{}, max)
}

// RowBudgetFromContext devuelve el presupuesto de filas de la tarea.
func RowBudgetFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(rowBudgetKey{}).(int); ok && v > 0 {
		return v
	}
	return 500
}
