' Origen: SISTEMA_CONTABLE EXCEL / Hoja9.cls
Attribute VB_Name = "Hoja9"
Attribute VB_Base = "0{00020820-0000-0000-C000-000000000046}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = True
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = True
Attribute VB_Control = "Label1, 1, 0, MSForms, Label"
Attribute VB_Control = "cboMes, 2, 1, MSForms, ComboBox"
Attribute VB_Control = "ConsultaDiario, 3, 2, MSForms, CommandButton"
Private Sub cboMes_Change()
Hoja9.Cells(1, 11).Value = cboMes.Column(1)
Hoja9.Cells(2, 11).Value = cboMes.Column(0)
End Sub

Private Sub ConsultaDiario_Click()

Dim Mensaje, Estilo, Título, Ctxt
Mensaje = "Proceso Terminado"    ' Define el mensaje.
Estilo = vbInformation    ' Define los botones.
Título = "Mensaje"    ' Define el título.

If Hoja9.Cells(1, 11).Value > 0 Then

    
    consulta1diario
        
        Range("A10:K1000").Select
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

    consulta2diario

        Range("A10:K1000").Select
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

'copia formulas
    
If Range("A49") > "" Then
     
   Rows("49:50").Select
   Selection.Insert Shift:=xlDown
   Range("A49") = "*"
   Range("I49") = "VAN..."
   Range("A50") = "*"
   Range("I50") = "VIENEN..."
   
   Range("J10:J49").Select
   Range("J49").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K10:K49").Select
   Range("K49").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J50") = Range("J49")
   Range("K50") = Range("K49")
   
   Range("j46:k1250").Select
   borrarlineas
   
   Range("j49:k49").Select
   linea
   
   Else

   If Range("A49") = "" And Range("a10") > "" Then
   
   Range("A49") = "*"
   Range("I49") = "TOTAL"
   
   Range("J10:J49").Select
   Range("J49").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K10:K49").Select
   Range("K49").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j49:k1250").Select
   borrarlineas
   
   Range("j49:k49").Select
   linea1
    Else
   End If

End If

If Range("A89") > "" Then
    
   Rows("89:90").Select
   Selection.Insert Shift:=xlDown
   Range("A89") = "*"
   Range("I89") = "VAN..."
   Range("A90") = "*"
   Range("I90") = "VIENEN..."
   
   Range("J50:J89").Select
   Range("J89").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K50:K89").Select
   Range("K89").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J90") = Range("J89")
   Range("K90") = Range("K89")
   
   Range("j89:k1250").Select
   borrarlineas
   
   Range("j89:k89").Select
   linea
   
   Else

   If Range("A89") = "" And Range("a51") > "" Then
   
   Range("A89") = "*"
   Range("I89") = "TOTAL"
   
   Range("J50:J89").Select
   Range("J89").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K50:K89").Select
   Range("K89").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j89:k1250").Select
   borrarlineas
   
   Range("j89:k89").Select
   linea1
   
    Else
   End If
End If

If Range("A129") > "" Then
    
   Rows("129:130").Select
   Selection.Insert Shift:=xlDown
   Range("A129") = "*"
   Range("I129") = "VAN..."
   Range("A130") = "*"
   Range("I130") = "VIENEN..."
   
   Range("J90:J129").Select
   Range("J129").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K90:K129").Select
   Range("K129").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J130") = Range("J129")
   Range("K130") = Range("K129")
   
   Range("j129:k1250").Select
   borrarlineas
   
   Range("j129:k129").Select
   linea
   
   Else

   If Range("A129") = "" And Range("a91") > "" Then
   
   Range("A129") = "*"
   Range("I129") = "TOTAL"
   
   Range("J90:J129").Select
   Range("J129").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K90:K129").Select
   Range("K129").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j129:k1250").Select
   borrarlineas
   
   Range("j129:k129").Select
   linea1
   
    Else
   End If
End If

