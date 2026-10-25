' Origen: COMPROBANTES DE RETENCION IVA E ISLR N°2 / Módulo10.bas
Attribute VB_Name = "Módulo10"
Sub Botón30_Haga_clic_en()
Attribute Botón30_Haga_clic_en.VB_ProcData.VB_Invoke_Func = " \n14"
'
' Botón30_Haga_clic_en Macro
'

'
    Range("D5").Select
    ActiveSheet.Range("$A$5:$S$77").AutoFilter Field:=4, Criteria1:="<", _
        Operator:=xlAnd
    Range("E93").Select
    ActiveWindow.SmallScroll Down:=-6
    ActiveSheet.Shapes("Button 28").Select
    ActiveSheet.Shapes("Button 28").Select
    Selection.Characters.Text = "Botón 28"
    With Selection.Characters(Start:=1, Length:=8).Font
        .Name = "Calibri"
        .FontStyle = "Normal"
        .Size = 11
        .Strikethrough = False
        .Superscript = False
        .Subscript = False
        .OutlineFont = False
        .Shadow = False
        .Underline = xlUnderlineStyleNone
        .ThemeColor = 2
        .TintAndShade = 0
        .ThemeFont = xlThemeFontNone
    End With
    ActiveSheet.Shapes("Button 30").Select
    Application.Goto Reference:="Bisel_Haga_clic_en"
    Selection.Delete
    Range("H85").Select
    ActiveSheet.Shapes("Button 28").Select
    ActiveWindow.SmallScroll Down:=-15
    Range("G91").Select
End Sub
