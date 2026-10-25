' Origen: COMPROBANTES DE RETENCION IVA E ISLR N°2 / Módulo4.bas
Attribute VB_Name = "Módulo4"
Sub ComprdeRetSyS_11Rectánguloredondeado_Haga_clic_en()
Attribute ComprdeRetSyS_11Rectánguloredondeado_Haga_clic_en.VB_ProcData.VB_Invoke_Func = " \n14"
'
' ComprdeRetSyS_11Rectánguloredondeado_Haga_clic_en Macro
'

'
    Range("A16:D16").Select
    Selection.Copy
    Sheets("Listado de Retenciones").Select
    Range("A4").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Sheets("Compr. de Ret. SyS").Select
    Range("E16:H16").Select
    Application.CutCopyMode = False
    Selection.Copy
    Sheets("Listado de Retenciones").Select
    Range("B4").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Sheets("Compr. de Ret. SyS").Select
    Range("I16").Select
    Application.CutCopyMode = False
    Selection.Copy
    Sheets("Listado de Retenciones").Select
    Range("G4").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Sheets("Compr. de Ret. SyS").Select
    Range("C22").Select
    Application.CutCopyMode = False
    Selection.Copy
    Sheets("Listado de Retenciones").Select
    Range("C4").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Sheets("Compr. de Ret. SyS").Select
    Range("D22").Select
    Application.CutCopyMode = False
    Selection.Copy
    Sheets("Listado de Retenciones").Select
    Range("F4").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Sheets("Compr. de Ret. SyS").Select
    Range("H22:I22").Select
    Application.CutCopyMode = False
    Selection.Copy
    Sheets("Listado de Retenciones").Select
    Range("H4").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Sheets("Compr. de Ret. SyS").Select
    Range("M22").Select
    Application.CutCopyMode = False
    Selection.Copy
    Sheets("Listado de Retenciones").Select
    Range("I4").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Sheets("Compr. de Ret. SyS").Select
    Range("N22").Select
    Application.CutCopyMode = False
    Selection.Copy
    Sheets("Listado de Retenciones").Select
    Range("J4").Select
    Selection.PasteSpecial Paste:=xlPasteValues, Operation:=xlNone, SkipBlanks _
        :=False, Transpose:=False
    Rows("4:4").Select
    Application.CutCopyMode = False
    Selection.Insert Shift:=xlDown, CopyOrigin:=xlFormatFromLeftOrAbove
    Sheets("Compr. de Ret. SyS").Select
    Range("A19").Select
End Sub