If Range("A169") > "" Then
    
   Rows("169:170").Select
   Selection.Insert Shift:=xlDown
   Range("A169") = "*"
   Range("I169") = "VAN..."
   Range("A170") = "*"
   Range("I170") = "VIENEN..."
   
   Range("J130:J169").Select
   Range("J169").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K130:K169").Select
   Range("K169").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J170") = Range("J169")
   Range("K170") = Range("K169")
   
   Range("j169:k1250").Select
   borrarlineas
   
   Range("j169:k169").Select
   linea
   
   Else

   If Range("A169") = "" And Range("a131") > "" Then
   
   Range("A169") = "*"
   Range("I169") = "TOTAL"
   
   Range("J130:J169").Select
   Range("J169").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K130:K169").Select
   Range("K169").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j169:k1250").Select
   borrarlineas
   
   Range("j169:k169").Select
   linea1
   
    Else
   End If
End If

If Range("A209") > "" Then
    
   Rows("209:210").Select
   Selection.Insert Shift:=xlDown
   Range("A209") = "*"
   Range("I209") = "VAN..."
   Range("A210") = "*"
   Range("I210") = "VIENEN..."
   
   Range("J170:J209").Select
   Range("J209").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K170:K209").Select
   Range("K209").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J210") = Range("J209")
   Range("K210") = Range("K209")
   
   Range("j209:k1250").Select
   borrarlineas
   
   Range("j209:k209").Select
   linea
   
   Else

   If Range("A209") = "" And Range("a171") > "" Then
   
   Range("A209") = "*"
   Range("I209") = "TOTAL"
   
   Range("J170:J209").Select
   Range("J209").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K170:K209").Select
   Range("K209").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j209:k1250").Select
   borrarlineas
   
   Range("j209:k209").Select
   linea1
   
    Else
   End If
End If

If Range("A249") > "" Then
    
   Rows("249:250").Select
   Selection.Insert Shift:=xlDown
   Range("A249") = "*"
   Range("I249") = "VAN..."
   Range("A250") = "*"
   Range("I250") = "VIENEN..."
   
   Range("J210:J249").Select
   Range("J249").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K210:K249").Select
   Range("K249").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J250") = Range("J249")
   Range("K250") = Range("K249")
   
   Range("j249:k1250").Select
   borrarlineas
   
   Range("j249:k249").Select
   linea
   
   Else

   If Range("A249") = "" And Range("a211") > "" Then
   
   Range("A249") = "*"
   Range("I249") = "TOTAL"
   
   Range("J210:J249").Select
   Range("J249").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K210:K249").Select
   Range("K249").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j249:k1250").Select
   borrarlineas
   
   Range("j249:k249").Select
   linea1
   
    Else
   End If
End If

If Range("A289") > "" Then
    
   Rows("289:290").Select
   Selection.Insert Shift:=xlDown
   Range("A289") = "*"
   Range("I289") = "VAN..."
   Range("A290") = "*"
   Range("I290") = "VIENEN..."
   
   Range("J250:J289").Select
   Range("J289").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K250:K289").Select
   Range("K289").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J290") = Range("J289")
   Range("K290") = Range("K289")
   
   Range("j289:k1250").Select
   borrarlineas
   
   Range("j289:k289").Select
   linea
   
   Else

   If Range("A289") = "" And Range("a251") > "" Then
   
   Range("A289") = "*"
   Range("I289") = "TOTAL"
   
   Range("J250:J289").Select
   Range("J289").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K250:K289").Select
   Range("K289").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j289:k1250").Select
   borrarlineas
   
   Range("j289:k289").Select
   linea1
   
    Else
   End If
End If

