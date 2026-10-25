' Origen: SISTEMA_CONTABLE EXCEL / Hoja4.cls
Attribute VB_Name = "Hoja4"
Attribute VB_Base = "0{00020820-0000-0000-C000-000000000046}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = True
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = True
Attribute VB_Control = "CommandButton1, 7, 0, MSForms, CommandButton"
Attribute VB_Control = "CommandButton2, 11, 1, MSForms, CommandButton"
Attribute VB_Control = "CommandButton3, 13, 2, MSForms, CommandButton"
Attribute VB_Control = "CommandButton4, 37, 3, MSForms, CommandButton"
Attribute VB_Control = "BotonVentas, 54, 4, MSForms, CommandButton"
Attribute VB_Control = "BotonCompras, 55, 5, MSForms, CommandButton"
Attribute VB_Control = "CommandButton5, 90, 6, MSForms, CommandButton"
Attribute VB_Control = "CommandButton6, 91, 7, MSForms, CommandButton"
Private Sub BotonCompras_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Registro Guardado"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.

If Hoja4.Range("E4").Value = "Compras" Then

fila = Cells(2, 43)

Hoja24.Cells(fila, 1).Value = Range("mes").Value
Hoja24.Cells(fila, 2).Value = Range("voucher").Value
Hoja24.Cells(fila, 3).Value = Range("fecha").Value
Hoja24.Cells(fila, 4).Value = Hoja4.Range("B6").Value
Hoja24.Cells(fila, 5).Value = Hoja4.Range("K4").Value
Hoja24.Cells(fila, 6).Value = Hoja4.Range("B8").Value
Hoja24.Cells(fila, 7).Value = Hoja4.Range("B9").Value
Hoja24.Cells(fila, 8).Value = Hoja4.Range("h31").Value
Hoja24.Cells(fila, 9).Value = Hoja4.Range("B31").Value
Hoja24.Cells(fila, 10).Value = Hoja4.Range("C31").Value

Hoja24.Cells(fila, 11).Value = Hoja4.Range("AQ20").Value
Hoja24.Cells(fila, 12).Value = Hoja4.Range("AQ21").Value
Hoja24.Cells(fila, 13).Value = Hoja4.Range("AQ22").Value
Hoja24.Cells(fila, 14).Value = Hoja4.Range("AQ23").Value
Hoja24.Cells(fila, 15).Value = Hoja4.Range("AQ24").Value
Hoja24.Cells(fila, 16).Value = Hoja4.Range("AQ25").Value
Hoja24.Cells(fila, 17).Value = Hoja4.Range("AQ26").Value
Hoja24.Cells(fila, 18).Value = Hoja4.Range("AQ27").Value
Hoja24.Cells(fila, 19).Value = Hoja4.Range("AQ28").Value
Hoja24.Cells(fila, 20).Value = Hoja4.Range("AQ29").Value
Hoja24.Cells(fila, 21).Value = Hoja4.Range("AQ30").Value

Hoja24.Cells(fila, 22).Value = Hoja4.Range("B10").Value
Hoja24.Cells(fila, 23).Value = Hoja4.Range("E7").Value
Hoja24.Cells(fila, 24).Value = Hoja4.Range("K5").Value
Hoja24.Cells(fila, 25).Value = Hoja4.Range("E9").Value
Hoja24.Cells(fila, 26).Value = Hoja4.Range("E10").Value

Respuesta = MsgBox(Mensaje, Estilo, Título)

Else

MsgBox "Esta presionando el Botón Compras"

End If



End Sub

Private Sub BotonVentas_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Registro Guardado"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.

If Hoja4.Range("E4").Value = "Ventas" Then

fila = Cells(2, 42)

