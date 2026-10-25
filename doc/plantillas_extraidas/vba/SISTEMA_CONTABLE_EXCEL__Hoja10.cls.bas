' Origen: SISTEMA_CONTABLE EXCEL / Hoja10.cls
Attribute VB_Name = "Hoja10"
Attribute VB_Base = "0{00020820-0000-0000-C000-000000000046}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = True
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = True
Attribute VB_Control = "Consultamayor, 1, 0, MSForms, CommandButton"
Attribute VB_Control = "Label1, 2, 1, MSForms, Label"
Attribute VB_Control = "ComboBox1, 3, 2, MSForms, ComboBox"


Private Sub ComboBox1_Change()
Hoja10.Cells(4, 8).Value = ComboBox1.Column(1)
Hoja10.Cells(4, 7).Value = ComboBox1.Column(0)
End Sub

Private Sub Consultamayor_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Proceso Terminado"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.

'Range("a12:h250").Select
Rows("12:250").Select
Selection.ClearContents

filtrocuenta




If Hoja10.Cells(4, 8).Value > 0 Then

    Rows("12:250").Select
    Selection.ClearContents
    
    consultamayor1
    ordenmayor
    
    Range("A12:H280").Select
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
Else

    Rows("12:250").Select
    Selection.ClearContents
    
    consultamayor2
    ordenmayor
    
    Range("A12:H280").Select
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
End If

'-----------------------------------------

If Range("A71") > "" Then
     
   Rows("71:72").Select
   Selection.Insert Shift:=xlDown
   Range("A71") = "*"
   Range("f71") = "VAN..."
   Range("A72") = "*"
   Range("f72") = "VIENEN..."

   Range("g12:g71").Select
   Range("g71").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("h12:h71").Select
   Range("h71").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("g72") = Range("g71")
   Range("h72") = Range("h71")

   Range("g71:h1250").Select
   borrarlineas
   
   Range("g71:h71").Select
   linea

Else

   If Range("a71") = "" And Range("A12") > "" Then

   Range("A71") = "*"
   Range("A72") = "*"
   Range("A73") = "*"
   Range("f71") = "SUBTOTALES"
   Range("f72") = "SALDO"
   Range("f73") = "TOTALES"
    
   Range("g12:g71").Select
   Range("g71").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
    
   Range("h12:h71").Select
   Range("h71").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"
    
   Range("g71:h1250").Select
   borrarlineas
    
   Range("g71:h71").Select
   linea

            If Range("g71") > Range("h71") Then
            Range("h72") = Range("g71") - Range("h71")
            Range("h72").Select
            Selection.NumberFormat = "#,##0.00"
            Else
    
                 If Range("h71") > Range("g71") Then
                 Range("g72") = Range("h71") - Range("g71")
                 Range("g72").Select
                 Selection.NumberFormat = "#,##0.00"
                 Else
        
                 End If
    
            End If

   Range("g71:g73").Select
   Range("g73").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-3]C:R[-1]C)"
   Range("g73").Select
   Selection.NumberFormat = "#,##0.00"
    
   Range("h71:h73").Select
   Range("h73").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-3]C:R[-1]C)"
   Range("h73").Select
   Selection.NumberFormat = "#,##0.00"
    
   Range("g73:h73").Select
   linea1
     
   Else
         
   End If

End If
        
'-------------------------------

If Range("A131") > "" Then
     
   Rows("131:132").Select
   Selection.Insert Shift:=xlDown
   Range("A131") = "*"
   Range("f131") = "VAN..."
   Range("A132") = "*"
   Range("f132") = "VIENEN..."

   Range("g72:g131").Select
   Range("g131").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("h72:h131").Select
   Range("h131").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("g132") = Range("g131")
   Range("h132") = Range("h131")

   Range("g131:h1250").Select
   borrarlineas
   
   Range("g131:h131").Select
   linea

Else

   If Range("a131") = "" And Range("A74") > "" Then

   Range("A131") = "*"
   Range("A132") = "*"
   Range("A133") = "*"
   Range("f131") = "SUBTOTALES"
   Range("f132") = "SALDO"
   Range("f133") = "TOTALES"

   Range("g72:g131").Select
   Range("g131").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("h72:h131").Select
   Range("h131").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("g131:h1250").Select
   borrarlineas
    
   Range("g131:h131").Select
   linea

            If Range("g131") > Range("h131") Then
            Range("h132") = Range("g131") - Range("h131")
            Range("h132").Select
            Selection.NumberFormat = "#,##0.00"
            Else

                 If Range("h131") > Range("g131") Then
                 Range("g132") = Range("h131") - Range("g131")
                 Range("g132").Select
                 Selection.NumberFormat = "#,##0.00"
                 Else
       
                 End If
     
            End If

    Range("g131:g133").Select
    Range("g133").Activate
    ActiveCell.FormulaR1C1 = "=SUM(R[-3]C:R[-1]C)"
    Range("g133").Select
    Selection.NumberFormat = "#,##0.00"
 
    Range("h131:h133").Select
    Range("h133").Activate
    ActiveCell.FormulaR1C1 = "=SUM(R[-3]C:R[-1]C)"
    Range("h133").Select
    Selection.NumberFormat = "#,##0.00"
        
    Range("g133:h133").Select
    linea1
    Else
        
    End If