If Range("A329") > "" Then
    
   Rows("329:330").Select
   Selection.Insert Shift:=xlDown
   Range("A329") = "*"
   Range("I329") = "VAN..."
   Range("A330") = "*"
   Range("I330") = "VIENEN..."
   
   Range("J290:J329").Select
   Range("J329").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K290:K329").Select
   Range("K329").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J330") = Range("J329")
   Range("K330") = Range("K329")
   
   Range("j329:k1250").Select
   borrarlineas
   
   Range("j329:k329").Select
   linea
   
   Else

   If Range("A329") = "" And Range("a291") > "" Then
   
   Range("A329") = "*"
   Range("I329") = "TOTAL"
   
   Range("J290:J329").Select
   Range("J329").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K290:K329").Select
   Range("K329").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j329:k1250").Select
   borrarlineas
   
   Range("j329:k329").Select
   linea1
   
    Else
   End If
End If

If Range("A369") > "" Then
    
   Rows("369:370").Select
   Selection.Insert Shift:=xlDown
   Range("A369") = "*"
   Range("I369") = "VAN..."
   Range("A370") = "*"
   Range("I370") = "VIENEN..."
   
   Range("J330:J369").Select
   Range("J369").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K330:K369").Select
   Range("K369").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J370") = Range("J369")
   Range("K370") = Range("K369")
   
   Range("j369:k1250").Select
   borrarlineas
   
   Range("j369:k369").Select
   linea
   
   Else

   If Range("A369") = "" And Range("a331") > "" Then
   
   Range("A369") = "*"
   Range("I369") = "TOTAL"
   
   Range("J330:J369").Select
   Range("J369").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K330:K369").Select
   Range("K369").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j369:k1250").Select
   borrarlineas
   
   Range("j369:k369").Select
   linea1
   
    Else
   End If
End If

If Range("A409") > "" Then
    
   Rows("409:410").Select
   Selection.Insert Shift:=xlDown
   Range("A409") = "*"
   Range("I409") = "VAN..."
   Range("A410") = "*"
   Range("I410") = "VIENEN..."
   
   Range("J370:J409").Select
   Range("J409").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K370:K409").Select
   Range("K409").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J410") = Range("J409")
   Range("K410") = Range("K409")
   
   Range("j409:k1250").Select
   borrarlineas
   
   Range("j409:k409").Select
   linea
   
   Else

   If Range("A409") = "" And Range("a371") > "" Then
   
   Range("A409") = "*"
   Range("I409") = "TOTAL"
   
   Range("J370:J409").Select
   Range("J409").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K370:K409").Select
   Range("K409").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j409:k1250").Select
   borrarlineas
   
   Range("j409:k409").Select
   linea1
   
    Else
   End If
End If

If Range("A449") > "" Then
    
   Rows("449:450").Select
   Selection.Insert Shift:=xlDown
   Range("A449") = "*"
   Range("I449") = "VAN..."
   Range("A450") = "*"
   Range("I450") = "VIENEN..."
   
   Range("J410:J449").Select
   Range("J449").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K410:K449").Select
   Range("K449").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J450") = Range("J449")
   Range("K450") = Range("K449")
   
   Range("j449:k1250").Select
   borrarlineas
   
   Range("j449:k449").Select
   linea
   
   Else

   If Range("A449") = "" And Range("a411") > "" Then
   
   Range("A449") = "*"
   Range("I449") = "TOTAL"
   
   Range("J410:J449").Select
   Range("J449").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K410:K449").Select
   Range("K449").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j449:k1250").Select
   borrarlineas
   
   Range("j449:k449").Select
   linea1
   
    Else
   End If
End If

If Range("A489") > "" Then
    
   Rows("489:490").Select
   Selection.Insert Shift:=xlDown
   Range("A489") = "*"
   Range("I489") = "VAN..."
   Range("A490") = "*"
   Range("I490") = "VIENEN..."
   
   Range("J450:J489").Select
   Range("J489").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K450:K489").Select
   Range("K489").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J490") = Range("J489")
   Range("K490") = Range("K489")
   
   Range("j489:k1250").Select
   borrarlineas
   
   Range("j489:k489").Select
   linea
   
   Else

   If Range("A489") = "" And Range("a451") > "" Then
   
   Range("A489") = "*"
   Range("I489") = "TOTAL"
   
   Range("J450:J489").Select
   Range("J489").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K450:K489").Select
   Range("K489").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j489:k1250").Select
   borrarlineas
   
   Range("j489:k489").Select
   linea1
   
    Else
   End If
