' Origen: SISTEMA_CONTABLE EXCEL / Hoja27.cls
Attribute VB_Name = "Hoja27"
Attribute VB_Base = "0{00020820-0000-0000-C000-000000000046}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = True
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = True
Attribute VB_Control = "Combomesrv, 4, 0, MSForms, ComboBox"
Attribute VB_Control = "Label1, 5, 1, MSForms, Label"
Attribute VB_Control = "Botonconsultarv, 6, 2, MSForms, CommandButton"
Private Sub Botonconsultarv_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Proceso Terminado"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.


CONSULTARV

Range("A11:W200").Select
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
    
Range("J11:R200").Select
    Selection.Style = "Comma"
    
If Range("A70") > "" Then
     
   Rows("70:71").Select
   Selection.Insert Shift:=xlDown
   Range("A70") = "*"
   Range("I70") = "VAN..."
   Range("A71") = "*"
   Range("I71") = "VIENEN..."
   
   Range("J11:J70").Select
   Range("J70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("K11:K70").Select
   Range("K70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("L11:L70").Select
   Range("L70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("M11:M70").Select
   Range("M70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("N11:N70").Select
   Range("N70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("O11:O70").Select
   Range("O70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("P11:P70").Select
   Range("P70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("Q11:Q70").Select
   Range("Q70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("R11:R70").Select
   Range("R70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("J71") = Range("J70")
   Range("K71") = Range("K70")
   Range("L71") = Range("L70")
   Range("M71") = Range("M70")
   Range("N71") = Range("N70")
   Range("O71") = Range("O70")
   Range("P71") = Range("P70")
   Range("Q71") = Range("Q70")
   Range("R71") = Range("R70")

   Range("j70:R70").Select
   linea

Else

   If Range("A70") = "" And Range("a11") > "" Then
   
   Range("A70") = "*"
   Range("I70") = "TOTAL"
   
   Range("J11:J70").Select
   Range("J70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("K11:K70").Select
   Range("K70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("L11:L70").Select
   Range("L70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("M11:M70").Select
   Range("M70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("N11:N70").Select
   Range("N70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("O11:O70").Select
   Range("O70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("P11:P70").Select
   Range("P70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("Q11:Q70").Select
   Range("Q70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"
   
   Range("R11:R70").Select
   Range("R70").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-60]C:R[-1]C)"

   Range("j70:R70").Select
   linea1
   Else
   End If

End If

If Range("A130") = "" And Range("a72") > "" Then
   
   Range("A130") = "*"
   Range("I130") = "TOTAL"
   
   Range("J71:J130").Select
   Range("J130").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
   
   Range("K71:K130").Select
   Range("K130").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
   
   Range("L71:L130").Select
   Range("L130").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
   
   Range("M71:M130").Select
   Range("M130").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
   
   Range("N71:N130").Select
   Range("N130").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
   
   Range("O71:O130").Select
   Range("O130").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
   
   Range("P71:P130").Select
   Range("P130").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
   
   Range("Q71:Q130").Select
   Range("Q130").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
   
   Range("R71:R130").Select
   Range("R130").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("j130:R130").Select
   linea1
   Else
   End If

filasvacias

Range("a1").Select

Respuesta = MsgBox(Mensaje, Estilo, Título)

End Sub

Private Sub Combomesrv_Change()
Hoja27.Cells(2, 25).Value = Combomesrv.Column(1)
Hoja27.Cells(3, 25).Value = Combomesrv.Column(0)
End Sub
