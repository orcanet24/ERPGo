' Origen: SISTEMA_CONTABLE EXCEL / Hoja28.cls
Attribute VB_Name = "Hoja28"
Attribute VB_Base = "0{00020820-0000-0000-C000-000000000046}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = True
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = True
Attribute VB_Control = "Combomesrc, 6, 0, MSForms, ComboBox"
Attribute VB_Control = "Label1, 7, 1, MSForms, Label"
Attribute VB_Control = "Botonconsultarc, 8, 2, MSForms, CommandButton"
Private Sub Botonconsultarc_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Proceso Terminado"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.

CONSULTARC

Range("A12:AC200").Select
    With Selection.Font
        .Name = "Arial Narrow"
        .Size = 10
        .Strikethrough = False
        .Superscript = False
        .Subscript = False
        .OutlineFont = False
        .Shadow = False
        .Underline = xlUnderlineStyleNone
        .ThemeColor = xlThemeColorLight1
        .TintAndShade = 0
        .ThemeFont = xlThemeFontNone
    End With
    With Selection.Font
        .Name = "Arial Narrow"
        .Size = 10
        .Strikethrough = False
        .Superscript = False
        .Subscript = False
        .OutlineFont = False
        .Shadow = False
        .Underline = xlUnderlineStyleNone
        .ThemeColor = xlThemeColorLight1
        .TintAndShade = 0
        .ThemeFont = xlThemeFontNone
    End With

Range("K12:U200").Select
    Selection.Style = "Comma"

If Range("A75") > "" Then
     
   Rows("75:76").Select
   Selection.Insert Shift:=xlDown
   Range("A75") = "*"
   Range("J75") = "VAN..."
   Range("A76") = "*"
   Range("J76") = "VIENEN..."
   
   Range("K12:K75").Select
   Range("K75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("L12:L75").Select
   Range("L75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("M12:M75").Select
   Range("M75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("N12:N75").Select
   Range("N75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("O12:O75").Select
   Range("O75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("P12:P75").Select
   Range("P75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("Q12:Q75").Select
   Range("Q75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("R12:R75").Select
   Range("R75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("S12:S75").Select
   Range("S75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("T12:T75").Select
   Range("T75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("U12:U75").Select
   Range("U75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("K76") = Range("K75")
   Range("L76") = Range("L75")
   Range("M76") = Range("M75")
   Range("N76") = Range("N75")
   Range("O76") = Range("O75")
   Range("P76") = Range("P75")
   Range("Q76") = Range("Q75")
   Range("R76") = Range("R75")
   Range("S76") = Range("S75")
   Range("T76") = Range("T75")
   Range("U76") = Range("U75")
   
   Range("K75:U75").Select
   linea

Else

   If Range("A75") = "" And Range("a12") > "" Then
   
   Range("A75") = "*"
   Range("J75") = "TOTAL"
   
   Range("K12:K75").Select
   Range("K75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("L12:L75").Select
   Range("L75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("M12:M75").Select
   Range("M75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("N12:N75").Select
   Range("N75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("O12:O75").Select
   Range("O75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("P12:P75").Select
   Range("P75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("Q12:Q75").Select
   Range("Q75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("R12:R75").Select
   Range("R75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("S12:S75").Select
   Range("S75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("T12:T75").Select
   Range("T75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("U12:U75").Select
   Range("U75").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"

   Range("K75:U75").Select
   linea1
   Else
   End If

End If

If Range("A139") = "" And Range("a77") > "" Then
   
   Range("A139") = "*"
   Range("J139") = "TOTAL"
   
      
   Range("K76:K139").Select
   Range("K139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("L76:L139").Select
   Range("L139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("M76:M139").Select
   Range("M139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("N76:N139").Select
   Range("N139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("O76:O139").Select
   Range("O139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("P76:P139").Select
   Range("P139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("Q76:Q139").Select
   Range("Q139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"
   
   Range("R76:R139").Select
   Range("R139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"

   Range("S76:S139").Select
   Range("S139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"

   Range("T76:T139").Select
   Range("T139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"

   Range("U76:U139").Select
   Range("U139").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-63]C:R[-1]C)"

   Range("K139:U139").Select
   linea1
   Else
   End If

filasvacias

Range("a1").Select

Respuesta = MsgBox(Mensaje, Estilo, Título)

End Sub

Private Sub Combomesrc_Change()
Hoja28.Cells(2, 31).Value = Combomesrc.Column(1)
Hoja28.Cells(3, 31).Value = Combomesrc.Column(0)
End Sub
