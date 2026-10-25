' Origen: SISTEMA_CONTABLE EXCEL / Módulo1.bas
Attribute VB_Name = "Módulo1"
Sub ser()

Range("dosdig").AdvancedFilter Action:=xlFilterCopy, CopyToRange:=Sheets("Ht2d").Range( _
        "b7"), Unique:=True


End Sub
Sub ser1()

Range("tresdig").AdvancedFilter Action:=xlFilterCopy, CopyToRange:=Sheets("Ht3d").Range( _
        "b7"), Unique:=True


End Sub
Sub ser2()

Range("cincodig").AdvancedFilter Action:=xlFilterCopy, CopyToRange:=Sheets("Ht5d").Range( _
        "b7"), Unique:=True


End Sub
Sub orden2d()

Range("a11:L153").Select
    Selection.Sort Key1:=Range("a12:A153"), Order1:=xlAscending, Header:=xlGuess, _
        OrderCustom:=1, MatchCase:=False, Orientation:=xlTopToBottom, _
        DataOption1:=xlSortNormal

End Sub
Sub orden3d()

Range("a11:L211").Select
    Selection.Sort Key1:=Range("a12:A211"), Order1:=xlAscending, Header:=xlGuess, _
        OrderCustom:=1, MatchCase:=False, Orientation:=xlTopToBottom, _
        DataOption1:=xlSortNormal

End Sub
Sub orden5d()

Range("a11:L1011").Select
    Selection.Sort Key1:=Range("a12:A1011"), Order1:=xlAscending, Header:=xlGuess, _
        OrderCustom:=1, MatchCase:=False, Orientation:=xlTopToBottom, _
        DataOption1:=xlSortNormal

End Sub
Sub ordenmayor()

Range("a11:h1250").Select
    Selection.Sort Key1:=Range("D12:D1250"), Order1:=xlAscending, Header:=xlGuess, _
        OrderCustom:=1, MatchCase:=False, Orientation:=xlTopToBottom, _
        DataOption1:=xlSortNormal

End Sub

Sub extraebalance2d()

Range("comprob2d").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("A6:A7"), CopyToRange:=Range("A11:l11"), Unique:=False

End Sub
Sub extraebalance3d()

Range("comprob3d").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("A6:A7"), CopyToRange:=Range("elsy"), Unique:=False

End Sub
Sub extraebalance5d()

Range("comprob5d").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("A6:A7"), CopyToRange:=Range("A11:l11"), Unique:=False

End Sub

Sub filasvacias()

intLastRow = Columns("a:a").Range("a65535").End(xlUp).Row
For r = intLastRow To 1 Step -1
 If Application.CountA(Rows(r)) = 0 Then Rows(r).Delete
Next r

End Sub

Sub consulta1diario()

Rows("10:1250").Select
Selection.Delete Shift:=xlUp

Range("DATADIARIO").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("j1:j2"), CopyToRange:=Range("A9:k9"), Unique:=False

End Sub

Sub consulta2diario()

Rows("10:1250").Select
Selection.Delete Shift:=xlUp

Range("DATADIARIO").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("j3:k3"), CopyToRange:=Range("A9:k9"), Unique:=False

End Sub
Sub CONSULTARV()

Rows("11:200").Select
Selection.Delete Shift:=xlUp

Range("DATARV").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("Y1:Y2"), CopyToRange:=Range("A10:W10"), Unique:=False
End Sub
Sub CONSULTARC()

Rows("12:200").Select
Selection.Delete Shift:=xlUp

Range("DATARC").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("AE1:AE2"), CopyToRange:=Range("A11:AC11"), Unique:=False

End Sub

Sub filtrocuenta()

Range("DATADIARIO").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("h1:h2"), CopyToRange:=Range("CABEZAMAYOR"), Unique:=False

End Sub

Sub consultamayor1()

Range("datamayor").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("h3:h4"), CopyToRange:=Range("mayor"), Unique:=False

End Sub

Sub consultamayor2()

Range("datamayor").AdvancedFilter Action:=xlFilterCopy, CriteriaRange:=Range("g1:g2"), CopyToRange:=Range("mayor"), Unique:=False

End Sub

Sub grabar()

'Vouchers

If Hoja4.Cells(6, 26).Value > 0 Then
Hoja6.Cells(4, 4).Value = Hoja4.Cells(6, 26).Value
Else
End If

If Hoja4.Cells(7, 26).Value > 0 Then
Hoja6.Cells(5, 4).Value = Hoja4.Cells(7, 26).Value
Else
End If

If Hoja4.Cells(8, 26).Value > 0 Then
Hoja6.Cells(6, 4).Value = Hoja4.Cells(8, 26).Value
Else
End If

If Hoja4.Cells(9, 26).Value > 0 Then
Hoja6.Cells(7, 4).Value = Hoja4.Cells(9, 26).Value
Else
End If

