' Origen: COMPROBANTES DE RETENCION IVA E ISLR N°2 / Módulo2.bas
Attribute VB_Name = "Módulo2"
Sub ListadodeRetenciones_1Rectánguloredondeado_Haga_clic_en()
Attribute ListadodeRetenciones_1Rectánguloredondeado_Haga_clic_en.VB_ProcData.VB_Invoke_Func = " \n14"
'
' ListadodeRetenciones_1Rectánguloredondeado_Haga_clic_en Macro
'

'
    Sheets("Listado de Retenciones").Select
    Sheets("Listado de Retenciones").Copy Before:=Sheets(4)
    Sheets("Listado de Retenciones (2)").Select
    Sheets("Listado de Retenciones (2)").Move After:=Sheets(6)
    Sheets("Listado de Retenciones").Select
    ActiveWindow.SmallScroll Down:=-3
    Rows("5:22").Select
    Selection.Delete Shift:=xlUp
    Range("A10").Select
End Sub