End If
        
'-------------------------------
If Range("A191") > "" Then
     
   Rows("191:192").Select
   Selection.Insert Shift:=xlDown
   Range("A191") = "*"
   Range("f191") = "VAN..."
   Range("A192") = "*"
   Range("f192") = "VIENEN..."

   Range("g132:g191").Select
   Range("g191").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("h132:h191").Select
   Range("h191").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("g192") = Range("g191")
   Range("h192") = Range("h191")

   Range("g191:h1250").Select
   borrarlineas
   
   Range("g191:h191").Select
   linea

Else

   If Range("a191") = "" And Range("a134") > "" Then

   Range("A191") = "*"
   Range("A192") = "*"
   Range("A193") = "*"
   Range("f191") = "SUBTOTALES"
   Range("f192") = "SALDO"
   Range("f193") = "TOTALES"

   Range("g132:g191").Select
   Range("g191").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("h132:h191").Select
   Range("h191").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("g191:h1250").Select
   borrarlineas
   
   Range("g191:h191").Select
   linea

           If Range("g191") > Range("h191") Then
           Range("h192") = Range("g191") - Range("h191")
           Range("h192").Select
           Selection.NumberFormat = "#,##0.00"
    
           Else

                 If Range("h191") > Range("g191") Then
                 Range("g192") = Range("h191") - Range("g191")
                 Range("g192").Select
                 Selection.NumberFormat = "#,##0.00"
                 Else

                 End If
    
           End If

   Range("g191:g193").Select
   Range("g193").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-3]C:R[-1]C)"
   Range("g193").Select
   Selection.NumberFormat = "#,##0.00"
 
   Range("h191:h193").Select
   Range("h193").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-3]C:R[-1]C)"
   Range("h193").Select
   Selection.NumberFormat = "#,##0.00"
    
   Range("g193:h193").Select
   linea1

   Else


   End If
 
End If
        
'-------------------------------

If Range("A251") > "" Then
     
   Rows("251:252").Select
   Selection.Insert Shift:=xlDown
   Range("A251") = "*"
   Range("f251") = "VAN..."
   Range("A252") = "*"
   Range("f252") = "VIENEN..."

   Range("g192:g251").Select
   Range("g251").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("h192:h251").Select
   Range("h251").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("g252") = Range("g251")
   Range("h252") = Range("h251")

   Range("g251:h1250").Select
   borrarlineas
   
   Range("g251:h251").Select
   linea

Else

   If Range("a251") = "" And Range("a194") > "" Then

   Range("A251") = "*"
   Range("A252") = "*"
   Range("A253") = "*"
   Range("f251") = "SUBTOTALES"
   Range("f252") = "SALDO"
   Range("f253") = "TOTALES"

   Range("g192:g251").Select
   Range("g251").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("h292:h251").Select
   Range("h251").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-59]C:R[-1]C)"

   Range("g251:h1250").Select
   borrarlineas
   
   Range("g251:h251").Select
   linea

           If Range("g251") > Range("h251") Then
           Range("h252") = Range("g251") - Range("h251")
           Range("h252").Select
           Selection.NumberFormat = "#,##0.00"
    
           Else

                 If Range("h251") > Range("g251") Then
                 Range("g252") = Range("h251") - Range("g251")
                 Range("g252").Select
                 Selection.NumberFormat = "#,##0.00"
                 Else

                 End If
    
           End If

   Range("g251:g253").Select
   Range("g253").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-3]C:R[-1]C)"
   Range("g253").Select
   Selection.NumberFormat = "#,##0.00"
 
   Range("h251:h253").Select
   Range("h253").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-3]C:R[-1]C)"
   Range("h253").Select
   Selection.NumberFormat = "#,##0.00"
    
   Range("g253:h253").Select
   linea1

   Else


   End If
 
End If
'-----------------------
 
 
    
    'elimina
    
filasvacias

Range("a1").Select

Respuesta = MsgBox(Mensaje, Estilo, Título)

End Sub