End If

If Range("A529") > "" Then
    
   Rows("529:530").Select
   Selection.Insert Shift:=xlDown
   Range("A529") = "*"
   Range("I529") = "VAN..."
   Range("A530") = "*"
   Range("I530") = "VIENEN..."
   
   Range("J490:J529").Select
   Range("J529").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K490:K529").Select
   Range("K529").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
  
   Range("J530") = Range("J529")
   Range("K530") = Range("K529")
    
   Range("j529:k1250").Select
   borrarlineas
   
   Range("j529:k529").Select
   linea
   
   Else

   If Range("A529") = "" And Range("a491") > "" Then
   
   Range("A529") = "*"
   Range("I529") = "TOTAL"
   
   Range("J490:J529").Select
   Range("J529").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K490:K529").Select
   Range("K529").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j529:k1250").Select
   borrarlineas
   
   Range("j529:k529").Select
   linea1
   
    Else
   End If
End If

If Range("A569") > "" Then
    
   Rows("569:570").Select
   Selection.Insert Shift:=xlDown
   Range("A569") = "*"
   Range("I569") = "VAN..."
   Range("A570") = "*"
   Range("I570") = "VIENEN..."
   
   Range("J530:J569").Select
   Range("J569").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K530:K569").Select
   Range("K569").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J570") = Range("J569")
   Range("K570") = Range("K569")
   
   Range("j569:k1250").Select
   borrarlineas
   
   Range("j569:k569").Select
   linea
   
   Else

   If Range("A569") = "" And Range("a531") > "" Then
   
   Range("A569") = "*"
   Range("I569") = "TOTAL"
   
   Range("J530:J569").Select
   Range("J569").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K530:K569").Select
   Range("K569").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j569:k1250").Select
   borrarlineas
   
   Range("j569:k569").Select
   linea1
   
    Else
   End If
End If

If Range("A609") > "" Then
    
   Rows("609:610").Select
   Selection.Insert Shift:=xlDown
   Range("A609") = "*"
   Range("I609") = "VAN..."
   Range("A610") = "*"
   Range("I610") = "VIENEN..."
   
   Range("J570:J609").Select
   Range("J609").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K570:K609").Select
   Range("K609").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J610") = Range("J609")
   Range("K610") = Range("K609")
   
   Range("j609:k1250").Select
   borrarlineas
   
   Range("j609:k609").Select
   linea
   
   Else

   If Range("A609") = "" And Range("a571") > "" Then
   
   Range("A609") = "*"
   Range("I609") = "TOTAL"
   
   Range("J570:J609").Select
   Range("J609").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K570:K609").Select
   Range("K609").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j609:k1250").Select
   borrarlineas
   
   Range("j609:k609").Select
   linea1
   
    Else
   End If
End If

If Range("A649") > "" Then
    
   Rows("649:650").Select
   Selection.Insert Shift:=xlDown
   Range("A649") = "*"
   Range("I649") = "VAN..."
   Range("A650") = "*"
   Range("I650") = "VIENEN..."
   
   Range("J610:J649").Select
   Range("J649").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K610:K649").Select
   Range("K649").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J650") = Range("J649")
   Range("K650") = Range("K649")
   
   Range("j649:k1250").Select
   borrarlineas
   
   Range("j649:k649").Select
   linea
   
   Else

   If Range("A649") = "" And Range("a611") > "" Then
   
   Range("A649") = "*"
   Range("I649") = "TOTAL"
   
   Range("J610:J649").Select
   Range("J649").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K610:K649").Select
   Range("K649").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j649:k1250").Select
   borrarlineas
   
   Range("j649:k649").Select
   linea1
   
    Else
   End If
End If