If Hoja4.Cells(10, 26).Value > 0 Then
Hoja6.Cells(8, 4).Value = Hoja4.Cells(10, 26).Value
Else
End If

If Hoja4.Cells(11, 26).Value > 0 Then
Hoja6.Cells(9, 4).Value = Hoja4.Cells(11, 26).Value
Else
End If

If Hoja4.Cells(12, 26).Value > 0 Then
Hoja6.Cells(10, 4).Value = Hoja4.Cells(12, 26).Value
Else
End If

If Hoja4.Cells(13, 26).Value > 0 Then
Hoja6.Cells(11, 4).Value = Hoja4.Cells(13, 26).Value
Else
End If

If Hoja4.Cells(14, 26).Value > 0 Then
Hoja6.Cells(12, 4).Value = Hoja4.Cells(14, 26).Value
Else
End If

If Hoja4.Cells(15, 26).Value > 0 Then
Hoja6.Cells(13, 4).Value = Hoja4.Cells(14, 26).Value
Else
End If

If Hoja4.Cells(16, 26).Value > 0 Then
Hoja6.Cells(14, 4).Value = Hoja4.Cells(16, 26).Value
Else
End If

If Hoja4.Cells(17, 26).Value > 0 Then
Hoja6.Cells(15, 4).Value = Hoja4.Cells(17, 26).Value
Else
End If

If Hoja4.Cells(18, 26).Value > 0 Then
Hoja6.Cells(16, 4).Value = Hoja4.Cells(18, 26).Value
Else
End If

If Hoja4.Cells(19, 26).Value > 0 Then
Hoja6.Cells(17, 4).Value = Hoja4.Cells(19, 26).Value
Else
End If

If Hoja4.Cells(20, 26).Value > 0 Then
Hoja6.Cells(18, 4).Value = Hoja4.Cells(20, 26).Value
Else
End If

End Sub

Sub linea()

Selection.Borders(xlDiagonalDown).LineStyle = xlNone
    Selection.Borders(xlDiagonalUp).LineStyle = xlNone
    Selection.Borders(xlEdgeLeft).LineStyle = xlNone
    With Selection.Borders(xlEdgeTop)
        .LineStyle = xlContinuous
        .Weight = xlThin
        .ColorIndex = xlAutomatic
    End With
    Selection.Borders(xlEdgeBottom).LineStyle = xlNone
    Selection.Borders(xlEdgeRight).LineStyle = xlNone
    Selection.Borders(xlInsideVertical).LineStyle = xlNone

End Sub

Sub linea1()

Selection.Borders(xlDiagonalDown).LineStyle = xlNone
    Selection.Borders(xlDiagonalUp).LineStyle = xlNone
    Selection.Borders(xlEdgeLeft).LineStyle = xlNone
    With Selection.Borders(xlEdgeTop)
        .LineStyle = xlContinuous
        .Weight = xlThin
        .ColorIndex = xlAutomatic
    End With
    With Selection.Borders(xlEdgeBottom)
        .LineStyle = xlDouble
        .Weight = xlThick
        .ColorIndex = xlAutomatic
    End With
    Selection.Borders(xlEdgeRight).LineStyle = xlNone
    Selection.Borders(xlInsideVertical).LineStyle = xlNone

End Sub

Sub borrarlineas()
    
    Selection.Borders(xlDiagonalDown).LineStyle = xlNone
    Selection.Borders(xlDiagonalUp).LineStyle = xlNone
    Selection.Borders(xlEdgeLeft).LineStyle = xlNone
    Selection.Borders(xlEdgeTop).LineStyle = xlNone
    Selection.Borders(xlEdgeBottom).LineStyle = xlNone
    Selection.Borders(xlEdgeRight).LineStyle = xlNone
    Selection.Borders(xlInsideVertical).LineStyle = xlNone
    Selection.Borders(xlInsideHorizontal).LineStyle = xlNone

End Sub

Sub buscar()
Attribute buscar.VB_ProcData.VB_Invoke_Func = "b\n14"

Dim ELSY As String
    
ELSY = InputBox("Introduzca el Número de Voucher a buscar", "Busqueda de Voucher")

Cells.Find(What:=ELSY, LookIn:=xlFormulas, LookAt _
:=xlPart, SearchOrder:=xlByRows, SearchDirection:=xlNext, MatchCase:= _
False).Activate

End Sub


Sub planc()
Attribute planc.VB_ProcData.VB_Invoke_Func = "p\n14"

UserForm1.Show


End Sub

Sub Columnasrvrc()
Attribute Columnasrvrc.VB_ProcData.VB_Invoke_Func = "s\n14"

ColumnasForm.Show


End Sub

Sub listasclientesproveedores()
Attribute listasclientesproveedores.VB_ProcData.VB_Invoke_Func = "l\n14"

If Hoja4.Range("e4").Value = "Ventas" Then

    ClientesForm.Show

    Else
    If Hoja4.Range("e4").Value = "Compras" Then
    
    Proveedores.Show
    
    End If

End If

End Sub
