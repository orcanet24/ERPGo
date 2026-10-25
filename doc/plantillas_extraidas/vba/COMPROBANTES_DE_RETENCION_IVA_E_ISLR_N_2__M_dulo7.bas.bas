' Origen: COMPROBANTES DE RETENCION IVA E ISLR N°2 / Módulo7.bas
Attribute VB_Name = "Módulo7"
Sub ComprdeRet_4Rectánguloredondeado_Haga_clic_en()
Attribute ComprdeRet_4Rectánguloredondeado_Haga_clic_en.VB_ProcData.VB_Invoke_Func = " \n14"
'
' ComprdeRet_4Rectánguloredondeado_Haga_clic_en Macro
'

'
    Sheets("Listado de Ret.-IVA").Select
    Rows("6:6").Select
    Selection.Insert Shift:=xlDown, CopyOrigin:=xlFormatFromLeftOrAbove
    Rows("4:4").Select
    Selection.Copy
    Rows("6:6").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Sheets("Compr. de Ret.").Select
    Range("N22").Select
    Application.CutCopyMode = False
    Selection.Copy
    Range("O15").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Application.CutCopyMode = False
    Range("P21").Select
    Range("Q2").Select
    Application.CutCopyMode = False
    Selection.Copy
    Range("O6").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Application.CutCopyMode = False
    Range("P21").Select
End Sub
Sub ComprdeRet_2Rectánguloredondeado_Haga_clic_en()
Attribute ComprdeRet_2Rectánguloredondeado_Haga_clic_en.VB_ProcData.VB_Invoke_Func = " \n14"
'
' ComprdeRet_2Rectánguloredondeado_Haga_clic_en Macro
'

'
    Sheets("Listado de Ret.-ISLR").Select
    Rows("6:6").Select
    Selection.Insert Shift:=xlDown, CopyOrigin:=xlFormatFromLeftOrAbove
    Rows("4:4").Select
    Selection.Copy
    Rows("6:6").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Sheets("Compr. de Ret.").Select
    Range("N22").Select
    Application.CutCopyMode = False
    Selection.Copy
    Range("O17").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Range("Q3").Select
    Application.CutCopyMode = False
    Selection.Copy
    Range("O6").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Application.CutCopyMode = False
    Range("P21").Select
End Sub