If Range("A689") > "" Then
    
   Rows("689:690").Select
   Selection.Insert Shift:=xlDown
   Range("A689") = "*"
   Range("I689") = "VAN..."
   Range("A690") = "*"
   Range("I690") = "VIENEN..."
   
   Range("J650:J689").Select
   Range("J689").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K650:K689").Select
   Range("K689").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J690") = Range("J689")
   Range("K690") = Range("K689")
   
   Range("j689:k1250").Select
   borrarlineas
   
   Range("j689:k689").Select
   linea
   
   Else

   If Range("A689") = "" And Range("a651") > "" Then
   
   Range("A689") = "*"
   Range("I689") = "TOTAL"
   
   Range("J650:J689").Select
   Range("J689").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K650:K689").Select
   Range("K689").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j689:k1250").Select
   borrarlineas
   
   Range("j689:k689").Select
   linea1
   
    Else
   End If
End If

If Range("A729") > "" Then
    
   Rows("729:730").Select
   Selection.Insert Shift:=xlDown
   Range("A729") = "*"
   Range("I729") = "VAN..."
   Range("A730") = "*"
   Range("I730") = "VIENEN..."
   
   Range("J690:J729").Select
   Range("J729").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K690:K729").Select
   Range("K729").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J730") = Range("J729")
   Range("K730") = Range("K729")
   
   Range("j729:k1250").Select
   borrarlineas
   
   Range("j729:k729").Select
   linea
   
   Else

   If Range("A729") = "" And Range("a691") > "" Then
   
   Range("A729") = "*"
   Range("I729") = "TOTAL"
   
   Range("J690:J729").Select
   Range("J729").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K690:K729").Select
   Range("K729").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j729:k1250").Select
   borrarlineas
   
   Range("j729:k729").Select
   linea1
   
    Else
   End If
End If

If Range("A769") > "" Then
    
   Rows("769:770").Select
   Selection.Insert Shift:=xlDown
   Range("A769") = "*"
   Range("I769") = "VAN..."
   Range("A770") = "*"
   Range("I770") = "VIENEN..."
   
   Range("J730:J769").Select
   Range("J769").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K730:K769").Select
   Range("K769").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J770") = Range("J769")
   Range("K770") = Range("K769")
   
   Range("j769:k1250").Select
   borrarlineas
   
   Range("j769:k769").Select
   linea
   
   Else

   If Range("A769") = "" And Range("a731") > "" Then
   
   Range("A769") = "*"
   Range("I769") = "TOTAL"
   
   Range("J730:J769").Select
   Range("J769").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K730:K769").Select
   Range("K769").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j769:k1250").Select
   borrarlineas
   
   Range("j769:k769").Select
   linea1
   
    Else
   End If
End If

If Range("A809") > "" Then
    
   Rows("809:810").Select
   Selection.Insert Shift:=xlDown
   Range("A809") = "*"
   Range("I809") = "VAN..."
   Range("A810") = "*"
   Range("I810") = "VIENEN..."
   
   Range("J770:J809").Select
   Range("J809").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K770:K809").Select
   Range("K809").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J810") = Range("J809")
   Range("K810") = Range("K809")
   
   Range("j809:k1250").Select
   borrarlineas
   
   Range("j809:k809").Select
   linea
   
   Else

   If Range("A809") = "" And Range("a771") > "" Then
   
   Range("A809") = "*"
   Range("I809") = "TOTAL"
   
   Range("J770:J809").Select
   Range("J809").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K770:K809").Select
   Range("K809").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j809:k1250").Select
   borrarlineas
   
   Range("j809:k809").Select
   linea1
   
    Else
   End If
End If

