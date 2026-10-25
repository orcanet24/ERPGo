' Origen: COMPROBANTES DE RETENCION IVA E ISLR N°2 / Módulo9.bas
Attribute VB_Name = "Módulo9"
Sub Botóndeopción653_Haga_clic_en()
Attribute Botóndeopción653_Haga_clic_en.VB_ProcData.VB_Invoke_Func = " \n14"
'
' Botóndeopción653_Haga_clic_en Macro
'

'
    Range("Q2").Select
    Selection.Copy
    Range("O6").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Application.CutCopyMode = False
End Sub
Sub Botóndeopción660_Haga_clic_en()
Attribute Botóndeopción660_Haga_clic_en.VB_ProcData.VB_Invoke_Func = " \n14"
'
' Botóndeopción660_Haga_clic_en Macro
'

'
    Range("Q3").Select
    Selection.Copy
    Range("O6").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Application.CutCopyMode = False
End Sub
