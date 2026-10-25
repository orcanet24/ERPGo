' Origen: SISTEMA_CONTABLE EXCEL / Hoja18.cls
Attribute VB_Name = "Hoja18"
Attribute VB_Base = "0{00020820-0000-0000-C000-000000000046}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = True
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = True
Attribute VB_Control = "Proceso3d, 1, 0, MSForms, CommandButton"
Private Sub Proceso2d_Click()

ser

'extrae

extraebalance2d


'copia formulas

Range("A154") = "*"
Range("A155") = "*"
Range("A156") = "*"

Range("B154") = "Subtotales"
Range("B155") = "Saldos"
Range("B156") = "Totales"


Range("c12:c153").Select
Range("c154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"

Range("d12:d153").Select
Range("d154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"

Range("e12:e153").Select
Range("e154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"

Range("f12:f153").Select
Range("f154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"


Range("G12:G153").Select
Range("G154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"


Range("Hf12:H153").Select
Range("H154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"

If Range("G154") > Range("H154") Then
     Range("H155") = Range("G154") - Range("H154")
     Else
    
    If Range("H154") > Range("G154") Then
         Range("G155") = Range("H154") - Range("G154")
            Else
                    
    End If

End If

Range("G154:G156").Select
Range("G156").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"

Range("H154:H156").Select
Range("H156").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"



If Range("H155") > 0 Then
     Range("F155") = "GANANCIA"
        
        Else
    
    If Range("G155") > 0 Then
        Range("F155") = "PERDIDA"
 
    Else
    End If

End If



Range("I12:I153").Select
Range("I154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"

Range("J12:J153").Select
Range("J154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"

If Range("I154") > Range("J154") Then
     Range("J155") = Range("I154") - Range("J154")
     Else
    
    If Range("J154") > Range("I154") Then
         Range("I155") = Range("J154") - Range("I154")
            Else
                    
    End If

End If

Range("I154:I156").Select
Range("I156").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"

Range("J154:J156").Select
Range("J156").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"



Range("K12:K153").Select
Range("K154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"

Range("L12:L153").Select
Range("L154").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-142]C:R[-1]C)"

If Range("K154") > Range("L154") Then
     Range("L155") = Range("K154") - Range("L154")
     Else
    
    If Range("L154") > Range("K154") Then
         Range("K155") = Range("L154") - Range("K154")
            Else
                    
    End If

End If

Range("K154:K156").Select
Range("K156").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"

Range("L154:L156").Select
Range("L156").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"

'ordena

orden2d

'elimina

filasvacias

Range("a1").Select

End Sub

Private Sub Proceso3d_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Proceso Terminado"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.

Range("a12:l250").Select
Selection.Borders(xlDiagonalDown).LineStyle = xlNone
Selection.Borders(xlDiagonalUp).LineStyle = xlNone
Selection.Borders(xlEdgeLeft).LineStyle = xlNone
Selection.Borders(xlEdgeTop).LineStyle = xlNone
Selection.Borders(xlEdgeBottom).LineStyle = xlNone
Selection.Borders(xlEdgeRight).LineStyle = xlNone
Selection.Borders(xlInsideVertical).LineStyle = xlNone
Selection.Borders(xlInsideHorizontal).LineStyle = xlNone
Selection.ClearContents


ser1

'extrae

extraebalance3d

'copia formulas

Range("A212") = "*"
Range("A213") = "*"
Range("A214") = "*"

Range("B212") = "Subtotales"
Range("B213") = "Saldos"
Range("B214") = "Totales"


Range("c12:c211").Select
Range("c212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"

Range("d12:d211").Select
Range("d212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"

Range("e12:e211").Select
Range("e212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"

Range("f12:f211").Select
Range("f212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"


Range("G12:G211").Select
Range("G212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"


Range("Hf12:H211").Select
Range("H212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"

If Range("G212") > Range("H212") Then
     Range("H213") = Range("G212") - Range("H212")
     Range("h213").Select
     Selection.NumberFormat = "#,##0.00"
     
     Else
    
    If Range("H212") > Range("G212") Then
         Range("G213") = Range("H212") - Range("G212")
         Range("g213").Select
         Selection.NumberFormat = "#,##0.00"
        
            Else
                    
    End If

End If

Range("G212:G214").Select
Range("G214").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"

Range("H212:H214").Select
Range("H214").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"



If Range("H213") > 0 Then
     Range("F213") = "GANANCIA"
        
        Else
    
    If Range("G213") > 0 Then
        Range("F213") = "PERDIDA"
 
    Else
    End If

End If



Range("I12:I211").Select
Range("I212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"

Range("J12:J211").Select
Range("J212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"

If Range("I212") > Range("J212") Then
     Range("J213") = Range("I212") - Range("J212")
     Range("j213").Select
     Selection.NumberFormat = "#,##0.00"
     
     Else
    
    If Range("J212") > Range("I212") Then
         Range("I213") = Range("J212") - Range("I212")
         Range("i213").Select
         Selection.NumberFormat = "#,##0.00"
     
            Else
                    
    End If

End If

Range("I212:I214").Select
Range("I214").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"

Range("J212:J214").Select
Range("J214").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"



Range("K12:K211").Select
Range("K212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"

Range("L12:L211").Select
Range("L212").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-200]C:R[-1]C)"

If Range("K212") > Range("L212") Then
     Range("L213") = Range("K212") - Range("L212")
     Range("l213").Select
     Selection.NumberFormat = "#,##0.00"
     
     Else
    
    If Range("L212") > Range("K212") Then
         Range("K213") = Range("L212") - Range("K212")
         Range("k213").Select
         Selection.NumberFormat = "#,##0.00"
     
            Else
                    
    End If

End If

Range("K212:K214").Select
Range("K214").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"

Range("L212:L214").Select
Range("L214").Activate
ActiveCell.FormulaR1C1 = "=SUM(R[-2]C:R[-1]C)"

Range("c212:l212").Select
linea

Range("g214:l214").Select
linea1


'ordena

orden3d

'elimina

filasvacias

Range("a1").Select

Respuesta = MsgBox(Mensaje, Estilo, Título)

End Sub