If Range("A849") > "" Then
    
   Rows("849:850").Select
   Selection.Insert Shift:=xlDown
   Range("A849") = "*"
   Range("I849") = "VAN..."
   Range("A850") = "*"
   Range("I850") = "VIENEN..."
   
   Range("J810:J849").Select
   Range("J849").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K810:K849").Select
   Range("K849").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J850") = Range("J849")
   Range("K850") = Range("K849")
   
   Range("j849:k1250").Select
   borrarlineas
   
   Range("j849:k849").Select
   linea
   
   Else

   If Range("A849") = "" And Range("a811") > "" Then
   
   Range("A849") = "*"
   Range("I849") = "TOTAL"
   
   Range("J810:J849").Select
   Range("J849").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K810:K849").Select
   Range("K849").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j849:k1250").Select
   borrarlineas
   
   Range("j849:k849").Select
   linea1
   
    Else
   End If
End If

If Range("A889") > "" Then
    
   Rows("889:890").Select
   Selection.Insert Shift:=xlDown
   Range("A889") = "*"
   Range("I889") = "VAN..."
   Range("A890") = "*"
   Range("I890") = "VIENEN..."
   
   Range("J850:J889").Select
   Range("J889").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K850:K889").Select
   Range("K889").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J890") = Range("J889")
   Range("K890") = Range("K889")
   
   Range("j889:k1250").Select
   borrarlineas
   
   Range("j889:k889").Select
   linea
   
   Else

   If Range("A889") = "" And Range("a851") > "" Then
   
   Range("A889") = "*"
   Range("I889") = "TOTAL"
   
   Range("J850:J889").Select
   Range("J889").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K850:K889").Select
   Range("K889").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j889:k1250").Select
   borrarlineas
   
   Range("j889:k889").Select
   linea1
   
    Else
   End If
End If

If Range("A929") > "" Then
    
   Rows("929:930").Select
   Selection.Insert Shift:=xlDown
   Range("A929") = "*"
   Range("I929") = "VAN..."
   Range("A930") = "*"
   Range("I930") = "VIENEN..."
   
   Range("J890:J929").Select
   Range("J929").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K890:K929").Select
   Range("K929").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J930") = Range("J929")
   Range("K930") = Range("K929")
   
   Range("j929:k1250").Select
   borrarlineas
   
   Range("j929:k929").Select
   linea
   
   Else

   If Range("A929") = "" And Range("a891") > "" Then
   
   Range("A929") = "*"
   Range("I929") = "TOTAL"
   
   Range("J890:J929").Select
   Range("J929").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K890:K929").Select
   Range("K929").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j929:k1250").Select
   borrarlineas
   
   Range("j929:k929").Select
   linea1
   
    Else
   End If
End If

If Range("A969") > "" Then
    
   Rows("969:970").Select
   Selection.Insert Shift:=xlDown
   Range("A969") = "*"
   Range("I969") = "VAN..."
   Range("A970") = "*"
   Range("I970") = "VIENEN..."
   
   Range("J930:J969").Select
   Range("J969").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K930:K969").Select
   Range("K969").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("J970") = Range("J969")
   Range("K970") = Range("K969")
   
   Range("j969:k1250").Select
   borrarlineas
   
   Range("j969:k969").Select
   linea
   
   Else

   If Range("A969") = "" And Range("a931") > "" Then
   
   Range("A969") = "*"
   Range("I969") = "TOTAL"
   
   Range("J930:J969").Select
   Range("J969").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
   Range("K930:K969").Select
   Range("K969").Activate
   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

   Range("j969:k1250").Select
   borrarlineas
   
   Range("j969:k969").Select
   linea1
   
    Else
   End If
End If

'If Range("A971") > "" Then
   
'   Range("A1009") = "*"
'   Range("I1009") = "TOTAL"
   
'   Range("J970:J1009").Select
'   Range("J1009").Activate
'   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"
   
'   Range("K970:K1009").Select
'   Range("K1009").Activate
'   ActiveCell.FormulaR1C1 = "=SUM(R[-39]C:R[-1]C)"

'   Range("j1009:k1009").Select
'   linea1
   
'   Else
'End If




    
    
    
    
    
    
    'elimina
    
    filasvacias

Range("a1").Select

Respuesta = MsgBox(Mensaje, Estilo, Título)

End Sub
