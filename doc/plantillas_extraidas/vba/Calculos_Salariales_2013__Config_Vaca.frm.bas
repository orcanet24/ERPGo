' Origen: Calculos Salariales 2013 / Config_Vaca.frm
Attribute VB_Name = "Config_Vaca"
Attribute VB_Base = "0{3B363A11-600F-42B1-96BA-85B5BAB7F701}{30EC6A06-EB50-464B-BFFF-05C3F4D8311B}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = False
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = False
Private Sub Frame2_Click()

End Sub

Private Sub CB_tipodia_Change()

End Sub

Private Sub CheckBox1_Click()

End Sub

Private Sub Chk_bonolottt_Click()
    If Chk_bonolottt.Value = True Then
            Label3.Enabled = False
            TB_cantidaddiasbono.Enabled = False
        Else
            Label3.Enabled = True
            TB_cantidaddiasbono.Enabled = True
    End If
End Sub

Private Sub Chk_vacafrac_Click()
    If Chk_vacafrac.Value = True Then
    Tb_meses.Enabled = False
    Else
    Tb_meses.Enabled = True
    End If
End Sub

' Instrucciones del boton ACEPTAR
Private Sub CommandButton1_Click()
    If Worksheets("Main").Range("B24").Value <> "Por: DORELIRA" Then
        MsgBox "Se ha detectado que la hoja fue alterada, eso puede ocurrir debido a que no ha comprado el programa por medio del vendedor autorizado en mercadolibre DORELIRA, NO se garantiza el buen funcionamiento ni soporte de esta hoja.", vbExclamation
        Exit Sub
    End If
    If OptionButton2.Value = True Then
        'Carga la hoja con los datos personalizados
        Range("E8").Value = TB_cantidaddias.Value
        Range("F8").Value = CB_tipodia.Value
        Range("F9").Value = CB_tipodia.Value
        Range("e9").Value = 0
        If Chk_bonolottt.Value = False Then
                Range("E15").Value = TB_cantidaddiasbono.Value
                Range("E16").Value = 0
            Else
                Range("E15").Value = 15
                Range("E16").FormulaLocal = "=SI(Main!$C$11<=15;SI(Main!$C$11>1;Main!$C$11-1;0);15)"
        End If
    Else
        'carga la hoja segun minimo de LOTTT
        Range("E8").Value = 15
        Range("E9").FormulaLocal = "=SI(Main!$C$11<=15;SI(Main!$C$11>1;Main!$C$11;0);15)"
        Range("E15").Value = 15
        Range("E16").FormulaLocal = "=SI(Main!$C$11<=15;SI(Main!$C$11>=1;Main!$C$11;0);15)"
        Range("F8").Value = "Hábiles"
        Range("F9").Value = "Hábiles"
    End If
    
    If Chk_vacafrac.Value = True Then
        Range("F44").FormulaLocal = "= MES(Main!C6) - MES(Main!C5)"
    Else
        Range("F44").Value = Tb_meses.Value
    End If
    
    End
End Sub

Private Sub CommandButton2_Click()
    End
End Sub

Private Sub F_diasvaca_Click()

End Sub

Private Sub Frame1_Click()

End Sub

Private Sub Frame3_Click()

End Sub

Private Sub Label1_Click()

End Sub

Private Sub OptionButton2_Click()
activatodo
End Sub

Private Sub RB_minlottt_Click()
    Chk_bonolottt.Value = True
    desactivatodo
End Sub

Private Sub Tb_meses_Change()

End Sub

Private Sub UserForm_Activate()
    RB_minlottt.Value = True
    Chk_bonolottt.Value = True
    Label3.Enabled = False
    TB_cantidaddiasbono.Enabled = False
    desactivatodo
    CB_tipodia.AddItem ("Hábiles")
    CB_tipodia.AddItem ("Contínuos")
    CB_tipodia.Value = "Hábiles"
    Chk_vacafrac.Value = True
End Sub

'Inhabilita los controles para configuracion personal dias
Private Sub desactivatodo()
    F_diasvaca.Enabled = False
    CB_tipodia.Enabled = False
    Label1.Enabled = False
    Label2.Enabled = False
    TB_cantidaddias.Enabled = False
    Label3.Enabled = False
    Frame3.Enabled = False
    TB_cantidaddiasbono.Enabled = False
    Chk_bonolottt.Enabled = False
End Sub

'Activa todos los controles para el calculo de vacaciones con parametros personalizados
Private Sub activatodo()
    F_diasvaca.Enabled = True
    CB_tipodia.Enabled = True
    Label1.Enabled = True
    Label2.Enabled = True
    TB_cantidaddias.Enabled = True
    Frame3.Enabled = True
    Chk_bonolottt.Enabled = True
End Sub