Hoja22.Cells(fila, 1).Value = Range("mes").Value
Hoja22.Cells(fila, 2).Value = Range("voucher").Value
Hoja22.Cells(fila, 3).Value = Range("fecha").Value
Hoja22.Cells(fila, 4).Value = Hoja4.Range("B6").Value
Hoja22.Cells(fila, 5).Value = Hoja4.Range("K4").Value
Hoja22.Cells(fila, 6).Value = Hoja4.Range("B8").Value
Hoja22.Cells(fila, 7).Value = Hoja4.Range("B9").Value
Hoja22.Cells(fila, 8).Value = Hoja4.Range("h31").Value
Hoja22.Cells(fila, 9).Value = Hoja4.Range("B31").Value
Hoja22.Cells(fila, 10).Value = Hoja4.Range("C31").Value

Hoja22.Cells(fila, 11).Value = Hoja4.Range("AQ11").Value
Hoja22.Cells(fila, 12).Value = Hoja4.Range("AQ12").Value
Hoja22.Cells(fila, 13).Value = Hoja4.Range("AQ13").Value
Hoja22.Cells(fila, 14).Value = Hoja4.Range("AQ14").Value
Hoja22.Cells(fila, 15).Value = Hoja4.Range("AQ15").Value
Hoja22.Cells(fila, 16).Value = Hoja4.Range("AQ16").Value
Hoja22.Cells(fila, 17).Value = Hoja4.Range("AQ17").Value
Hoja22.Cells(fila, 18).Value = Hoja4.Range("AQ18").Value
Hoja22.Cells(fila, 19).Value = Hoja4.Range("AQ19").Value

Hoja22.Cells(fila, 20).Value = Hoja4.Range("B10").Value
Hoja22.Cells(fila, 21).Value = Hoja4.Range("E7").Value
Hoja22.Cells(fila, 22).Value = Hoja4.Range("K5").Value
Hoja22.Cells(fila, 23).Value = Hoja4.Range("E9").Value
Hoja22.Cells(fila, 24).Value = Hoja4.Range("E10").Value

Respuesta = MsgBox(Mensaje, Estilo, Título)

Else

'msgbox=("Esta presionando el Botón Compras",vbInformation,"AVISO")
MsgBox "Esta presionando el Botón Ventas"

End If


End Sub

Private Sub CommandButton1_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Registro Guardado"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.

If Hoja4.Cells(12, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 3).Value = "*"
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)
Hoja8.Cells(fila, 6).Value = "------------------------- X -------------------------"
Else
End If


