' Origen: Calculos Salariales 2013 / Módulo2.bas
Attribute VB_Name = "Módulo2"
Sub Ir_BonoNOC()
Attribute Ir_BonoNOC.VB_ProcData.VB_Invoke_Func = " \n14"
    Sheets("HoraExtra_BonoNoct").Select
    Range("E2").Select
End Sub
Sub Ir_SalarioIntegral_Calc()
Attribute Ir_SalarioIntegral_Calc.VB_ProcData.VB_Invoke_Func = " \n14"
    Sheets("Salario Integral Calc").Select
    Range("C4").Select
End Sub
Sub Volver_y_Copiar()
Attribute Volver_y_Copiar.VB_ProcData.VB_Invoke_Func = " \n14"
    On Error GoTo Celdamala
    Valor = Range("E17").Value
    Sheets("Prestaciones").Select
    Editable = ActiveCell
    ActiveCell.Value = Valor
    Exit Sub
Celdamala:
MsgBox "Error, la celda debe estar posicionada en la columna de SUELDO", vbCritical
    Sheets("Prestaciones").Select
    Resume Next
End Sub
Sub Ir_Prestaciones()
Attribute Ir_Prestaciones.VB_ProcData.VB_Invoke_Func = " \n14"
    Sheets("Prestaciones").Select
End Sub
Sub configvaca()
Config_Vaca.Show
End Sub

Sub Ver_About()
About.Show
End Sub