If Hoja4.Cells(12, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(12, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(12, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(12, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(12, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(12, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(12, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(12, 38).Value

Else
End If

If Hoja4.Cells(13, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(13, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(13, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(13, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(13, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(13, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(13, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(13, 38).Value

Else
End If

If Hoja4.Cells(14, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(14, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(14, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(14, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(14, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(14, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(14, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(14, 38).Value

Else
End If

If Hoja4.Cells(15, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(15, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(15, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(15, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(15, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(15, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(15, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(15, 38).Value

Else
End If

If Hoja4.Cells(16, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(16, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(16, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(16, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(16, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(16, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(16, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(16, 38).Value

Else
End If

If Hoja4.Cells(17, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(17, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(17, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(17, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(17, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(17, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(17, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(17, 38).Value

Else
End If

If Hoja4.Cells(18, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(18, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(18, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(18, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(18, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(18, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(18, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(18, 38).Value

Else
End If

If Hoja4.Cells(19, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(19, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(19, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(19, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(19, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(19, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(19, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(19, 38).Value

Else
End If

If Hoja4.Cells(20, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(20, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(20, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(20, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(20, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(20, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(20, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(20, 38).Value

Else
End If

If Hoja4.Cells(21, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(21, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(21, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(21, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(21, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(21, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(21, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(21, 38).Value

Else
End If

If Hoja4.Cells(22, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(22, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(22, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(22, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(22, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(22, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(22, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(22, 38).Value

Else
End If

If Hoja4.Cells(23, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(23, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(23, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(23, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(23, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(23, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(23, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(23, 38).Value

Else
End If

If Hoja4.Cells(24, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(24, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(24, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(24, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(24, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(24, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(24, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(24, 38).Value

Else
End If

If Hoja4.Cells(25, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(25, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(25, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(25, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(25, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(25, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(25, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(25, 38).Value

Else
End If

If Hoja4.Cells(26, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(26, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(26, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(26, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(26, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(26, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(26, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(26, 38).Value

Else
End If

If Hoja4.Cells(27, 1).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(27, 1).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(27, 2).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(27, 4).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(27, 5).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(27, 31).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(27, 34).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(27, 38).Value

Else
End If



'Destino



If Hoja4.Cells(12, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(12, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(12, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(12, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(12, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(12, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(12, 39).Value

Else
End If

If Hoja4.Cells(12, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(12, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(12, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(12, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(12, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(12, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(12, 40).Value

Else
End If

If Hoja4.Cells(13, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(13, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(13, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(13, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(13, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(13, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(13, 39).Value

Else
End If

If Hoja4.Cells(13, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(13, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(13, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(13, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(13, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(13, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(13, 40).Value

Else
End If

If Hoja4.Cells(14, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(14, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(14, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(14, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(14, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(14, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(14, 39).Value

Else
End If

If Hoja4.Cells(14, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(14, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(14, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(14, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(14, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(14, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(14, 40).Value

Else
End If

If Hoja4.Cells(15, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(15, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(15, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(15, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(15, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(15, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(15, 39).Value

Else
End If

If Hoja4.Cells(15, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(15, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(15, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(15, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(15, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(15, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(15, 40).Value

Else
End If

If Hoja4.Cells(16, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(16, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(16, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(16, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(16, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(16, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(16, 39).Value

Else
End If

If Hoja4.Cells(16, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(16, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(16, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(16, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(16, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(16, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(16, 40).Value

Else
End If

If Hoja4.Cells(17, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(17, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(17, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(17, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(17, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(17, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(17, 39).Value

Else
End If

If Hoja4.Cells(17, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(17, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(17, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(17, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(17, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(17, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(17, 40).Value

Else
End If

If Hoja4.Cells(18, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(18, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(18, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(18, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(18, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(18, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(18, 39).Value

Else
End If

If Hoja4.Cells(18, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(18, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(18, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(18, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(18, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(18, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(18, 40).Value

Else
End If

If Hoja4.Cells(19, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(19, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(19, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(19, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(19, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(19, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(19, 39).Value

Else
End If

If Hoja4.Cells(19, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(19, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(19, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(19, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(19, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(19, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(19, 40).Value

Else
End If

If Hoja4.Cells(20, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(20, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(20, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(20, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(20, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(20, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(20, 39).Value

Else
End If

If Hoja4.Cells(20, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(20, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(20, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(20, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(20, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(20, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(20, 40).Value

Else
End If

If Hoja4.Cells(21, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(21, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(21, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(21, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(21, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(21, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(21, 39).Value

Else
End If

If Hoja4.Cells(21, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(21, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(21, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(21, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(21, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(21, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(21, 40).Value

Else
End If

If Hoja4.Cells(22, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(22, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(22, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(22, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(22, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(22, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(22, 39).Value

Else
End If

If Hoja4.Cells(22, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(22, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(22, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(22, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(22, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(22, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(22, 40).Value

Else
End If

If Hoja4.Cells(23, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(23, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(23, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(23, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(23, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(23, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(23, 39).Value

Else
End If

If Hoja4.Cells(23, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(23, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(23, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(23, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(23, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(23, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(23, 40).Value

Else
End If

If Hoja4.Cells(24, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(24, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(24, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(24, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(24, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(24, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(24, 39).Value

Else
End If

If Hoja4.Cells(21, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(24, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(24, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(24, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(24, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(24, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(24, 40).Value

Else
End If

If Hoja4.Cells(25, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(25, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(25, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(25, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(25, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(25, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(25, 39).Value

Else
End If

If Hoja4.Cells(25, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(25, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(25, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(25, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(25, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(25, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(25, 40).Value

Else
End If

If Hoja4.Cells(26, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(26, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(26, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(26, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(26, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(26, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(26, 39).Value

Else
End If

If Hoja4.Cells(26, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(26, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(26, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(26, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(26, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(26, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(26, 40).Value

Else
End If

If Hoja4.Cells(27, 14).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(27, 14).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(27, 15).Value
Hoja8.Cells(fila, 7).Value = Hoja4.Cells(27, 16).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(27, 32).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(27, 35).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(27, 39).Value

Else
End If

If Hoja4.Cells(27, 18).Value > 0 Then
fila = Cells(2, 7)
Hoja8.Cells(fila, 1).Value = Range("mes").Value
Hoja8.Cells(fila, 2).Value = Range("fecha").Value
Hoja8.Cells(fila, 3).Value = Range("voucher").Value
Hoja8.Cells(fila, 4).Value = Range("comprobante").Value
Hoja8.Cells(fila, 9).Value = Range("glosa").Value
Hoja8.Cells(fila, 12).Value = Hoja2.Cells(4, 2)

Hoja8.Cells(fila, 5).Value = Hoja4.Cells(27, 18).Value
Hoja8.Cells(fila, 6).Value = Hoja4.Cells(27, 19).Value
Hoja8.Cells(fila, 8).Value = Hoja4.Cells(27, 20).Value

Hoja8.Cells(fila, 10).Value = Hoja4.Cells(27, 33).Value

Hoja8.Cells(fila, 11).Value = Hoja4.Cells(27, 36).Value

Hoja8.Cells(fila, 15).Value = Hoja4.Cells(27, 40).Value

Else
End If

grabar

Respuesta = MsgBox(Mensaje, Estilo, Título)

End Sub

Private Sub CommandButton2_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Ingrese Nuevo Registro"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.

Range("e4").Select
Selection.ClearContents

Range("b6").Select
Selection.ClearContents

Range("b7").Select
Selection.ClearContents

Range("b8").Select
Selection.ClearContents

Range("b9").Select
Selection.ClearContents

Range("b10").Select
Selection.ClearContents

Range("b31").Select
Selection.ClearContents

Range("c31").Select
Selection.ClearContents

Range("a12:b27").Select
Selection.ClearContents

Range("d12:d27").Select
Selection.ClearContents

Range("e7:e10").Select
Selection.ClearContents

Range("e12:e27").Select
Selection.ClearContents

Range("f12:f27").Select
Selection.ClearContents

Range("b30").Select
Selection.ClearContents

Range("b4").Select

Respuesta = MsgBox(Mensaje, Estilo, Título)

End Sub

Private Sub CommandButton3_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Registro Cancelado"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.

Range("e4").Select
Selection.ClearContents

Range("b6").Select
Selection.ClearContents

Range("b7").Select
Selection.ClearContents

Range("b8").Select
Selection.ClearContents

Range("b9").Select
Selection.ClearContents

Range("b10").Select
Selection.ClearContents

Range("b31").Select
Selection.ClearContents

Range("c31").Select
Selection.ClearContents

Range("a12:b27").Select
Selection.ClearContents

Range("d12:d27").Select
Selection.ClearContents

Range("e12:e27").Select
Selection.ClearContents

Range("f12:f27").Select
Selection.ClearContents

Range("b30").Select
Selection.ClearContents

Range("b4").Select

Respuesta = MsgBox(Mensaje, Estilo, Título)

End Sub

Private Sub CommandButton4_Click()

Hoja8.Activate

buscar

End Sub

Private Sub CommandButton5_Click()

Hoja22.Activate

buscar

End Sub

Private Sub CommandButton6_Click()

Hoja24.Activate

buscar

End Sub
